package acme

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

type acme struct {
	parent *os.Root
}

var _ system.ACMEPort = &acme{}

// New returns an adapter, that requests certificates from an ACME server.
// The adapter stores the ACME account data in the directory "acme" of parent.
// It creates this directory when it needs it.
func New(parent *os.Root) *acme {
	return &acme{
		parent: parent,
	}
}

// RequestCertificate requests a new certificate and key for the configured domain.
func (a *acme) RequestCertificate(ctx context.Context, cfg apisystem.SecurityACME) (*apisystem.CertificatePost, error) {
	caURL, err := url.ParseRequestURI(cfg.CAURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse CA URL %q: %w", cfg.CAURL, err)
	}

	root, err := file.MkdirAllOpenSubRoot(a.parent, "acme", 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to open acme directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	// Remove unrelated directories when done.
	defer cleanupAccounts(root, caURL.Hostname(), cfg.Email)

	certBytes, keyBytes, err := tls.RunACMEChallenge(ctx, root.Name(), cfg.CAURL, cfg.Domain, cfg.Email, string(cfg.Challenge), cfg.Provider, cfg.Address, "", "", "", cfg.ProviderResolvers, cfg.ProviderEnvironment)
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
