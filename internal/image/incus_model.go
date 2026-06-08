package image

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type IncusImage struct {
	ID          int64    `json:"-"`
	Name        string   `json:"name" db:"primary=yes"`
	Aliases     []string `json:"aliases" db:"marshal=json"`
	Description string   `json:"description"`

	OperatingSystem string `json:"os"`
	Release         string `json:"release"`
	Architecture    string `json:"arch"`
	Variant         string `json:"variant"`

	Versions api.IncusImageVersions `json:"versions"`

	Source *string `json:"source" db:"leftjoin=image_sources.name"`

	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (i IncusImage) Validate() error {
	err := ValidateIncusImageName(i.Name)
	if err != nil {
		return err
	}

	if i.OperatingSystem == "" {
		return domain.NewValidationErrf("Invalid incus image, operating system can not be empty")
	}

	if i.Release == "" {
		return domain.NewValidationErrf("Invalid incus image, release can not be empty")
	}

	if !slices.Contains([]string{"amd64", "arm64", "armhf", "riscv64"}, i.Architecture) {
		return domain.NewValidationErrf("Invalid incus image, architecture is not supported")
	}

	if i.Variant == "" {
		return domain.NewValidationErrf("Invalid incus image, variant can not be empty")
	}

	if i.Name != strings.Join([]string{i.OperatingSystem, i.Release, i.Architecture, i.Variant}, ":") {
		return domain.NewValidationErrf(`Invalid incus image, name needs to match "os:release:architecture:variant"`)
	}

	set := make(map[string]bool, len(i.Aliases))
	for _, alias := range i.Aliases {
		if set[alias] {
			return domain.NewValidationErrf(`Invalid aliases, %q is contained multiple times`, alias)
		}

		set[alias] = true
	}

	return nil
}

func ValidateIncusImageName(name string) error {
	nameParts := strings.Split(name, ":")
	if len(nameParts) != 4 {
		return domain.NewValidationErrf(`Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
	}

	if nameParts[0] == "" {
		return domain.NewValidationErrf("Invalid incus image, operating system can not be empty")
	}

	if nameParts[1] == "" {
		return domain.NewValidationErrf("Invalid incus image, release can not be empty")
	}

	if !slices.Contains([]string{"amd64", "arm64", "armhf", "riscv64"}, nameParts[2]) {
		return domain.NewValidationErrf("Invalid incus image, architecture is not supported")
	}

	if nameParts[3] == "" {
		return domain.NewValidationErrf("Invalid incus image, variant can not be empty")
	}

	return nil
}

// ValidateIncusImageVersion checks the version matches the expected version
// format in simplestreams.
// https://github.com/lxc/incus/blob/1d64af1e40ced8716280bd4fcf044dce4ca6d5cf/shared/simplestreams/products.go#L87-L95
func ValidateIncusImageVersion(version string) error {
	const versionLayout = "20060102"

	if len(version) < 8 {
		return fmt.Errorf(`Invalid version, version is required to be a 8 digits long date in the format "yyyymmdd"`)
	}

	_, err := time.Parse(versionLayout, version[0:8])
	if err != nil {
		return fmt.Errorf(`Invalid version, version is required to be a 8 digits long date in the format "yyyymmdd": %w`, err)
	}

	return nil
}

func (i IncusImage) Path() string {
	return path.Join(i.OperatingSystem, i.Release, i.Architecture, i.Variant)
}

func (i IncusImage) FilePath() string {
	return filepath.Join(
		strings.ToLower(i.OperatingSystem),
		strings.ToLower(i.Release),
		strings.ToLower(i.Architecture),
		strings.ToLower(i.Variant),
	)
}

type IncusImageFilter struct {
	ID              *int
	Name            *string
	OperatingSystem *string
	Release         *string
	Architecture    *string
	Variant         *string
	Source          *string
}

type ExprIncusImageVersionFile struct {
	Name            string `json:"name"              yaml:"name"              expr:"name"`
	OperatingSystem string `json:"operating_system"  yaml:"operating_system"  expr:"operating_system"`
	Release         string `json:"release"           yaml:"release"           expr:"release"`
	Architecture    string `json:"architecture"      yaml:"architecture"      expr:"architecture"`
	Variant         string `json:"variant"           yaml:"variant"           expr:"variant"`
	Version         string `json:"version"           yaml:"version"           expr:"version"`
	Filename        string `json:"filename"          yaml:"filename"          expr:"filename"`
	FileType        string `json:"file_type"         yaml:"file_type"         expr:"file_type"`
	Size            int64  `json:"size"              yaml:"size"              expr:"size"`
}

type IncusImages []IncusImage

type UsageInformation struct {
	TotalSpaceBytes     uint64
	AvailableSpaceBytes uint64
	UsedSpaceBytes      uint64
}
