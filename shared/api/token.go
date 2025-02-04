package api

import (
	"time"

	"github.com/google/uuid"
)

// Token defines a registration token for use during registration.
//
// swagger:model
type Token struct {
	// UUID of the token, which serves as the the token.
	// Example: b32d0079-c48b-4957-b1cb-bef54125c861
	UUID uuid.UUID `json:"uuid" yaml:"uuid"`

	// Value indicating, how many times the token might be used for registration.
	// Example: 10
	UsesRemaining int `json:"uses_remaining" yaml:"uses_remaining"`

	// The time at which the token expires in RFC3339 format with seconds precision.
	// Example: "2025-02-04T07:25:47Z"
	ExpireAt time.Time `json:"expire_at" yaml:"expire_at"`

	// Description of this token.
	// Example: "Test Environment"
	Description string `json:"description" yaml:"description"`
}
