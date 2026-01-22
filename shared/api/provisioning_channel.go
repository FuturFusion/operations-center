package api

import (
	"time"
)

// ChannelPut represents the fields available for update for a channel.
//
// swagger:model
type ChannelPut struct {
	// Description of the channel.
	// Example: stable channel, used for production.
	Description string `json:"description" yaml:"description"`
}

// ChannelPost represents the fields available when creating a channel.
//
// swagger:model
type ChannelPost struct {
	ChannelPut

	// Name of the channel.
	// Example: stable
	Name string `json:"name" yaml:"name"`
}

// Channel defines a channel.
//
// swagger:model
type Channel struct {
	ChannelPost

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
