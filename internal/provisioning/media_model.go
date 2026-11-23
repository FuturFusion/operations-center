package provisioning

import (
	"encoding/json"
	"time"
)

// MediaType tells the kinds of generated media apart.
type MediaType string

const (
	// MediaTypeSeedImage is a pre-seeded install image of a public token seed.
	MediaTypeSeedImage MediaType = "seed-image"

	// MediaTypeSecureBoot is a secure boot enrollment media.
	MediaTypeSecureBoot MediaType = "secure-boot"
)

// Media records a generated media, the cluster member storing it and until when it is kept.
type Media struct {
	ID       int64
	Type     MediaType `db:"primary=yes"`
	Name     string    `db:"primary=yes"`
	MemberID int64

	// Config holds the details of the media, its content depends on Type.
	Config json.RawMessage `db:"marshal=json"`

	ExpiresAt time.Time
}

type Medias []Media

type MediaFilter struct {
	Type     *MediaType
	Name     *string
	MemberID *int64
}

// LeaseDue reports whether the lease of m, which lasts for ttl, is to be extended at now.
func (m Media) LeaseDue(now time.Time, ttl time.Duration) bool {
	return m.ExpiresAt.Sub(now) < ttl/2
}
