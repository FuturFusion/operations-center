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
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type localConfig struct {
	Network system.NetworkLocal `json:"network" yaml:"network"`

	Settings system.SettingsLocal `json:"settings" yaml:"settings"`
}

type InternalConfig struct {
	IsBackgroundTasksDisabled bool
	SourcePollSkipFirst       bool
}

type enver interface {
	VarDir() string
	IsIncusOS() bool
}

// defaultStore holds the config singleton. It is only ever replaced as a whole,
// by Init and by InitTest.
var defaultStore atomic.Pointer[store[localConfig]]

// sharedStore holds the config shared by all cluster members, the provisional one until it is loaded from the database.
var sharedStore atomic.Pointer[store[SharedConfig]]

func sharedConfigValidator() validateFunc[SharedConfig] {
	return func(ctx context.Context, currentLocalCfg localConfig, oldCfg, newCfg SharedConfig, env enver) error {
		err := validateShared(oldCfg, newCfg, env.IsIncusOS())
		if err != nil {
			return err
		}

		// A shared config stored before the address was required is kept until the network is changed.
		if isSharedNetworkChanged(oldCfg, newCfg) {
			err = validateSharedNetworkConfig(currentLocalCfg.Network, oldCfg.Network, newCfg.Network)
			if err != nil {
				return err
			}
		}

		return validateDelegated(ctx, oldCfg, newCfg)
	}
}

// errSharedNotLoaded is returned for updates of the shared config before it has been loaded from the database.
var errSharedNotLoaded = domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Shared configuration can not be changed before the database is ready")

func persistSharedNotLoaded(context.Context, section, SharedConfig, SharedConfig) (SharedConfig, error) {
	return SharedConfig{}, errSharedNotLoaded
}

// newProvisionalSharedStore returns a store holding cfg until the shared config is loaded from the database, rejecting all updates.
func newProvisionalSharedStore(env enver, cfg SharedConfig) (*store[SharedConfig], error) {
	cfg, err := normalizeShared(cfg)
	if err != nil {
		return nil, err
	}

	return newStore(env, persistSharedNotLoaded, notifyShared, normalizeShared, sharedConfigValidator(), cfg), nil
}

// ProvisionalShared returns the shared config the database is initialized with if it does not hold one yet.
func ProvisionalShared() SharedConfig {
	return sharedStore.Load().Get()
}

// LoadShared replaces the provisional shared config with cfg read from the database and persists later updates using persist.
func LoadShared(ctx context.Context, cfg SharedConfig, persist func(ctx context.Context, change SharedChange) (SharedConfig, error)) error {
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

	sharedStore.Load().replace(ctx, normalized, func(ctx context.Context, update section, _ SharedConfig, newCfg SharedConfig) (SharedConfig, error) {
		return persist(ctx, SharedChange{section: update, cfg: newCfg})
	})

	if reflect.DeepEqual(cfg, normalized) {
		return nil
	}

	_, err = persist(ctx, SharedChange{section: sectionChanged, cfg: normalized})
	if err != nil {
		return fmt.Errorf("Failed to persist loaded shared config: %w", err)
	}

	return nil
}

// SharedChange is a change of one section of the shared config, or of all of it.
type SharedChange struct {
	section section
	cfg     SharedConfig
}

// Apply returns stored with the changed section taken from the change.
func (c SharedChange) Apply(stored SharedConfig) SharedConfig {
	return withSharedSection(stored, c.cfg, c.section)
}

// Revert returns stored with the changed section taken from previous.
func (c SharedChange) Revert(stored SharedConfig, previous SharedConfig) SharedConfig {
	return withSharedSection(stored, previous, c.section)
}

// Applied reports whether stored still holds the changed section.
func (c SharedChange) Applied(stored SharedConfig) bool {
	return sharedEqual(c.Apply(stored), stored)
}

