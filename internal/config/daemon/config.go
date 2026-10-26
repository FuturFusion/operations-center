// Package config holds the runtime configuration of the daemon.
//
// The configuration is a process wide singleton, backed by a store which
// documents the locking rules that apply to every change.
package config

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type localConfig struct {
	Network system.Network `json:"network" yaml:"network"`

	Settings system.Settings `json:"settings" yaml:"settings"`
}

type InternalConfig struct {
	IsBackgroundTasksDisabled bool
	SourcePollSkipFirst       bool
}

type enver interface {
	VarDir() string
	IsIncusOS() bool
	Clustered() bool
}

// defaultStore holds the config singleton. It is only ever replaced as a whole,
// by Init and by InitTest.
var defaultStore atomic.Pointer[store[localConfig]]

// sharedStore holds the config shared by all cluster members, the provisional one until it is loaded from the database.
var sharedStore atomic.Pointer[store[system.SharedConfig]]

func sharedConfigValidator() validateFunc[system.SharedConfig] {
	return func(ctx context.Context, currentLocalCfg localConfig, oldCfg, newCfg system.SharedConfig, env enver) error {
		err := validateShared(oldCfg, newCfg, env.IsIncusOS())
		if err != nil {
			return err
		}

		// A shared config stored before the address was required is kept until the network is changed.
		if isSharedNetworkChanged(oldCfg, newCfg) {
			err = validateSharedNetworkConfig(currentLocalCfg.Network, oldCfg.Network, newCfg.Network, env.Clustered())
			if err != nil {
				return err
			}
		}

		return validateDelegated(ctx, oldCfg, newCfg)
	}
}

// errSharedNotLoaded is returned for updates of the shared config before it has been loaded from the database.
var errSharedNotLoaded = domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Shared configuration can not be changed before the database is ready")

func persistSharedNotLoaded(context.Context, system.SharedConfig) error {
	return errSharedNotLoaded
}

// newProvisionalSharedStore returns a store holding cfg until the shared config is loaded from the database, rejecting all updates.
func newProvisionalSharedStore(env enver, cfg system.SharedConfig) (*store[system.SharedConfig], error) {
	cfg, err := normalizeShared(cfg)
	if err != nil {
		return nil, err
	}

	return newStore(env, persistSharedNotLoaded, notifyShared, normalizeShared, sharedConfigValidator(), cfg), nil
}

// ProvisionalShared returns the shared config the database is initialized with if it does not hold one yet.
func ProvisionalShared() system.SharedConfig {
	return sharedStore.Load().Get()
}

// LoadShared replaces the provisional shared config with cfg read from the database and persists later updates using persist.
func LoadShared(ctx context.Context, cfg system.SharedConfig, persist func(ctx context.Context, cfg system.SharedConfig) error) error {
	normalized, err := normalizeShared(cfg)
	if err != nil {
		return fmt.Errorf("Failed to load shared config: %w", err)
	}

	// Only the validation owned by the config package runs here, see Init.
	localStore := defaultStore.Load()
	err = validateShared(normalized, normalized, localStore.env.IsIncusOS())
	if err != nil {
		return fmt.Errorf("Failed to load shared config: %w", err)
	}

	sharedStore.Load().replace(ctx, normalized, persist)

	if reflect.DeepEqual(cfg, normalized) {
		return nil
	}

	err = persist(ctx, normalized)
	if err != nil {
		return fmt.Errorf("Failed to persist loaded shared config: %w", err)
	}

	return nil
}

// ReloadShared applies cfg already persisted by another cluster member.
func ReloadShared(ctx context.Context, cfg system.SharedConfig) error {
	normalized, err := normalizeShared(cfg)
	if err != nil {
		return fmt.Errorf("Failed to reload shared config: %w", err)
	}

	sharedStore.Load().replace(ctx, normalized, nil)

	return nil
}

func init() {
	env := environment.New(ApplicationName, ApplicationEnvPrefix)
	defaultStore.Store(newLocalStore(env, diskPersister(env), localConfig{}))
	sharedStore.Store(newStore(env, persistSharedNotLoaded, notifyShared, normalizeShared, sharedConfigValidator(), system.SharedConfig{}))
}

func Init(env enver) error {
	initInternalConfig()

	// Read the provisional shared config before the config file is rewritten with the local part only.
	provisional, err := loadProvisionalShared(env)
	if err != nil {
		return fmt.Errorf("Failed to initialize shared config: %w", err)
	}

	localCfg, contents, err := loadLocalConfig(env)
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	localCfg, err = normalize(localCfg)
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	// Only the validation owned by the config package runs here. The subsystems
	// which validate the settings they own are wired up after Init, and making
	// their probes a prerequisite for the daemon to come up would keep an
	// offline installation from starting at all.
	err = validate(localCfg, localCfg, env.IsIncusOS(), env.Clustered())
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	defaultStore.Store(newLocalStore(env, diskPersister(env), localCfg))

	shared, err := newProvisionalSharedStore(env, provisional)
	if err != nil {
		return fmt.Errorf("Failed to initialize shared config: %w", err)
	}

	sharedStore.Store(shared)

	// Only write the config file if it is missing or if normalization changed
	// something, so an unchanged config survives a restart untouched.
	normalized, err := marshalConfig(localCfg)
	if err != nil {
		return fmt.Errorf("Failed to persist initialized global config: %w", err)
	}

	if bytes.Equal(contents, normalized) {
		return ensureConfigFileMode(env)
	}

	err = saveToDisk(env, localCfg)
	if err != nil {
		return fmt.Errorf("Failed to persist initialized global config: %w", err)
	}

	return nil
}

