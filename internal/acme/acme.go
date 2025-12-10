package acme

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxc/incus/v6/shared/subprocess"
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

// certificateNeedsUpdate returns true if the domain doesn't match the certificate's DNS names
// or it's valid for less than 30 days.
func certificateNeedsUpdate(domain string, cert *x509.Certificate) bool {
	return !slices.Contains(cert.DNSNames, domain) || time.Now().After(cert.NotAfter.Add(-30*24*time.Hour))
}

type environment interface {
	VarDir() string
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

	if !force && !certificateNeedsUpdate(cfg.Domain, cert) {
		log.Debug("Skipping renewal for certificate that is valid for more than 30 days")
		return nil, nil
	}

	tmpDir, err := os.MkdirTemp("", "lego")
	if err != nil {
		return nil, fmt.Errorf("Failed to create temporary directory: %w", err)
	}

	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			log.Warn("Failed to remove temporary directory", slog.Any("error", err))
		}
	}()

	env := os.Environ()

	args := []string{
		"--accept-tos",
		"--domains", cfg.Domain,
		"--email", cfg.Email,
		"--path", tmpDir,
		"--server", cfg.CAURL,
	}

	switch cfg.Challenge {
	case api.ACMEChallengeDNS:
		env = append(env, cfg.ProviderEnvironment...)
		if cfg.Provider == "" {
			return nil, fmt.Errorf("%q challenge type requires acme.dns.provider configuration key to be set", cfg.Challenge)
		}

		args = append(args, "--dns", cfg.Provider)
		if len(cfg.ProviderResolvers) > 0 {
			for _, resolver := range cfg.ProviderResolvers {
				args = append(args, "--dns.resolvers", resolver)
			}
		}

	case api.ACMEChallengeHTTP:
		args = append(args, "--http", "--http.port", cfg.Address)
	}

	args = append(args, "run")
	log.Debug("Initiating certificate renewal")
	_, _, err = subprocess.RunCommandSplit(ctx, env, nil, "lego", args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to run lego command: %w", err)
	}

	// Load the generated certificate.
	certData, err := os.ReadFile(filepath.Join(tmpDir, "certificates", fmt.Sprintf("%s.crt", cfg.Domain)))
	if err != nil {
		return nil, err
	}

	caData, err := os.ReadFile(filepath.Join(tmpDir, "certificates", fmt.Sprintf("%s.issuer.crt", cfg.Domain)))
	if err != nil {
		return nil, err
	}

	keyData, err := os.ReadFile(filepath.Join(tmpDir, "certificates", fmt.Sprintf("%s.key", cfg.Domain)))
	if err != nil {
		return nil, err
	}

	return &api.SystemCertificatePost{
		Certificate: string(append(certData, caData...)),
		Key:         string(keyData),
	}, nil
}