// withSharedSection returns dst with section taken from src, all of src for sectionChanged.
func withSharedSection(dst SharedConfig, src SharedConfig, s section) SharedConfig {
	switch s {
	case sectionNetwork:
		dst.Network = src.Network

	case sectionSecurity:
		dst.Security = src.Security

	case sectionSettings:
		dst.Settings = src.Settings

	case sectionUpdates:
		dst.Updates = src.Updates

	default:
		return src
	}

	return dst
}

// sharedEqual reports whether a and b are the same shared config once normalized.
func sharedEqual(a SharedConfig, b SharedConfig) bool {
	normalizedA, err := normalizeShared(a)
	if err != nil {
		return false
	}

	normalizedB, err := normalizeShared(b)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(normalizedA, normalizedB)
}

func init() {
	env := environment.New(ApplicationName, ApplicationEnvPrefix)
	defaultStore.Store(newLocalStore(env, diskPersister(env), localConfig{}))
	sharedStore.Store(newStore(env, persistSharedNotLoaded, notifyShared, normalizeShared, sharedConfigValidator(), SharedConfig{}))
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
	err = validate(localCfg, localCfg, env.IsIncusOS())
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

func GetLocalNetwork() system.NetworkLocal {
	return defaultStore.Load().Get().Network
}

func UpdateLocalNetwork(ctx context.Context, cfg system.NetworkLocalPut) error {
	return defaultStore.Load().Update(ctx, sectionNetwork, func(_ context.Context, c localConfig) localConfig {
		c.Network.NetworkLocalPut = cfg

		return c
	})
}

func GetNetwork() system.NetworkGlobal {
	return sharedStore.Load().Get().Network
}

func UpdateNetwork(ctx context.Context, cfg system.NetworkGlobalPut) error {
	return sharedStore.Load().Update(ctx, sectionNetwork, func(ctx context.Context, c SharedConfig) SharedConfig {
		c.Network.NetworkGlobalPut = cfg

		return c
	})
}

func NetworkSetDefaults(cfg system.NetworkLocalPut) (system.NetworkLocalPut, error) {
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
				return system.NetworkLocalPut{}, err
			}

			newCfg.RestServerAddress = net.JoinHostPort(ip.String(), DefaultRestServerPort)

			return newCfg, nil
		}

		if host == "" {
			host = "::"
		}

		_, err = parseIP(host)
		if err != nil {
			return system.NetworkLocalPut{}, err
		}

		if port == "" {
			port = DefaultRestServerPort
		}

		newCfg.RestServerAddress = net.JoinHostPort(host, port)
	}

	return newCfg, nil
}

func GetLocalSettings() system.SettingsLocal {
	return defaultStore.Load().Get().Settings
}

func UpdateLocalSettings(ctx context.Context, cfg system.SettingsLocalPut) error {
	return defaultStore.Load().Update(ctx, sectionSettings, func(_ context.Context, c localConfig) localConfig {
		c.Settings.SettingsLocalPut = cfg

		return c
	})
}

func GetSecurity() system.Security {
	return sharedStore.Load().Get().Security
}

func UpdateSecurity(ctx context.Context, cfg system.SecurityPut) error {
	return sharedStore.Load().Update(ctx, sectionSecurity, func(ctx context.Context, c SharedConfig) SharedConfig {
		c.Security.SecurityPut = cfg

		return c
	})
}

func GetSettings() system.SettingsGlobal {
	return sharedStore.Load().Get().Settings
}

func UpdateSettings(ctx context.Context, cfg system.SettingsGlobalPut) error {
	return sharedStore.Load().Update(ctx, sectionSettings, func(ctx context.Context, c SharedConfig) SharedConfig {
		c.Settings.SettingsGlobalPut = cfg

		return c
	})
}

func GetUpdates() system.Updates {
	return sharedStore.Load().Get().Updates
}

func UpdateUpdates(ctx context.Context, cfg system.UpdatesPut) error {
	return sharedStore.Load().Update(ctx, sectionUpdates, func(ctx context.Context, c SharedConfig) SharedConfig {
		c.Updates.UpdatesPut = cfg

		return c
	})
}
