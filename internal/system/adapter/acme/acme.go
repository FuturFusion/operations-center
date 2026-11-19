package acme

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/lxc/incus/v7/shared/tls"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/system"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

// Matches the threshold of github.com/lxc/incus/v7/shared/tls.CertificateNeedsUpdate.
const renewalLifetimeRatio = 0.8

type acme struct {
	env interface {
		VarDir() string
	}
	root *os.Root
}

var _ system.ACMEPort = &acme{}

// New returns an adapter, that requests certificates from an ACME server.
// The ACME account data is stored in root.
func New(env interface{ VarDir() string }, root *os.Root) *acme {
	return &acme{
		env:  env,
		root: root,
	}
}

// UpdateCertificate requests a new certificate, if ACME is enabled and the
// current certificate needs renewal or force is set.
func (a *acme) UpdateCertificate(ctx context.Context, cfg apisystem.SecurityACME, force bool) (*apisystem.CertificatePost, error) {
	log := slog.With(slog.String("domain", cfg.Domain), slog.String("ca_url", cfg.CAURL), slog.String("challenge", string(cfg.Challenge)))
	if !config.IsACMEEnabled(cfg) {
		return nil, nil
	}

	// Load the certificate.
	certInfo, err := tls.KeyPairAndCA(a.env.VarDir(), "server", tls.CertServer, true)
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

	caURL, err := url.ParseRequestURI(cfg.CAURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse CA URL %q: %q", cfg.CAURL, err)
	}

	// Remove unrelated directories when done.
	defer cleanupAccounts(a.root, caURL.Hostname(), cfg.Email)

	certBytes, keyBytes, err := tls.RunACMEChallenge(ctx, a.root.Name(), cfg.CAURL, cfg.Domain, cfg.Email, string(cfg.Challenge), cfg.Provider, cfg.Address, "", "", "", cfg.ProviderResolvers, cfg.ProviderEnvironment)
	if err != nil {
		return nil, err
	}

	return &apisystem.CertificatePost{
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