func GetLocalNetwork() system.Network {
	return defaultStore.Load().Get().Network
}

func UpdateLocalNetwork(ctx context.Context, cfg system.NetworkPut) error {
	return defaultStore.Load().Update(ctx, sectionNetwork, func(_ context.Context, c localConfig) localConfig {
		c.Network.NetworkPut = cfg

		return c
	})
}

func GetNetwork() system.NetworkGlobal {
	return sharedStore.Load().Get().Network
}

func UpdateNetwork(ctx context.Context, cfg system.NetworkGlobalPut) error {
	return sharedStore.Load().Update(ctx, sectionNetwork, func(ctx context.Context, c system.SharedConfig) system.SharedConfig {
		c.Network.NetworkGlobalPut = cfg

		return c
	})
}

func UpdateNetworkClusterAddress(ctx context.Context, address string) error {
	store := defaultStore.Load()
	return store.set(ctx, func(oldCfg localConfig, persist func(newCfg localConfig) error) error {
		if oldCfg.Network.ClusterAddress == address {
			return nil
		}

		newCfg := oldCfg
		newCfg.Network.ClusterAddress = address

		err := validateNetworkConfigClusterAddress(oldCfg.Network, newCfg.Network, store.env.Clustered())
		if err != nil {
			return err
		}

		err = persist(newCfg)
		if err != nil {
			return err
		}

		return lifecycle.NetworkUpdateSignal.TryEmit(ctx, newCfg.Network)
	})
}

func NetworkSetDefaults(cfg system.NetworkPut) (system.NetworkPut, error) {
	newCfg := cfg
	parseIP := func(addr string) (net.IP, error) {
		if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") && len(addr) > 2 {
			addr = addr[1 : len(addr)-1]
		}

		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, domain.NewValidationErrf("Invalid config, %q is not a valid IP address", addr)
		}

		return ip, nil
	}

	if cfg.RestServerAddress != "" {
		host, port, err := net.SplitHostPort(cfg.RestServerAddress)
		if err != nil {
			ip, err := parseIP(cfg.RestServerAddress)
			if err != nil {
				return system.NetworkPut{}, err
			}

			newCfg.RestServerAddress = net.JoinHostPort(ip.String(), DefaultRestServerPort)

			return newCfg, nil
		}

		if host == "" {
			host = "::"
		}

		_, err = parseIP(host)
		if err != nil {
			return system.NetworkPut{}, err
		}

		if port == "" {
			port = DefaultRestServerPort
		}

		newCfg.RestServerAddress = net.JoinHostPort(host, port)
	}

	return newCfg, nil
}

func GetLocalSettings() system.Settings {
	return defaultStore.Load().Get().Settings
}

func UpdateLocalSettings(ctx context.Context, cfg system.SettingsPut) error {
	return defaultStore.Load().Update(ctx, sectionSettings, func(_ context.Context, c localConfig) localConfig {
		c.Settings.SettingsPut = cfg

		return c
	})
}

func GetSecurity() system.Security {
	return sharedStore.Load().Get().Security
}

func UpdateSecurity(ctx context.Context, cfg system.SecurityPut) error {
	return sharedStore.Load().Update(ctx, sectionSecurity, func(ctx context.Context, c system.SharedConfig) system.SharedConfig {
		c.Security.SecurityPut = cfg

		return c
	})
}

func GetSettings() system.SettingsGlobal {
	return sharedStore.Load().Get().Settings
}

func UpdateSettings(ctx context.Context, cfg system.SettingsGlobalPut) error {
	return sharedStore.Load().Update(ctx, sectionSettings, func(ctx context.Context, c system.SharedConfig) system.SharedConfig {
		c.Settings.SettingsGlobalPut = cfg

		return c
	})
}

func GetUpdates() system.Updates {
	return sharedStore.Load().Get().Updates
}

func UpdateUpdates(ctx context.Context, cfg system.UpdatesPut) error {
	return sharedStore.Load().Update(ctx, sectionUpdates, func(ctx context.Context, c system.SharedConfig) system.SharedConfig {
		c.Updates.UpdatesPut = cfg

		return c
	})
}

func GetCluster() system.Cluster {
	return sharedStore.Load().Get().Cluster
}

func UpdateCluster(ctx context.Context, cfg system.ClusterPut) error {
	return sharedStore.Load().Update(ctx, sectionCluster, func(ctx context.Context, c system.SharedConfig) system.SharedConfig {
		c.Cluster.ClusterPut = cfg

		return c
	})
}
