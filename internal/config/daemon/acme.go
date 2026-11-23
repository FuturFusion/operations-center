package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"

	incusvalidate "github.com/lxc/incus/v7/shared/validate"

	"github.com/FuturFusion/operations-center/internal/util/pathutil"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// validateACMEConfig validates the system ACME configuration. Assumes SetDefaults has been called.
func validateACMEConfig(s system.SecurityACME) error {
	// Skip validation, if config is not initialized.
	if s.Domain == "" && s.Email == "" && s.CAURL == "" && !s.AgreeTOS {
		return nil
	}

	err := incusvalidate.IsListenAddress(true, true, false)(s.Address)
	if err != nil {
		return fmt.Errorf("Failed to parse ACME HTTP challenge address %q: %w", s.Address, err)
	}

	u, err := url.ParseRequestURI(s.CAURL)
	if err != nil {
		return fmt.Errorf("Failed to parse ACME CA URL %q: %w", s.CAURL, err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("Failed to determine scheme for ACME CA URL %q", s.CAURL)
	}

	if u.Hostname() == "" {
		return fmt.Errorf("Failed to determine host for ACME CA URL %q", s.CAURL)
	}

	// The host and the email are used as directory names for the ACME account.
	if !pathutil.IsPathElement(u.Hostname()) {
		return fmt.Errorf("ACME CA URL host %q is invalid", u.Hostname())
	}

	if s.Email != "" {
		_, err := mail.ParseAddress(s.Email)
		if err != nil || !pathutil.IsPathElement(s.Email) {
			return fmt.Errorf("ACME email %q is not a valid email address", s.Email)
		}
	}

	if u.Port() != "" {
		portInt, err := strconv.Atoi(u.Port())
		if err != nil {
			return fmt.Errorf("ACME CA URL port %q is invalid: %w", u.Port(), err)
		}

		if portInt < 1 || portInt > 0xffff {
			return fmt.Errorf("ACME CA URL port %d is invalid", portInt)
		}
	}

	for _, e := range s.ProviderEnvironment {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return fmt.Errorf("Invalid key=value format for provider environment: %q", e)
		}
	}

	return s.Challenge.Validate()
}

// IsACMEEnabled returns whether the ACME config is complete, so the server
// certificate is managed through ACME.
func IsACMEEnabled(cfg system.SecurityACME) bool {
	return cfg.Domain != "" && cfg.Email != "" && cfg.CAURL != "" && cfg.AgreeTOS
}

// acmeConfigChanged returns whether the new config has changed from the old one.
func acmeConfigChanged(oldCfg, newCfg system.SecurityACME) bool {
	if oldCfg.AgreeTOS != newCfg.AgreeTOS ||
		oldCfg.CAURL != newCfg.CAURL ||
		oldCfg.Challenge != newCfg.Challenge ||
		oldCfg.Domain != newCfg.Domain ||
		oldCfg.Email != newCfg.Email ||
		oldCfg.Address != newCfg.Address ||
		oldCfg.Provider != newCfg.Provider ||
		!slices.Equal(oldCfg.ProviderEnvironment, newCfg.ProviderEnvironment) ||
		!slices.Equal(oldCfg.ProviderResolvers, newCfg.ProviderResolvers) {
		return true
	}

	return false
}
