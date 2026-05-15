package image

import (
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

//
//generate-expr: IncusImage

type IncusImage struct {
	ID          int64  `json:"-"`
	Name        string `json:"name" db:"primary=yes"`
	Description string `json:"description"`

	OperatingSystem string `json:"os"`
	Release         string `json:"release"`
	Architecture    string `json:"arch"`
	Variant         string `json:"variant"`

	Versions api.IncusImageVersions `json:"versions"`

	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (i IncusImage) Validate() error {
	nameParts := strings.Split(i.Name, ":")
	if len(nameParts) != 4 {
		return domain.NewValidationErrf(`Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
	}

	if i.OperatingSystem == "" {
		return domain.NewValidationErrf("Invalid incus image, operating system can not be empty")
	}

	if i.Release == "" {
		return domain.NewValidationErrf("Invalid incus image, release can not be empty")
	}

	// TODO: validate for valid architectures
	if i.Architecture == "" {
		return domain.NewValidationErrf("Invalid incus image, architecture can not be empty")
	}

	if i.Variant == "" {
		return domain.NewValidationErrf("Invalid incus image, variant can not be empty")
	}

	if i.Name != strings.Join([]string{i.OperatingSystem, i.Release, i.Architecture, i.Variant}, ":") {
		return domain.NewValidationErrf(`Invalid incus image, name needs to match "os:release:architecture:variant"`)
	}

	return nil
}

func (i IncusImage) Path() string {
	return path.Join(i.OperatingSystem, i.Release, i.Architecture, i.Variant)
}

func (i IncusImage) FilePath() string {
	return filepath.Join(i.OperatingSystem, i.Release, i.Architecture, i.Variant)
}

type IncusImageFilter struct {
	ID              *int
	Name            *string
	OperatingSystem *string
	Release         *string
	Architecture    *string
	Variant         *string
}

type IncusImages []IncusImage

type UsageInformation struct {
	TotalSpaceBytes     uint64
	AvailableSpaceBytes uint64
	UsedSpaceBytes      uint64
}
