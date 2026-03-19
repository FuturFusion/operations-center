package system

import (
	"context"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func WithACMEUpdateCertificateFunc(acmeUpdateCertificateFunc func(
	ctx context.Context,
	fsEnv interface {
		VarDir() string
		CacheDir() string
	},
	cfg system.SecurityACME,
	force bool,
) (*system.CertificatePost, error),
) SystemServiceOption {
	return func(s *systemService) {
		s.acmeUpdateCertificateFunc = acmeUpdateCertificateFunc
	}
}
