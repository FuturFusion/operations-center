package config

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/maniartech/signals"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type config struct {
	Network api.SystemNetwork `json:"network" yaml:"network"`

	Security api.SystemSecurity `json:"security" yaml:"security"`

	Updates api.SystemUpdates `json:"updates" yaml:"updates"`
}

type enver interface {
	VarDir() string
	IsIncusOS() bool
}

// Global variables to hold the config singleton.
var (
	globalConfigInstanceMu sync.Mutex
	globalConfigInstance   config

	saveFunc = saveToDisk

	env enver = environment.New(ApplicationName, ApplicationEnvPrefix)

	NetworkUpdateSignal  = signals.NewSync[api.SystemNetwork]()
	SecurityUpdateSignal = signals.NewSync[api.SystemSecurity]()
	UpdatesUpdateSignal  = signals.NewSync[api.SystemUpdates]()
)

func Init(vardir enver) error {
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

	cfg.Network.SystemNetworkPut, err = NetworkSetDefaults(cfg.Network.SystemNetworkPut)
	if err != nil {
		return fmt.Errorf("Invalid network config: %w", err)
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

	var err error

	newCfg := globalConfigInstance
	newCfg.Network.SystemNetworkPut, err = NetworkSetDefaults(cfg)
	if err != nil {
		return err
	}

	err = validateAndSave(newCfg)
	if err != nil {
		return err
	}

	NetworkUpdateSignal.Emit(ctx, api.SystemNetwork{
		SystemNetworkPut: cfg,
	})

	return nil
}

func NetworkSetDefaults(cfg api.SystemNetworkPut) (api.SystemNetworkPut, error) {
	newCfg := cfg
	parseIP := func(addr string) (net.IP, error) {
		if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") && len(addr) > 2 {
			addr = addr[1 : len(addr)-1]
		}

		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, fmt.Errorf("%q is not a valid IP address", addr)
		}

		return ip, nil
	}

	if cfg.RestServerAddress != "" {
		host, port, err := net.SplitHostPort(cfg.RestServerAddress)
		if err != nil {
			ip, err := parseIP(cfg.RestServerAddress)
			if err != nil {
				return api.SystemNetworkPut{}, err
			}

			newCfg.RestServerAddress = net.JoinHostPort(ip.String(), DefaultRestServerPort)
			return newCfg, nil
		}

		if host == "" {
			host = "::"
		}

		_, err = parseIP(host)
		if err != nil {
			return api.SystemNetworkPut{}, err
		}

		if port == "" {
			port = DefaultRestServerPort
		}

		newCfg.RestServerAddress = net.JoinHostPort(host, port)
	}

	return newCfg, nil
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
	if cfg.Network.RestServerAddress != "" {
		host, portStr, err := net.SplitHostPort(cfg.Network.RestServerAddress)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" is not a valid address: %v`, err)
		}

		if host != "" {
			ip := net.ParseIP(host)
			if ip == nil {
				return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" does not contain a valid ip`)
			}
		}

		if portStr != "" {
			port, err := strconv.ParseInt(portStr, 10, 64)
			if err != nil {
				return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" does not contain a valid port`)
			}

			if port < 1 || port > 0xffff {
				return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" port out of range (%d - %d)`, 1, 0xffff)
			}
		}
	}

	if (cfg.Network.RestServerAddress != "" && cfg.Network.OperationsCenterAddress == "") ||
		(cfg.Network.RestServerAddress == "" && cfg.Network.OperationsCenterAddress != "") {
		return domain.NewValidationErrf(`Invalid config, "network.address" and "network.rest_server_address" either both are set or both are unset`)
	}

	if cfg.Network.OperationsCenterAddress != "" {
		_, err := url.Parse(cfg.Network.OperationsCenterAddress)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "network.address" property is expected to be a valid URL: %v`, err)
		}
	}

	// Updates configuration
	if cfg.Updates.Source != "" {
		_, err := url.Parse(cfg.Updates.Source)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "updates.source" property is expected to be a valid URL: %v`, err)
		}
	}

	if cfg.Updates.SignatureVerificationRootCA == "" {
		return domain.NewValidationErrf(`Invalid config, "updates.signature_verification_root_ca" can not be empty`)
	}

	pemBlock, _ := pem.Decode([]byte(cfg.Updates.SignatureVerificationRootCA))
	if pemBlock == nil {
		return domain.NewValidationErrf(`Invalid config, pem decode for "updates.signature_verification_root_ca" failed`)
	}

	if cfg.Updates.FilterExpression != "" {
		_, err := expr.Compile(cfg.Updates.FilterExpression, expr.Env(provisioning.Update{}))
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, failed to compile filter expression: %v`, err)
		}
	}

	if cfg.Updates.FileFilterExpression != "" {
		_, err := expr.Compile(cfg.Updates.FileFilterExpression, expr.Env(provisioning.UpdateFile{}))
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, failed to compile file filter expression: %v`, err)
		}
	}

	// Security configuration
	if cfg.Security.OIDC.Issuer != "" {
		_, err := url.Parse(cfg.Security.OIDC.Issuer)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "security.oidc.issuer" property is expected to be a valid URL: %v`, err)
		}
	}

	if cfg.Security.OpenFGA.APIURL != "" {
		_, err := url.Parse(cfg.Security.OpenFGA.APIURL)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "security.openfga.api_url" property is expected to be a valid URL: %v`, err)
		}
	}

	// Updating the configuration requires at least one certificate fingerprint to be present in order to have a fallback authentication method.
	isTrustedTLSClientCertFingerprintsUpdated := !slices.Equal(globalConfigInstance.Security.TrustedTLSClientCertFingerprints, cfg.Security.TrustedTLSClientCertFingerprints)
	if env.IsIncusOS() && isTrustedTLSClientCertFingerprintsUpdated && len(cfg.Security.TrustedTLSClientCertFingerprints) == 0 {
		return domain.NewValidationErrf(`Invalid config, "security.trusted_tls_client_cert_fingerprints" property can not be empty when running on IncusOS`)
	}

	return nil
}
