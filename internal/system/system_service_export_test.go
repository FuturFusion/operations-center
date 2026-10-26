package system

import (
	"context"
	"crypto/x509"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func WithACMEUpdateCertificateFunc(acmeUpdateCertificateFunc func(
	ctx context.Context,
	fsEnv interface {
		CacheDir() string
	},
	cfg system.SecurityACME,
	current *x509.Certificate,
	force bool,
) (*system.CertificatePost, error),
) SystemServiceOption {
	return func(s *systemService) {
		s.acmeUpdateCertificateFunc = acmeUpdateCertificateFunc
	}
}
