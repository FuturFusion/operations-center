package api

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type UpdateSeverity string

const (
	UpdateSeverityNone     UpdateSeverity = "none"
	UpdateSeverityLow      UpdateSeverity = "low"
	UpdateSeverityMedium   UpdateSeverity = "medium"
	UpdateSeverityHigh     UpdateSeverity = "high"
	UpdateSeverityCritical UpdateSeverity = "critical"
)

var updateSeverities = map[UpdateSeverity]struct{}{
	UpdateSeverityNone:     {},
	UpdateSeverityLow:      {},
	UpdateSeverityMedium:   {},
	UpdateSeverityHigh:     {},
	UpdateSeverityCritical: {},
}

func (u UpdateSeverity) String() string {
	return string(u)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u UpdateSeverity) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateSeverity) UnmarshalText(text []byte) error {
	_, ok := updateSeverities[UpdateSeverity(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update severity", string(text))
	}

	*u = UpdateSeverity(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateSeverity) Value() (driver.Value, error) {
	return string(u), nil
}

// Scan implements the sql.Scanner interface.
func (u *UpdateSeverity) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update severity")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update severity", value)
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
	Severity UpdateSeverity `json:"severity" yaml:"severity"`

	// Origin of the Update.
	// Example: linuxcontainers.org
	Origin string `json:"origin" yaml:"origin"`

	// Channel of the Update.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`

	// Changelog of the Update as plain text.
	Changelog string `json:"changelog" yaml:"changelog"`
}

type UpdateFileComponent string

const (
	UpdateFileComponentOS    UpdateFileComponent = "os"
	UpdateFileComponentIncus UpdateFileComponent = "incus"
	UpdateFileComponentDebug UpdateFileComponent = "debug"
)

var updateFileComponents = map[UpdateFileComponent]struct{}{
	UpdateFileComponentOS:    {},
	UpdateFileComponentIncus: {},
	UpdateFileComponentDebug: {},
}

func (u UpdateFileComponent) String() string {
	return string(u)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u UpdateFileComponent) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateFileComponent) UnmarshalText(text []byte) error {
	_, ok := updateFileComponents[UpdateFileComponent(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update file component", string(text))
	}

	*u = UpdateFileComponent(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateFileComponent) Value() (driver.Value, error) {
	return string(u), nil
}

// Scan implements the sql.Scanner interface.
func (u *UpdateFileComponent) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update file component")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update file component", value)
	}
}

type UpdateFileType string

const (
	UpdateFileTypeUndefined                UpdateFileType = ""
	UpdateFileTypeImageRaw                 UpdateFileType = "image-raw"
	UpdateFileTypeImageISO                 UpdateFileType = "image-iso"
	UpdateFileTypeImageManifest            UpdateFileType = "image-manifest"
	UpdateFileTypeUpdateEFI                UpdateFileType = "update-efi"
	UpdateFileTypeUpdateUsr                UpdateFileType = "update-usr"
	UpdateFileTypeUpdateUsrVerity          UpdateFileType = "update-usr-verity"
	UpdateFileTypeUpdateUsrVeritySignature UpdateFileType = "update-usr-verity-signature"
)

var updateFileType = map[UpdateFileType]struct{}{
	UpdateFileTypeUndefined:                {},
	UpdateFileTypeImageRaw:                 {},
	UpdateFileTypeImageISO:                 {},
	UpdateFileTypeImageManifest:            {},
	UpdateFileTypeUpdateEFI:                {},
	UpdateFileTypeUpdateUsr:                {},
	UpdateFileTypeUpdateUsrVerity:          {},
	UpdateFileTypeUpdateUsrVeritySignature: {},
}

func (u UpdateFileType) String() string {
	return string(u)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u UpdateFileType) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateFileType) UnmarshalText(text []byte) error {
	_, ok := updateFileType[UpdateFileType(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update file type", string(text))
	}

	*u = UpdateFileType(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateFileType) Value() (driver.Value, error) {
	return string(u), nil
}

// Scan implements the sql.Scanner interface.
func (u *UpdateFileType) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update file type")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update file type", value)
	}
}

type Architecture string

const (
	ArchitectureUndefined              Architecture = ""
	Architecture64BitIntelX86          Architecture = "x86_64"
	Architecture64BitARMV8LittleEndian Architecture = "aarch64"
)

var architecture = map[Architecture]struct{}{
	ArchitectureUndefined:              {},
	Architecture64BitIntelX86:          {},
	Architecture64BitARMV8LittleEndian: {},
}

func (u Architecture) String() string {
	return string(u)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u Architecture) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *Architecture) UnmarshalText(text []byte) error {
	_, ok := architecture[Architecture(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update file type", string(text))
	}

	*u = Architecture(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u Architecture) Value() (driver.Value, error) {
	return string(u), nil
}

// Scan implements the sql.Scanner interface.
func (u *Architecture) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update file type")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update file type", value)
	}
}

// UpdateFile defines an update file.
//
// swagger:model
type UpdateFile struct {
	// Filename of the File.
	// Example: IncusOS_202501311418.efi.gz
	Filename string `json:"filename" yaml:"filename"`

	// URL of the File.
	// Example: https://github.com/lxc/incus-os/releases/download/202501311418/IncusOS_202501311418.efi.gz
	URL string `json:"url" yaml:"url"`

	// Size of the File in bytes.
	// Example: 54300000
	Size int `json:"size" yaml:"size"`

	// Sha256 checksum of the file in hex encoding (64 ascii characters in the alphabet [0-9A-Fa-f])
	// Example: 11465a836ce54a8f293ac9234aa51050094cfbb8906c1a10ab9487dd92088643
	Sha256 string `json:"sha256"`

	// Component the file provides. One of: os, incus, debug
	// Example: os
	Component UpdateFileComponent `json:"component" yaml:"component"`

	// Type of the file. One of: image-raw, image-iso, image-manifest, update-efi, update-usr, update-usr-verity, update-usr-verity-signature
	// Example: image-raw
	Type UpdateFileType `json:"type" yaml:"type"`

	// Architecture of the file. E.g. x86_64, aarch64
	// Example: x86_64
	Architecture Architecture `json:"arch" yaml:"arch"`
}
