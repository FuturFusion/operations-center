package provisioning

import (
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
)

//
//generate-expr: Channel

type Channel struct {
	ID          int64     `json:"-"`
	Name        string    `json:"name" db:"primary=yes"`
	Description string    `json:"description"`
	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

type ChannelFilter struct {
	ID   *int
	Name *string
}

type ChannelUpdate struct {
	ChannelID int `db:"primary=yes"`
	UpdateID  int `db:"primary=yes"`
}

type ChannelUpdateFilter struct {
	ChannelID *int
	UpdateID  *int
}

func (u Channel) Validate() error {
	if u.Name == "" {
		return domain.NewValidationErrf("Invalid channel, validation of name failed: name must not be empty")
	}

	return nil
}

type Channels []Channel
