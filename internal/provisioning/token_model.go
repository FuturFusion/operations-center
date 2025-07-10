package provisioning

import (
	"math"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/FuturFusion/operations-center/internal/domain"
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
	Applications []string
	Network      incusosapi.SystemNetworkConfig
}
