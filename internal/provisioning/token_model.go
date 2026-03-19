package provisioning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

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
