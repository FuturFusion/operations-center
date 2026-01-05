package system

import (
	"context"

	"github.com/FuturFusion/operations-center/shared/api"
)

func WithACMEUpdateCertificateFunc(acmeUpdateCertificateFunc func(
	ctx context.Context,
	fsEnv interface {
		VarDir() string
		CacheDir() string
	},
	cfg api.SystemSecurityACME,
	force bool,
) (*api.SystemCertificatePost, error),
) SystemServiceOption {
	return func(s *systemService) {
		s.acmeUpdateCertificateFunc = acmeUpdateCertificateFunc
	}
}
