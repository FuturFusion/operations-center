package api

import (
	"time"
)

// UpdateExposedchannelPut represents the fields available for update
// for an exposed channel for updates.
//
// swagger:model
type UpdateExposedchannelPut struct {
	// Description of the exposed channel.
	// Example: stable channel, used for production.
	Description string `json:"description" yaml:"description"`
}

// UpdateExposedchannelPost represents the fields available when creating
// an exposed channel for updates.
//
// swagger:model
type UpdateExposedchannelPost struct {
	UpdateExposedchannelPut

	// Name of the exposed channel.
	// Example: stable
	Name string `json:"name" yaml:"name"`
}

// UpdateExposedchannel defines an update channel, that is exposed to systems
// provisioned by Operations Center.
//
// swagger:model
type UpdateExposedchannel struct {
	UpdateExposedchannelPost

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
