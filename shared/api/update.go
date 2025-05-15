package api

import (
	"bytes"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type UpdateComponent string

const (
	UpdateComponentHypervisorOS     UpdateComponent = "Hypervisor OS"
	UpdateComponentMigrationManager UpdateComponent = "Migration Manager"
	UpdateComponentWorker           UpdateComponent = "Migration Manager Worker"
	UpdateComponentOperationsCenter UpdateComponent = "Operations Center"
)

var updateComponents = map[UpdateComponent]struct{}{
	UpdateComponentHypervisorOS:     {},
	UpdateComponentMigrationManager: {},
	UpdateComponentWorker:           {},
	UpdateComponentOperationsCenter: {},
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u UpdateComponent) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateComponent) UnmarshalText(text []byte) error {
	_, ok := updateComponents[UpdateComponent(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid update component", string(text))
	}

	*u = UpdateComponent(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateComponent) Value() (driver.Value, error) {
	return string(u), nil
}

// Scan implements the sql.Scanner interface.
func (u *UpdateComponent) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update component")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update component", value)
	}
}

type UpdateComponents []UpdateComponent

func (u UpdateComponents) String() string {
	strs := make([]string, 0, len(u))
	for _, uc := range u {
		strs = append(strs, string(uc))
	}

	return strings.Join(strs, ", ")
}

// MarshalText implements the encoding.TextMarshaler interface.
func (u UpdateComponents) MarshalText() ([]byte, error) {
	s := make([]string, 0, len(u))
	for _, c := range u {
		s = append(s, string(c))
	}

	return []byte(strings.Join(s, ",")), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateComponents) UnmarshalText(text []byte) error {
	components := bytes.Split(text, []byte(","))
	*u = make(UpdateComponents, 0, len(components))
	for _, component := range components {
		if len(component) == 0 {
			continue
		}

		var c UpdateComponent
		err := c.UnmarshalText(component)
		if err != nil {
			return err
		}

		*u = append(*u, c)
	}

	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateComponents) Value() (driver.Value, error) {
	return u.MarshalText()
}

// Scan implements the sql.Scanner interface.
func (u *UpdateComponents) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid list of update components")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for a list of update components", value)
	}
}

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

	// List of Components, that are available with the Update. Allowed entries:
	// HypervisorOS, Migration Manager, Migration Manager Worker, Operations Center
	// Example: ["HypervisorOS", "Migration Manager"]
	Components UpdateComponents `json:"components" yaml:"components"`

	// Version of the Update as opaque string.
	// Example: 202501311418
	Version string `json:"version" yaml:"version"`

	// PublishedAt is the date, when the Update has been published in RFC3339 format.
	// Example: 2025-02-12T09:59:00Z
	PublishedAt time.Time `json:"published_at" yaml:"published_at"`

	// Severity of the Update. Allowed values: none, low, medium, high, critical
	// Example: none
	Severity UpdateSeverity `json:"severity" yaml:"severity"`

	// Channel of the Update.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`
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

	// Component the file provides. One of: os, incus, debug
	// Example: os
	Component UpdateFileComponent `json:"component" yaml:"component"`

	// Type of the file. One of: image-raw, image-iso, image-manifest, update-efi, update-usr, update-usr-verity, update-usr-verity-signature
	// Example: image-raw
	Type UpdateFileType `json:"type" yaml:"type"`
}
