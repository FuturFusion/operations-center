package provisioning

import (
	"context"
	"io"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
)

// SecureBootCertificateSourcePort provides the secure boot certificates of
// IncusOS, which are the ones a server has to trust to boot it.
type SecureBootCertificateSourcePort interface {
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
}

// SecureBootCertificateCatalogPort provides the certificate material of the
// well known UEFI certificates, that Operations Center ships, so an entry, that
// is only allow listed by fingerprint, can be enrolled again after the key
// databases of a server have been wiped.
type SecureBootCertificateCatalogPort interface {
	// CertificatesByFingerprint resolves lower case hex encoded SHA256
	// fingerprints of the DER encoding of a certificate to its PEM encoding. It
	// returns the fingerprints, that are not part of the catalog, in the order
	// they have been asked for.
	CertificatesByFingerprint(fingerprints []string) (certificates []string, unknown []string)
}

// SecureBootMediaPort generates and stores the secure boot enrollment media,
// which enrolls the certificates of IncusOS on a server, whose BMC can not
// modify the UEFI key databases itself.
type SecureBootMediaPort interface {
	// CheckSupported reports, whether the enrollment media can be generated for
	// architecture at all, so a deployment, which would fail to generate it, is
	// rejected before it touches the server.
	CheckSupported(ctx context.Context, architecture images.UpdateFileArchitecture) error

	// Generate builds the enrollment media, that boots on architecture and
	// enrolls the certificates, and returns the ID addressing it. Generating the
	// very same media twice returns the same ID and does the work only once.
	Generate(ctx context.Context, architecture images.UpdateFileArchitecture, certificates SecureBootCertificates) (string, error)

	// Open returns the already generated enrollment media addressed by id.
	Open(ctx context.Context, id string) (*SecureBootMediaImage, error)

	// Prune removes the enrollment media, that has not been accessed for ttl.
	Prune(ctx context.Context, ttl time.Duration) error
}

// SecureBootCertificates holds the PEM encoded certificates to be enrolled into
// the UEFI key databases of a server, in the order they are enrolled in.
type SecureBootCertificates struct {
	PK  string
	KEK []string
	DB  []string
	DBX []string
}

func (c SecureBootCertificates) IsEmpty() bool {
	return c.PK == "" && len(c.KEK) == 0 && len(c.DB) == 0 && len(c.DBX) == 0
}

// SecureBootMediaImage is one generated secure boot enrollment media, ready to
// be served.
type SecureBootMediaImage struct {
	// Content holds the image itself. It is the callers duty to close it.
	Content  io.ReadSeekCloser
	Filename string
	Size     int64
	ModTime  time.Time
}
