package provisioning

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/shared/api"
)

var ExpireAtInfinity = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)

const UsesRemainingInfinity = math.MaxInt

type Token struct {
	ID            int64
	UUID          uuid.UUID `db:"primary=yes"`
	UsesRemaining int
	ExpireAt      time.Time
	Description   string
	Channel       string `db:"join=channels.name"`
	AutoRemove    bool
}

func (t Token) Validate() error {
	if t.UsesRemaining < 0 {
		return domain.NewValidationErrf(`Value for "uses remaining" can not be negative`)
	}

	if t.ExpireAt.Before(time.Now()) {
		return domain.NewValidationErrf(`Value for "expire at" can not be in the past`)
	}

	if t.Channel == "" {
		return domain.NewValidationErrf(`Channel can not be empty`)
	}

	return nil
}

type Tokens []Token

type TokenImageSeedConfigs struct {
	Applications     api.SeedApplications     `json:"applications"`
	Incus            api.SeedIncus            `json:"incus"`
	Install          api.SeedInstall          `json:"install"`
	MigrationManager api.SeedMigrationManager `json:"migration_manager"`
	Network          api.SeedNetwork          `json:"network"`
	OperationsCenter api.SeedOperationsCenter `json:"operations_center"`
	Security         api.SeedSecurity         `json:"security"`
	Update           api.SeedUpdate           `json:"update"`
}

// ApplyTrustedClientCertificates passes the given X509 PEM encoded client
// certificates trusted by Operations Center on to the deployed system, so
// whoever has access to Operations Center also has access to what Operations
// Center deploys.
//
// The Operations Center and Migration Manager seeds are only populated if the
// user did not provide their own set of trusted client certificates for them.
// The Incus preseed is exclusively managed by Operations Center.
func (t *TokenImageSeedConfigs) ApplyTrustedClientCertificates(certificatesPEM []string) error {
	if len(certificatesPEM) == 0 {
		return nil
	}

	if t.Incus.Preseed == nil {
		t.Incus.Preseed = &incusapi.InitPreseed{}
	}

	if len(t.Incus.Preseed.Certificates) == 0 {
		certificates, err := securitytls.TrustedClientCertificates(certificatesPEM)
		if err != nil {
			return fmt.Errorf("Failed to derive Incus certificates from the trusted client certificates: %w", err)
		}

		t.Incus.Preseed.Certificates = certificates
	}

	if len(t.OperationsCenter.TrustedClientCertificates) == 0 {
		t.OperationsCenter.TrustedClientCertificates = certificatesPEM
	}

	if len(t.MigrationManager.TrustedClientCertificates) == 0 {
		t.MigrationManager.TrustedClientCertificates = certificatesPEM
	}

	return nil
}

// Value implements the sql driver.Valuer interface.
func (t TokenImageSeedConfigs) Value() (driver.Value, error) {
	return json.Marshal(t)
}

// Scan implements the sql.Scanner interface.
func (t *TokenImageSeedConfigs) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid token seeds")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*t = TokenImageSeedConfigs{}
			return nil
		}

		return json.Unmarshal([]byte(v), t)

	case []byte:
		if len(v) == 0 {
			*t = TokenImageSeedConfigs{}
			return nil
		}

		return json.Unmarshal(v, t)

	default:
		return fmt.Errorf("type %T is not supported for token seeds", value)
	}
}

type TokenSeed struct {
	ID          int64
	Token       uuid.UUID `db:"primary=yes&join=tokens.uuid"`
	Name        string    `db:"primary=yes"`
	Description string
	Public      bool
	Seeds       TokenImageSeedConfigs
	LastUpdated time.Time `db:"update_timestamp"`
}

func (t TokenSeed) Validate() error {
	if t.Name == "" {
		return domain.NewValidationErrf("Invalid token seed, name can not be empty")
	}

	return nil
}

type TokenSeeds []TokenSeed

// SeedImageInfo describes a generated seed image, as far as serving it and
// following the progress of a reader require.
type SeedImageInfo struct {
	Size    int64
	ModTime time.Time
}

type TokenImage struct {
	// Content is the image itself. It is the caller's responsibility to close it.
	SeedImageInfo
	Content  io.ReadSeekCloser
	Filename string
}

const SeedImageCacheIDLength = 12

// SeedImageCacheID returns the cache ID for a pre-seeded image.
func SeedImageCacheID(id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		id.String(),
		name,
		imageType.String(),
		architecture.String(),
		channel,
	}, "\x00")))

	return base64.RawURLEncoding.EncodeToString(sum[:])[:SeedImageCacheIDLength]
}

// SeedImageFingerprintID returns the ID of a pre-seeded image generated from
// the given fingerprint.
//
// Together with the cache ID it addresses one generated image. It is the final
// path segment of the URL a prepared image is served under.
func SeedImageFingerprintID(fingerprint string) string {
	sum := sha256.Sum256([]byte(fingerprint))

	return base64.RawURLEncoding.EncodeToString(sum[:])[:SeedImageCacheIDLength]
}
