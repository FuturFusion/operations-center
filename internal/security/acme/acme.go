package acme

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxc/incus/v7/shared/tls"
	"github.com/lxc/incus/v7/shared/validate"

	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// ValidateACMEConfig validates the system ACME configuration. Assumes SetDefaults has been called.
func ValidateACMEConfig(s system.SecurityACME) error {
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

	// The host and the email are used as directory names for the ACME account.
	if !file.IsPathElement(u.Hostname()) {
		return fmt.Errorf("ACME CA URL host %q is invalid", u.Hostname())
	}

	if s.Email != "" {
		addr, err := mail.ParseAddress(s.Email)
		if err != nil || addr.Address != s.Email || !file.IsPathElement(s.Email) {
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

// Matches the threshold of github.com/lxc/incus/v7/shared/tls.CertificateNeedsUpdate.
const renewalLifetimeRatio = 0.8

// IsEnabled returns whether the ACME config is complete, so the server
// certificate is managed through ACME.
func IsEnabled(cfg system.SecurityACME) bool {
	return cfg.Domain != "" && cfg.Email != "" && cfg.CAURL != "" && cfg.AgreeTOS
}

// ACMEConfigChanged returns whether the new config has changed from the old one.
func ACMEConfigChanged(oldCfg, newCfg system.SecurityACME) bool {
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

// UpdateCertificate updates the certificate.
func UpdateCertificate(
	ctx context.Context,
	fsEnv interface {
		VarDir() string
		CacheDir() string
	},
	cfg system.SecurityACME,
	force bool,
) (*system.CertificatePost, error) {
	log := slog.With(slog.String("domain", cfg.Domain), slog.String("ca_url", cfg.CAURL), slog.String("challenge", string(cfg.Challenge)))
	if !IsEnabled(cfg) {
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

	if !force && !tls.CertificateNeedsUpdate(cfg.Domain, cert) {
		renewalAfter := cert.NotBefore.Add(time.Duration(float64(cert.NotAfter.Sub(cert.NotBefore)) * renewalLifetimeRatio))
		log.InfoContext(ctx, "Skipping renewal for certificate that is not due for renewal", slog.Time("renewal_after", renewalAfter), slog.Time("not_after", cert.NotAfter))
		return nil, nil
	}

	dir := filepath.Join(fsEnv.CacheDir(), "acme")
	root, err := file.MkdirAllOpenRoot(dir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("Failed to create acme account path %q: %w", dir, err)
	}

	defer root.Close()

	caURL, err := url.ParseRequestURI(cfg.CAURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse CA URL %q: %q", cfg.CAURL, err)
	}

	// Remove unrelated directories when done.
	defer cleanupAccounts(root, caURL.Hostname(), cfg.Email)

	certBytes, keyBytes, err := tls.RunACMEChallenge(ctx, dir, cfg.CAURL, cfg.Domain, cfg.Email, string(cfg.Challenge), cfg.Provider, cfg.Address, "", "", "", cfg.ProviderResolvers, cfg.ProviderEnvironment)
	if err != nil {
		return nil, err
	}

	return &system.CertificatePost{
		Certificate: string(certBytes),
		Key:         string(keyBytes),
	}, nil
}

// cleanupAccounts removes the certificates directory from root. It also removes
// the accounts of other CAs and other emails. It ignores errors. The next
// renewal repeats the cleanup.
func cleanupAccounts(root *os.Root, caHostname string, email string) {
	_ = root.RemoveAll("certificates")
	accountPath := "accounts"

	entries, _ := fs.ReadDir(root.FS(), accountPath)
	for _, e := range entries {
		if e.Name() != caHostname {
			// Remove other CA URL paths if the config changed.
			_ = root.RemoveAll(filepath.Join(accountPath, e.Name()))
			continue
		}

		entries, _ := fs.ReadDir(root.FS(), filepath.Join(accountPath, caHostname))
		for _, e := range entries {
			// Remove other email paths if the config changed.
			if e.Name() != email {
				_ = root.RemoveAll(filepath.Join(accountPath, caHostname, e.Name()))
			}
		}
	}
}
