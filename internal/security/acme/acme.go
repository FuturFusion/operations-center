package acme

import (
	"context"
	"crypto/x509"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

// Matches the threshold of github.com/lxc/incus/v7/shared/tls.CertificateNeedsUpdate.
const renewalLifetimeRatio = 0.8

// isEnabled mirrors config.IsACMEEnabled. This package is removed with the ACME port.
func isEnabled(cfg system.SecurityACME) bool {
	return cfg.Domain != "" && cfg.Email != "" && cfg.CAURL != "" && cfg.AgreeTOS
}

// UpdateCertificate requests a new certificate if current is missing, about to expire or does not match the configured domain.
// The ACME account data is stored in acmeRoot.
func UpdateCertificate(
	ctx context.Context,
	acmeRoot *os.Root,
	cfg system.SecurityACME,
	current *x509.Certificate,
	force bool,
) (*system.CertificatePost, error) {
	log := slog.With(slog.String("domain", cfg.Domain), slog.String("ca_url", cfg.CAURL), slog.String("challenge", string(cfg.Challenge)))
	if !isEnabled(cfg) {
		return nil, nil
	}

	if !force && current != nil && !tls.CertificateNeedsUpdate(cfg.Domain, current) {
		renewalAfter := current.NotBefore.Add(time.Duration(float64(current.NotAfter.Sub(current.NotBefore)) * renewalLifetimeRatio))
		log.InfoContext(ctx, "Skipping renewal for certificate that is not due for renewal", slog.Time("renewal_after", renewalAfter), slog.Time("not_after", current.NotAfter))
		return nil, nil
	}

	caURL, err := url.ParseRequestURI(cfg.CAURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse CA URL %q: %q", cfg.CAURL, err)
	}

	// Remove unrelated directories when done.
	defer cleanupAccounts(acmeRoot, caURL.Hostname(), cfg.Email)

	certBytes, keyBytes, err := tls.RunACMEChallenge(ctx, acmeRoot.Name(), cfg.CAURL, cfg.Domain, cfg.Email, string(cfg.Challenge), cfg.Provider, cfg.Address, "", "", "", cfg.ProviderResolvers, cfg.ProviderEnvironment)
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
