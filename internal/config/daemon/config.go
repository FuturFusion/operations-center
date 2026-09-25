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
	"strings"
	"sync/atomic"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type config struct {
	Network system.Network `json:"network" yaml:"network"`

	Security system.Security `json:"security" yaml:"security"`

	Settings system.Settings `json:"settings" yaml:"settings"`

	Updates system.Updates `json:"updates" yaml:"updates"`
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
var defaultStore atomic.Pointer[store]

func init() {
	defaultStore.Store(newStore(environment.New(ApplicationName, ApplicationEnvPrefix), saveToDisk, config{}))
}

func Init(env enver) error {
	initInternalConfig()

	cfg, contents, err := loadConfig(env)
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	cfg, err = normalize(cfg)
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	// Only the validation owned by the config package runs here. The subsystems
	// which validate the settings they own are wired up after Init, and making
	// their probes a prerequisite for the daemon to come up would keep an
	// offline installation from starting at all.
	err = validate(cfg, cfg, env.IsIncusOS())
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	defaultStore.Store(newStore(env, saveToDisk, cfg))

	// Only write the config file if it is missing or if normalization changed
	// something, so an unchanged config survives a restart untouched.
	normalized, err := marshalConfig(cfg)
	if err != nil {
		return fmt.Errorf("Failed to persist initialized global config: %w", err)
	}

	if bytes.Equal(contents, normalized) {
		return ensureConfigFileMode(env)
	}

	err = saveToDisk(env, cfg)
	if err != nil {
		return fmt.Errorf("Failed to persist initialized global config: %w", err)
	}

	return nil
}

// ValidateFile validates the config file of env without applying it.
func ValidateFile(env enver) error {
	cfg, _, err := loadConfig(env)
	if err != nil {
		return err
	}

	cfg, err = normalize(cfg)
	if err != nil {
		return err
	}

	return validate(cfg, cfg, env.IsIncusOS())
}

func GetNetwork() system.Network {
	return defaultStore.Load().get().Network
}

func UpdateNetwork(ctx context.Context, cfg system.NetworkPut) error {
	return defaultStore.Load().update(ctx, sectionNetwork, func(c config) config {
		c.Network.NetworkPut = cfg

		return c
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

func GetSecurity() system.Security {
	return defaultStore.Load().get().Security
}

func UpdateSecurity(ctx context.Context, cfg system.SecurityPut) error {
	return defaultStore.Load().update(ctx, sectionSecurity, func(c config) config {
		c.Security.SecurityPut = cfg

		return c
	})
}

func GetSettings() system.Settings {
	return defaultStore.Load().get().Settings
}

func UpdateSettings(ctx context.Context, cfg system.SettingsPut) error {
	return defaultStore.Load().update(ctx, sectionSettings, func(c config) config {
		c.Settings.SettingsPut = cfg

		return c
	})
}

func GetUpdates() system.Updates {
	return defaultStore.Load().get().Updates
}

func UpdateUpdates(ctx context.Context, cfg system.UpdatesPut) error {
	return defaultStore.Load().update(ctx, sectionUpdates, func(c config) config {
		c.Updates.UpdatesPut = cfg

		return c
	})
}
