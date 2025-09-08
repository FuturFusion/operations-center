package config

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/maniartech/signals"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type config struct {
	Network api.SystemNetwork `json:"network" yaml:"network"`

	Security api.SystemSecurity `json:"security" yaml:"security"`

	Updates api.SystemUpdates `json:"updates" yaml:"updates"`
}

type varDirer interface {
	VarDir() string
}

// Global variables to hold the config singleton.
var (
	globalConfigInstanceMu sync.Mutex
	globalConfigInstance   config

	saveFunc = saveToDisk

	env varDirer = environment.New(ApplicationName, ApplicationEnvPrefix)

	NetworkUpdateSignal  = signals.NewSync[api.SystemNetwork]()
	SecurityUpdateSignal = signals.NewSync[api.SystemSecurity]()
	UpdatesUpdateSignal  = signals.NewSync[api.SystemUpdates]()
)

func Init(vardir varDirer) error {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	env = vardir

	err := loadConfig()
	if err != nil {
		return fmt.Errorf("Failed to initialize global config: %w", err)
	}

	err = validateAndSave(globalConfigInstance)
	if err != nil {
		return fmt.Errorf("Failed to persist initialized global config: %w", err)
	}

	return nil
}

func loadConfig() error {
	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal built in default config: %w", err)
	}

	filename := filepath.Join(env.VarDir(), ConfigFilename)
	contents, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			globalConfigInstance = cfg

			return nil
		}

		return err
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal config %q: %w", filename, err)
	}

	err = validate(cfg)
	if err != nil {
		return fmt.Errorf("Invalid config: %w", err)
	}

	globalConfigInstance = cfg

	return nil
}

func GetNetwork() api.SystemNetwork {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	return globalConfigInstance.Network
}

func UpdateNetwork(ctx context.Context, cfg api.SystemNetworkPut) error {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	newCfg := globalConfigInstance
	newCfg.Network.SystemNetworkPut = cfg

	err := validateAndSave(newCfg)
	if err != nil {
		return err
	}

	NetworkUpdateSignal.Emit(ctx, api.SystemNetwork{
		SystemNetworkPut: cfg,
	})

	return nil
}

func GetSecurity() api.SystemSecurity {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	return globalConfigInstance.Security
}

func UpdateSecurity(ctx context.Context, cfg api.SystemSecurityPut) error {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	newCfg := globalConfigInstance
	newCfg.Security.SystemSecurityPut = cfg

	err := validateAndSave(newCfg)
	if err != nil {
		return err
	}

	SecurityUpdateSignal.Emit(ctx, api.SystemSecurity{
		SystemSecurityPut: cfg,
	})

	return nil
}

func GetUpdates() api.SystemUpdates {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	return globalConfigInstance.Updates
}

func UpdateUpdates(ctx context.Context, cfg api.SystemUpdatesPut) error {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	newCfg := globalConfigInstance
	newCfg.Updates.SystemUpdatesPut = cfg

	err := validateAndSave(newCfg)
	if err != nil {
		return err
	}

	UpdatesUpdateSignal.Emit(ctx, api.SystemUpdates{
		SystemUpdatesPut: cfg,
	})

	return nil
}

func validateAndSave(cfg config) error {
	err := validate(cfg)
	if err != nil {
		return fmt.Errorf("Failed to validate configuration: %w", err)
	}

	return saveFunc(cfg)
}

func saveToDisk(cfg config) error {
	filename := filepath.Join(env.VarDir(), ConfigFilename)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Failed to open config %q for writing: %w", filename, err)
	}

	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	err = enc.Encode(cfg)
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return fmt.Errorf("Failed to close config %q: %w", filename, err)
	}

	// Update in-memory copy of the config.
	globalConfigInstance = cfg

	return nil
}

func validate(cfg config) error {
	// Network configuration
	if cfg.Network.RestServerPort < 0 {
		return fmt.Errorf(`Invalid config, "network.rest_server_port" can not be negative`)
	}

	if (cfg.Network.RestServerAddress != "" && cfg.Network.OperationsCenterAddress == "") ||
		(cfg.Network.RestServerAddress == "" && cfg.Network.OperationsCenterAddress != "") {
		return fmt.Errorf(`Invalid config, "network.address" and "network.rest_server_address" either both are set or both are unset`)
	}

	if cfg.Network.OperationsCenterAddress != "" {
		_, err := url.Parse(cfg.Network.OperationsCenterAddress)
		if err != nil {
			return fmt.Errorf(`Invalid config, "network.address" property is expected to be a valid URL: %w`, err)
		}
	}

	// Updates configuration
	if cfg.Updates.Source != "" {
		_, err := url.Parse(cfg.Updates.Source)
		if err != nil {
			return fmt.Errorf(`Invalid config, "updates.source" property is expected to be a valid URL: %w`, err)
		}
	}

	if cfg.Updates.SignatureVerificationRootCA == "" {
		return fmt.Errorf(`Invalid config, "updates.signature_verification_root_ca" can not be empty`)
	}

	pemBlock, _ := pem.Decode([]byte(cfg.Updates.SignatureVerificationRootCA))
	if pemBlock == nil {
		return fmt.Errorf(`Invalid config, pem decode for "updates.signature_verification_root_ca" failed`)
	}

	if cfg.Updates.FilterExpression != "" {
		_, err := expr.Compile(cfg.Updates.FilterExpression, expr.Env(provisioning.Update{}))
		if err != nil {
			return fmt.Errorf(`Invalid config, failed to compile filter expression: %v`, err)
		}
	}

	if cfg.Updates.FileFilterExpression != "" {
		_, err := expr.Compile(cfg.Updates.FileFilterExpression, expr.Env(provisioning.UpdateFile{}))
		if err != nil {
			return fmt.Errorf(`Invalid config, failed to compile file filter expression: %v`, err)
		}
	}

	// Security configuration
	if cfg.Security.OIDC.Issuer != "" {
		_, err := url.Parse(cfg.Security.OIDC.Issuer)
		if err != nil {
			return fmt.Errorf(`Invalid config, "security.oidc.issuer" property is expected to be a valid URL: %w`, err)
		}
	}

	if cfg.Security.OpenFGA.APIURL != "" {
		_, err := url.Parse(cfg.Security.OpenFGA.APIURL)
		if err != nil {
			return fmt.Errorf(`Invalid config, "security.openfga.api_url" property is expected to be a valid URL: %w`, err)
		}
	}

	return nil
}
