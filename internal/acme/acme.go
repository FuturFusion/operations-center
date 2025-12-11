package acme

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxc/incus/v6/shared/tls"
	"github.com/lxc/incus/v6/shared/validate"

	"github.com/FuturFusion/operations-center/shared/api"
)

// ValidateACMEConfig validates the system ACME configuration. Assumes SetDefaults has been called.
func ValidateACMEConfig(s api.SystemSecurityACME) error {
	// Skip validation, if config is not initialized.
	if s.Domain == "" && s.Email == "" && s.CAURL == "" && !s.AgreeTOS {
		return nil
	}

	err := validate.IsListenAddress(true, true, false)(s.Address)
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

// ACMEConfigChanged returns whether the new config has changed from the old one.
func ACMEConfigChanged(oldCfg, newCfg api.SystemSecurityACME) bool {
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

type environment interface {
	VarDir() string
	CacheDir() string
}

// UpdateCertificate updates the certificate.
func UpdateCertificate(ctx context.Context, fsEnv environment, cfg api.SystemSecurityACME, force bool) (*api.SystemCertificatePost, error) {
	log := slog.With(slog.String("domain", cfg.Domain), slog.String("caURL", cfg.CAURL), slog.String("challenge", string(cfg.Challenge)))
	if cfg.Domain == "" || cfg.Email == "" || cfg.CAURL == "" || !cfg.AgreeTOS {
		return nil, nil
	}

	// Load the certificate.
	certInfo, err := tls.KeyPairAndCA(fsEnv.VarDir(), "server", tls.CertServer, true)
	if err != nil {
		return nil, fmt.Errorf("Failed to load certificate and key file: %w", err)
	}

	cert, err := certInfo.PublicKeyX509()
	if err != nil {
		return nil, fmt.Errorf("Failed to parse certificate: %w", err)
	}

	if !force && !tls.CertificateNeedsUpdate(cfg.Domain, cert, 30*24*time.Hour) {
		log.Debug("Skipping renewal for certificate that is valid for more than 30 days")
		return nil, nil
	}

	dir := filepath.Join(fsEnv.CacheDir(), "acme")
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("Failed to create acme account path %q: %w", dir, err)
	}

	caURL, err := url.ParseRequestURI(cfg.CAURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse CA URL %q: %q", cfg.CAURL, err)
	}

	// Remove unrelated directories when done.
	defer func() {
		_ = os.RemoveAll(filepath.Join(dir, "certificates"))
		accountPath := filepath.Join(dir, "accounts")

		entries, _ := os.ReadDir(accountPath)
		for _, e := range entries {
			if e.Name() != caURL.Hostname() {
				// Remove other CA URL paths if the config changed.
				_ = os.RemoveAll(filepath.Join(accountPath, e.Name()))
			} else {
				entries, _ := os.ReadDir(filepath.Join(accountPath, caURL.Hostname()))
				for _, e := range entries {
					// Remove other email paths if the config changed.
					if e.Name() != cfg.Email {
						_ = os.RemoveAll(filepath.Join(accountPath, caURL.Hostname(), e.Name()))
					}
				}
			}
		}
	}()

	certBytes, keyBytes, err := tls.RunACMEChallenge(ctx, dir, cfg.CAURL, cfg.Domain, cfg.Email, string(cfg.Challenge), cfg.Provider, cfg.Address, "", cfg.ProviderResolvers, cfg.ProviderEnvironment)
	if err != nil {
		return nil, err
	}

	return &api.SystemCertificatePost{
		Certificate: string(certBytes),
		Key:         string(keyBytes),
	}, nil
}
