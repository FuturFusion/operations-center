package api

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
)

type UpdateStatus string

const (
	UpdateStatusUnknown UpdateStatus = "unknown"
	UpdateStatusPending UpdateStatus = "pending"
	UpdateStatusReady   UpdateStatus = "ready"
)

var updateStatuses = map[UpdateStatus]struct{}{
	UpdateStatusUnknown: {},
	UpdateStatusPending: {},
	UpdateStatusReady:   {},
}

func (s UpdateStatus) String() string {
	return string(s)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s UpdateStatus) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *UpdateStatus) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = UpdateStatusUnknown
		return nil
	}

	_, ok := updateStatuses[UpdateStatus(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update status", string(text))
	}

	*s = UpdateStatus(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s UpdateStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface.
func (s *UpdateStatus) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update status")
	}

	switch v := value.(type) {
	case string:
		return s.UnmarshalText([]byte(v))

	case []byte:
		return s.UnmarshalText(v)

	default:
		return fmt.Errorf("type %T is not supported for update status", value)
	}
}

// Update defines an update for a given server type.
//
// swagger:model
type Update struct {
	// UUID of the update.
	UUID uuid.UUID `json:"uuid" yaml:"uuid"`

	// Version of the Update as opaque string.
	// Example: 202501311418
	Version string `json:"version" yaml:"version"`

	// PublishedAt is the date, when the Update has been published in RFC3339 format.
	// Example: 2025-02-12T09:59:00Z
	PublishedAt time.Time `json:"published_at" yaml:"published_at"`

	// Severity of the Update. Allowed values: none, low, medium, high, critical
	// Example: none
	Severity images.UpdateSeverity `json:"severity" yaml:"severity"`

	// Origin of the Update.
	// Example: linuxcontainers.org
	Origin string `json:"origin" yaml:"origin"`

	// // URL of the File.
	// // Example: releases/download/202501311418/
	URL string `json:"url" yaml:"url"`

	// UpstreamChannels holds the name of the channels from upstream (source)
	// the update is part of.
	// Example: stable
	UpstreamChannels []string `json:"upstream_channels" yaml:"upstream_channels"`

	// Changelog of the Update as plain text.
	Changelog string `json:"changelog" yaml:"changelog"`

	// Status contains the status the update is currently in.
	// Possible values for status are: pending, ready
	// Example: ready
	Status UpdateStatus `json:"update_status" yaml:"update_status"`
}

// UpdateFile defines an update file.
//
// swagger:model
type UpdateFile struct {
	// Filename of the File.
	// Example: IncusOS_202501311418.efi.gz
	Filename string `json:"filename" yaml:"filename"`

	// Size of the File in bytes.
	// Example: 54300000
	Size int `json:"size" yaml:"size"`

	// Sha256 checksum of the file in hex encoding (64 ascii characters in the alphabet [0-9A-Fa-f])
	// Example: 11465a836ce54a8f293ac9234aa51050094cfbb8906c1a10ab9487dd92088643
	Sha256 string `json:"sha256"`

	// Component the file provides. One of: os, incus, debug
	// Example: os
	Component images.UpdateFileComponent `json:"component" yaml:"component"`

	// Type of the file. One of: image-raw, image-iso, image-manifest, update-efi, update-usr, update-usr-verity, update-usr-verity-signature
	// Example: image-raw
	Type images.UpdateFileType `json:"type" yaml:"type"`

	// Architecture of the file. E.g. x86_64, aarch64
	// Example: x86_64
	Architecture images.UpdateFileArchitecture `json:"architecture" yaml:"architecture"`
}
