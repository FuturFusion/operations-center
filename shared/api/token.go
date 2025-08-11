package api

import (
	"time"

	"github.com/google/uuid"
)

// Token defines a registration token for use during registration.
//
// swagger:model
type Token struct {
	TokenPut

	// UUID of the token, which serves as the the token.
	// Example: b32d0079-c48b-4957-b1cb-bef54125c861
	UUID uuid.UUID `json:"uuid" yaml:"uuid"`
}

// TokenPut defines the configurable properties of Token.
//
// swagger:model
type TokenPut struct {
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

// TokenISOPost defines the configuration to generate a pre-seeded ISO for a given Token.
//
// Operations Center just passes through the provided configuration for application.yaml,
// install.yaml and network.yaml as is without any validation of the provided configuration
// besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenISOPost struct {
	// Applications represents the applications configuration (application.yaml) to be included in the pre-seeded ISO.
	Applications map[string]any `json:"applications" yaml:"applications"`

	// Network represents the network configuration (network.yaml) to be included in the pre-seeded ISO.
	Network map[string]any `json:"network" yaml:"network"`

	// Install represents the install configuration (install.yaml) to be included in the pre-seeded ISO.
	Install map[string]any `json:"install" yaml:"install"`
}
