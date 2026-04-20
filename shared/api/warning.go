package api

import (
	"time"

	"github.com/google/uuid"
)

// WarningStatus is the acknowledgement status of a warning.
type WarningStatus string

const (
	WarningStatusNew          WarningStatus = "new"
	WarningStatusAcknowledged WarningStatus = "acknowledged"
)

// WarningType represents a warning message group.
type WarningType string

const (
	// WarningTypeUnreachable indicates a server is unreachable.
	WarningTypeUnreachable WarningType = "Server unreachable"

	// WarningTypeClusterRollingUpdateNextAction indicates a warning during
	// evaluation of the next rolling cluster update action.
	WarningTypeClusterRollingUpdateNextAction WarningType = "Cluster update next action failed"

	// WarningTypeUpdateRefreshFailed indicates a warning happening during the
	// refresh of the updates.
	WarningTypeUpdateRefreshFailed WarningType = "Update refresh failed"

	// WarningTypeACMECertificateUpdateFailed indicates a warning happening during
	// the ACME certificate udpate.
	WarningTypeACMECertificateUpdateFailed WarningType = "ACME certificate update failed"

	// WarningTypeClusterInventoryResyncFailed indicates a warning during resync
	// of a cluster's inventory.
	WarningTypeClusterInventoryResyncFailed WarningType = "Inventory resync failed"

	// WarningTypeUpdateChannelMismatch indicates a warning where the update
	// channel reported by a server does not match the expected update channel
	// defined in Operations Center.
	WarningTypeUpdateChannelMismatch WarningType = "Update channel mismatch"

	// WarningTypeServerRegistrationScriptletFailed indicates a warning during
	// server registration where the registration scriptlet failed.
	WarningTypeServerRegistrationScriptletFailed WarningType = "Server registration scriptlet failed"

	// WarningTypeVersionDatailsMissing indicates a warning where version details
	// for a given update (OS or application) is missing.
	WarningTypeVersionDatailsMissing WarningType = "Update version details missing"
)

// WarningScope represents a scope for a warning.
type WarningScope struct {
	// Action scope of the warning.
	// Example: sync
	Scope string `json:"scope" yaml:"scope"`

	// Entity the warning relates to.
	// Example: source
	EntityType string `json:"entity_type" yaml:"entity_type"`

	// Name of the entity.
	// Example: mySource
	Entity string `json:"entity" yaml:"entity"`
}

// WarningPut represents configurable properties of a warning.
//
// swagger:model
type WarningPut struct {
	// Current acknowledgement status of the warning.
	// Example: new
	Status WarningStatus `json:"status" yaml:"status"`
}

// Warning represents a record of a warning.
//
// swagger:model
type Warning struct {
	WarningPut `yaml:",inline"`

	// Unique identifier of the warning.
	// Example: a2095069-a527-4b2a-ab23-1739325dcac7
	UUID uuid.UUID `json:"uuid" yaml:"uuid"`

	// Scope of the warning.
	Scope WarningScope `json:"scope" yaml:"scope"`

	// Type of the warning.
	// Example: Networks not imported
	Type WarningType `json:"type" yaml:"type"`

	// First time the warning occurred for the first time, that is, the timestamp
	// when the respective warning has been seen or observed by Operations Center
	// for the first time. RFC3339 format.
	// Example: 2025-01-01 01:00:00
	FirstOccurrence time.Time `json:"first_occurrence" yaml:"first_occurrence"`

	// Most recent time the warning occurred, that is, the timestamp when the
	// respective warning has been seen or observed by Operations Center for the
	// last time. RFC3339 format.
	// Example: 2025-01-01 01:00:00
	LastOccurrence time.Time `json:"last_occurrence" yaml:"last_occurrence"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`

	// Messages associated with the warning type.
	// Example: list of messages
	Messages []string `json:"messages" yaml:"messages"`

	// Number of times the warning has been seen.
	// Example: 10
	Count int `json:"count" yaml:"count"`
}
