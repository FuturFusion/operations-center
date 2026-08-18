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

	"github.com/FuturFusion/operations-center/internal/domain"
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

type TokenImage struct {
	// Content is the image itself. It is the caller's responsibility to close it.
	Content  io.ReadSeekCloser
	Size     int64
	ModTime  time.Time
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
