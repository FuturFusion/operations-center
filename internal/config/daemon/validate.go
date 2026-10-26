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
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// validate performs the validation the config package owns itself. It is a pure
// function: it reads no global state, takes no lock and performs no I/O.
func validate(oldLocalCfg, newLocalCfg localConfig, isIncusOS bool, clustered bool) error {
	// Network configuration
	err := validateNetworkConfig(oldLocalCfg.Network, newLocalCfg.Network, isIncusOS, clustered)
	if err != nil {
		return err
	}

	// Settings configuration
	err = logger.ValidateLevel(newLocalCfg.Settings.LogLevel)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.log_level" property is invalid: %v`, err)
	}

	err = logger.ValidateComponentLevels(newLocalCfg.Settings.LogLevels)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.log_levels" property is invalid: %v`, err)
	}

	err = metrics.ValidateComponents(newLocalCfg.Settings.MetricsComponents)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.metrics_components" property is invalid: %v`, err)
	}

	return nil
}

func validateShared(oldGlobalCfg, newGlobalCfg system.SharedConfig, isIncusOS bool) error {
	// Updates configuration
	err := validateURI(newGlobalCfg.Updates.Source, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "updates.source" property is expected to be a valid source URL: %v`, err)
	}

	if newGlobalCfg.Updates.SignatureVerificationRootCA == "" {
		return domain.NewValidationErrf(`Invalid config, "updates.signature_verification_root_ca" can not be empty`)
	}

	_, err = certificate.Decode([]byte(newGlobalCfg.Updates.SignatureVerificationRootCA))
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, pem decode for "updates.signature_verification_root_ca" failed: %v`, err)
	}

	// Security configuration
	err = validateURI(newGlobalCfg.Security.OIDC.Issuer, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.oidc.issuer" property is expected to be a valid issuer URL: %v`, err)
	}

	err = validateURI(newGlobalCfg.Security.OpenFGA.APIURL, false, false, true)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.openfga.api_url" property is expected to be a valid URL: %v`, err)
	}

	err = acme.ValidateACMEConfig(newGlobalCfg.Security.ACME)
	if err != nil {
		return err
	}

	_, err = securitytls.CertificateFingerprints(newGlobalCfg.Security.TrustedTLSClientCertificates)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "security.trusted_tls_client_certificates" contains an invalid certificate: %v`, err)
	}

	for _, p := range newGlobalCfg.Security.TrustedHTTPSProxies {
		if net.ParseIP(p) == nil {
			return fmt.Errorf("HTTPS Proxy address %q is not a valid IP", p)
		}
	}

	// Updating the configuration requires at least one certificate fingerprint or
	// trusted client certificate to be present in order to have a fallback
	// authentication method.
	hasNoTrustedTLSClients := len(newGlobalCfg.Security.TrustedTLSClientCertFingerprints) == 0 && len(newGlobalCfg.Security.TrustedTLSClientCertificates) == 0
	if isIncusOS && isTrustedTLSClientsChanged(oldGlobalCfg, newGlobalCfg) && hasNoTrustedTLSClients {
		return domain.NewValidationErrf(`Invalid config, "security.trusted_tls_client_cert_fingerprints" and "security.trusted_tls_client_certificates" properties can not both be empty when running on IncusOS`)
	}

	return nil
}

// ValidateNetworkConfig validates a candidate network configuration against the
// currently committed one, without changing anything.
func ValidateNetworkConfig(cfg system.Network) error {
	local := defaultStore.Load()

	return validateNetworkConfig(local.Get().Network, cfg, local.env.IsIncusOS(), local.env.Clustered())
}

func ValidateSharedNetworkConfig(localCfg system.Network, sharedCfg system.NetworkGlobal) error {
	local := defaultStore.Load()
	shared := sharedStore.Load()
	return validateSharedNetworkConfig(localCfg, shared.Get().Network, sharedCfg, local.env.Clustered())
}

func ValidateClusterAddress(address string) error {
	if address == "" {
		return domain.NewValidationErrf("Cluster address can not be empty")
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	if port == "" {
		return domain.NewValidationErrf("Cluster address must specify a port")
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 0xffff {
		return domain.NewValidationErrf("Cluster address port %q must be a number between 1 and 65535", port)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return domain.NewValidationErrf("Cluster address must specify an IP address")
	}

	if !ip.IsGlobalUnicast() {
		return domain.NewValidationErrf("Cluster address IP must be a global unicast IP")
	}

	return nil
}

func validateNetworkConfigClusterAddress(oldCfg, newCfg system.Network, clustered bool) error {
	isClusterAddressChanged := oldCfg.ClusterAddress != "" && oldCfg.ClusterAddress != newCfg.ClusterAddress
	if isClusterAddressChanged && clustered {
		return domain.NewValidationErrf(`Invalid config, "network.cluster_address" is required for cluster connectivity and can not be changed`)
	}

	if newCfg.ClusterAddress != "" {
		err := ValidateClusterAddress(newCfg.ClusterAddress)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, "network.cluster_address" is invalid: %v`, err)
		}
	}

	return nil
}

func validateNetworkConfig(oldLocalCfg, newLocalCfg system.Network, isIncusOS bool, clustered bool) error {
	isRestServerAddressChanged := oldLocalCfg.RestServerAddress != newLocalCfg.RestServerAddress
	if isIncusOS && isRestServerAddressChanged && newLocalCfg.RestServerAddress == "" {
		return domain.NewValidationErrf(`Invalid config, "network.rest_server_address" can not be empty when running on IncusOS`)
	}

	err := validateNetworkConfigClusterAddress(oldLocalCfg, newLocalCfg, clustered)
	if err != nil {
		return err
	}

	if newLocalCfg.RestServerAddress != "" {
		host, portStr, err := net.SplitHostPort(newLocalCfg.RestServerAddress)
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

	return nil
}

func validateSharedNetworkConfig(currentLocalCfg system.Network, _, newGlobalCfg system.NetworkGlobal, clustered bool) error {
	// Listening on the network requires the address Operations Center is reached by.
	if newGlobalCfg.OperationsCenterAddress == "" && (clustered || currentLocalCfg.RestServerAddress != "") {
		return domain.NewValidationErrf(`Invalid config, "network.address" must be set if Operations Center is clustered or "network.rest_server_address" is set`)
	}

	err := validateURI(newGlobalCfg.OperationsCenterAddress, false, false, true)
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
