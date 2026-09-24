package provisioning

import (
	"context"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/shared/api"
)

// SecureBootCertificateSourcePort provides the secure boot certificates of
// IncusOS, which are the ones a server has to trust to boot it.
type SecureBootCertificateSourcePort interface {
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
	GetSecureBootPlatformKeyUpdate(ctx context.Context) ([]byte, error)
}

// SecureBootCertificateCatalogPort provides the certificate material of the
// well known UEFI certificates, that Operations Center ships.
type SecureBootCertificateCatalogPort interface {
	CertificatesByFingerprint(fingerprints []string) (certificates []string, unknown []string)
}

// SecureBootMediaPort generates and stores the secure boot enrollment media,
// which enrolls the certificates of IncusOS on a server.
type SecureBootMediaPort interface {
	CheckSupported(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture) error
	Generate(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture, certificates SecureBootCertificates) (string, error)
	Open(ctx context.Context, imageType api.ImageType, id string) (*SecureBootMediaImage, error)
	Prune(ctx context.Context, ttl time.Duration, inUse []string) error
}
