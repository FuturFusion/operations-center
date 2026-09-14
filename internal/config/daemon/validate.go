package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// validate performs the validation the config package owns itself. It is a pure
// function: it reads no global state, takes no lock and performs no I/O.
func validate(oldCfg, newCfg config, isIncusOS bool) error {
	// Network configuration
	err := validateNetworkConfig(oldCfg.Network, newCfg.Network, isIncusOS)
	if err != nil {
		return err
	}

	// Updates configuration
	err = validateURI(newCfg.Updates.Source, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "updates.source" property is expected to be a valid source URL: %v`, err)
	}

	if newCfg.Updates.SignatureVerificationRootCA == "" {
		return domain.NewValidationErrf(`Invalid config, "updates.signature_verification_root_ca" can not be empty`)
	}

	_, err = certificate.Decode([]byte(newCfg.Updates.SignatureVerificationRootCA))
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, pem decode for "updates.signature_verification_root_ca" failed: %v`, err)
	}

	// Security configuration
	err = validateURI(newCfg.Security.OIDC.Issuer, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.oidc.issuer" property is expected to be a valid issuer URL: %v`, err)
	}

	err = validateURI(newCfg.Security.OpenFGA.APIURL, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.openfga.api_url" property is expected to be a valid URL: %v`, err)
	}

	err = acme.ValidateACMEConfig(newCfg.Security.ACME)
	if err != nil {
		return err
	}

	_, err = securitytls.CertificateFingerprints(newCfg.Security.TrustedTLSClientCertificates)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.trusted_tls_client_certificates" contains an invalid certificate: %v`, err)
	}

	for _, p := range newCfg.Security.TrustedHTTPSProxies {
		if net.ParseIP(p) == nil {
			return fmt.Errorf("HTTPS Proxy address %q is not a valid IP", p)
		}
	}

	// Updating the configuration requires at least one certificate fingerprint or
	// trusted client certificate to be present in order to have a fallback
	// authentication method.
	hasNoTrustedTLSClients := len(newCfg.Security.TrustedTLSClientCertFingerprints) == 0 && len(newCfg.Security.TrustedTLSClientCertificates) == 0
	if isIncusOS && isTrustedTLSClientsChanged(oldCfg, newCfg) && hasNoTrustedTLSClients {
		return domain.NewValidationErrf(`Invalid config, "security.trusted_tls_client_cert_fingerprints" and "security.trusted_tls_client_certificates" properties can not both be empty when running on IncusOS`)
	}

	// Settings configuration
	err = logger.ValidateLevel(newCfg.Settings.LogLevel)
	if err != nil {
		return err
	}

	return nil
}

// ValidateNetworkConfig validates a candidate network configuration against the
// currently committed one, without changing anything.
func ValidateNetworkConfig(cfg system.Network) error {
	s := defaultStore.Load()

	return validateNetworkConfig(s.get().Network, cfg, s.env.IsIncusOS())
}

func validateNetworkConfig(oldCfg, newCfg system.Network, isIncusOS bool) error {
	isRestServerAddressChanged := oldCfg.RestServerAddress != newCfg.RestServerAddress
	if isIncusOS && isRestServerAddressChanged && newCfg.RestServerAddress == "" {
		return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" can not be empty when running on IncusOS`)
	}

	if newCfg.RestServerAddress != "" {
		host, portStr, err := net.SplitHostPort(newCfg.RestServerAddress)
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

	if (newCfg.RestServerAddress != "" && newCfg.OperationsCenterAddress == "") ||
		(newCfg.RestServerAddress == "" && newCfg.OperationsCenterAddress != "") {
		return domain.NewValidationErrf(`Invalid config, "network.address" and "network.rest_server_address" either both are set or both are unset`)
	}

	err := validateURI(newCfg.OperationsCenterAddress, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "network.address" property is expected to be a valid URL: %v`, err)
	}

	return nil
}

func validateURI(inURI string, required bool, enforceNoPath bool, enforceNoQuery bool) error {
	if required && inURI == "" {
		return fmt.Errorf("Required URI is empty")
	}

	if inURI != "" {
		endpoint, err := url.ParseRequestURI(inURI)
		if err != nil {
			return err
		}

		if endpoint.Scheme == "" {
			return fmt.Errorf("Failed to determine scheme")
		}

		if endpoint.Hostname() == "" {
			return fmt.Errorf("Failed to determine host")
		}

		if endpoint.Port() != "" {
			portInt, err := strconv.Atoi(endpoint.Port())
			if err != nil {
				return fmt.Errorf("Port %q is invalid: %w", endpoint.Port(), err)
			}

			if portInt < 1 || portInt > 0xffff {
				return fmt.Errorf("Port %d is invalid", portInt)
			}
		}

		if enforceNoPath && endpoint.Path != "" {
			return fmt.Errorf("Contains path")
		}

		if enforceNoQuery && endpoint.RawQuery != "" {
			return fmt.Errorf("Contains query")
		}

		if strings.Contains(inURI, "#") {
			return fmt.Errorf("Contains fragment")
		}

		if endpoint.User.Username() != "" {
			return fmt.Errorf("Contains username")
		}

		_, hasPassword := endpoint.User.Password()
		if hasPassword {
			return fmt.Errorf("Contains password")
		}
	}

	return nil
}
