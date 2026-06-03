package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IncusImagePut represents the fields available for update for an incus image.
//
// swagger:model
type IncusImagePut struct {
	// Aliases of the incus image.
	Aliases IncusImageAlias `json:"aliases" yaml:"aliases"`

	// Description of the incus image.
	Description string `json:"description" yaml:"description"`
}

// IncusImagePost represents the fields available when creating an incus image version.
//
// swagger:model
type IncusImagePost struct {
	// OperatingSystem of the incus image.
	// Example: almalinux
	OperatingSystem string `json:"os" yaml:"os"`

	// Release of the operating system of the incus image.
	// Example: 10
	Release string `json:"release" yaml:"release"`

	// Architecture the incus image is built for.
	// Example: amd64
	Architecture string `json:"arch" yaml:"arch"`

	// Variant of the incus image.
	// Example: default
	Variant string `json:"variant" yaml:"variant"`

	// Version of the incus image.
	// Example: 20260616
	Version string `json:"version" yaml:"version"`
}

// IncusImage defines an incus image.
//
// swagger:model
type IncusImage struct {
	// Loosly based on simplestreams.Product from
	// https://github.com/lxc/incus/blob/40dd4f151d52c06b178482aa2518abfb9df3e6fb/shared/simplestreams/products.go#L25-L40

	IncusImagePut `yaml:",inline"`

	// Name of the incus image in the format "os:release:architecture:variant".
	// Example: almalinux:10:amd64:cloud
	Name string `json:"name" yaml:"name"`

	// OperatingSystem of the incus image.
	// Example: almalinux
	OperatingSystem string `json:"os" yaml:"os"`

	// Release of the operating system of the incus image.
	// Example: 10
	Release string `json:"release" yaml:"release"`

	// Architecture the incus image is built for.
	// Example: amd64
	Architecture string `json:"arch" yaml:"arch"`

	// Variant of the incus image.
	// Example: default
	Variant string `json:"variant" yaml:"variant,omitempty"`

	// Versions of the incus image.
	Versions map[string]IncusImageVersion `json:"versions" yaml:"versions"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}

type IncusImageAlias []string

func (i *IncusImageAlias) UnmarshalJSON(data []byte) error {
	var aliasList []string
	err := json.Unmarshal(data, &aliasList)
	if err == nil {
		*i = aliasList
		return nil
	}

	var aliasCSV string
	err = json.Unmarshal(data, &aliasCSV)
	if err == nil {
		if aliasCSV == "" {
			*i = nil
			return nil
		}

		aliases := strings.Split(aliasCSV, ",")
		for i := range aliases {
			aliases[i] = strings.TrimSpace(aliases[i])
		}

		*i = aliases
		return nil
	}

	return fmt.Errorf("IncusImageAlias: expected string or []string")
}

type IncusImageVersion struct {
	// Items of the incus image version. This is the file artifacts, that are
	// available for a given incus image version.
	Items map[string]IncusImageVersionItem `json:"items" yaml:"items"`
}

type IncusImageVersionItem struct {
	// CombinedSha256DiskKvmImg holds the combinded sha256 hash of the incus image
	// versions metadata ("incus.tar.xz") and the "disk.qcow2" image.
	CombinedSha256DiskKvmImg string `json:"combined_disk-kvm-img_sha256,omitempty" yaml:"combined_disk-kvm-img_sha256"`

	// CombinedSha256RootXz holds the combinded sha256 hash of the incus image
	// versions metadata ("incus.tar.xz") and the "root.tar.xz" image.
	CombinedSha256RootXz string `json:"combined_rootxz_sha256,omitempty" yaml:"combined_rootxz_sha256"`

	// CombinedSha256SquashFs holds the combinded sha256 hash of the incus image
	// versions metadata ("incus.tar.xz") and the "root.squashfs" image.
	CombinedSha256SquashFs string `json:"combined_squashfs_sha256,omitempty" yaml:"combined_squashfs_sha256"`

	// FileType holds the file type. This defaults to the file name like
	// incus.tar.xz. Special file types are "squashfs" and "disk-kvm.img".
	FileType string `json:"ftype" yaml:"ftype"`

	// Path, relative to the repositories root folder, where the particular
	// file artifact is stored.
	Path string `json:"path" yaml:"path"`

	// HashSha256 holds the sha256 hash of the file.
	HashSha256 string `json:"sha256,omitempty" yaml:"sha256"`

	// Size holds the size in bytes of the file.
	Size int64 `json:"size" yaml:"size"`
}

type IncusImageVersions map[string]IncusImageVersion

// Value implements the sql driver.Valuer interface.
func (i IncusImageVersions) Value() (driver.Value, error) {
	return json.Marshal(i)
}

// Scan implements the sql.Scanner interface.
func (i *IncusImageVersions) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid incus image versions")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*i = IncusImageVersions{}
			return nil
		}

		return json.Unmarshal([]byte(v), i)

	case []byte:
		if len(v) == 0 {
			*i = IncusImageVersions{}
			return nil
		}

		return json.Unmarshal(v, i)

	default:
		return fmt.Errorf("type %T is not supported for incus image versions", value)
	}
}
