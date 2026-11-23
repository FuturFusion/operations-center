package environment

import (
	"context"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
)

type Environment interface {
	LogDir() string
	RunDir() string
	VarDir() string
	CacheDir() string
	UsrShareDir() string
	GetUnixSocket() string
	UserConfigDir() (string, error)
	IsIncusOS() bool
	GetToken(ctx context.Context) (string, error)
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
	GetSecureBootPlatformKeyUpdate(ctx context.Context) ([]byte, error)
}
