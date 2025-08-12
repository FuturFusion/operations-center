package provisioning

import (
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
}

func (t Token) Validate() error {
	if t.UsesRemaining < 0 {
		return domain.NewValidationErrf(`Value for "uses remaining" can not be negative`)
	}

	if t.ExpireAt.Before(time.Now()) {
		return domain.NewValidationErrf(`Value for "expire at" can not be in the past`)
	}

	return nil
}

type Tokens []Token

type TokenSeedConfig struct {
	ImageType    api.ImageType
	Applications map[string]any
	Network      map[string]any
	Install      map[string]any
}

func (t TokenSeedConfig) Validate() error {
	var imageType api.ImageType
	err := imageType.UnmarshalText([]byte(t.ImageType))
	if t.ImageType == "" || err != nil {
		return domain.NewValidationErrf("Invalid token seed configuration, validation of image type failed: %v", err)
	}

	return nil
}
