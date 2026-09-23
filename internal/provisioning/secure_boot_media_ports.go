package provisioning

import (
	"context"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
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
// which enrolls the certificates of IncusOS on a server, whose BMC can not
// modify the UEFI key databases itself.
type SecureBootMediaPort interface {
	// Generate builds the enrollment media for the certificates and returns the
	// ID addressing it.
	Generate(ctx context.Context, certificates SecureBootCertificates) (string, error)

	// Open returns the already generated enrollment media addressed by id.
	Open(ctx context.Context, id string) (*SecureBootMediaImage, error)

	// Prune removes the enrollment media, that has not been accessed for ttl.
	Prune(ctx context.Context, ttl time.Duration) error
}
