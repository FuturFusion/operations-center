package image

import (
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/file"
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

	Source *string `json:"source" db:"leftjoin=incus_image_sources.name"`

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

	err = api.ValidateIncusImageArchitecture(i.Architecture)
	if err != nil {
		return err
	}

	if i.Variant == "" {
		return domain.NewValidationErrf("Invalid incus image, variant can not be empty")
	}

	err = validateFilePathElements(i.OperatingSystem, i.Release, i.Variant)
	if err != nil {
		return err
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

	err := api.ValidateIncusImageArchitecture(nameParts[2])
	if err != nil {
		return err
	}

	if nameParts[3] == "" {
		return domain.NewValidationErrf("Invalid incus image, variant can not be empty")
	}

	return nil
}

func fixArchitectureMapping(architecture string) string {
	switch architecture {
	case "x86_64":
		return "amd64"

	case "aarch64":
		return "arm64"

	default:
		return architecture
	}
}

// ValidateIncusImageFilename checks that filename is a single, non-empty file
// name. The name is used to store the file. So it must not contain path
// separators and it must not be "." or "..".
func ValidateIncusImageFilename(filename string) error {
	if !file.IsPathElement(filename) {
		return domain.NewValidationErrf("Invalid incus image, %q is not a valid file name", filename)
	}

	return nil
}

func validateFilePathElements(elements ...string) error {
	for _, element := range elements {
		err := ValidateIncusImageFilename(element)
		if err != nil {
			return err
		}
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
