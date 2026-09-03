// Package bios provides the catalog of BIOS profiles, that is shipped with
// Operations Center.
package bios

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"

	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

//go:embed profiles/*.yaml
var builtinProfiles embed.FS

// Catalogue serves the BIOS profiles from an immutable set of YAML documents.
type Catalogue struct {
	profiles provisioning.BIOSProfiles
}

var _ provisioning.BIOSProfilePort = Catalogue{}

// New returns the catalog of BIOS profiles shipped with Operations Center.
func New() (Catalogue, error) {
	return NewFromFS(builtinProfiles, "profiles")
}

// NewFromFS returns a catalog of the BIOS profiles read from the YAML
// documents found in dir of fsys.
func NewFromFS(fsys fs.FS, dir string) (Catalogue, error) {
	entries, err := fs.Glob(fsys, dir+"/*.yaml")
	if err != nil {
		return Catalogue{}, fmt.Errorf("Failed to list BIOS profiles in %q: %w", dir, err)
	}

	slices.Sort(entries)

	profiles := provisioning.BIOSProfiles{}
	seenNames := map[string]string{}

	for _, entry := range entries {
		body, err := fs.ReadFile(fsys, entry)
		if err != nil {
			return Catalogue{}, fmt.Errorf("Failed to read BIOS profiles from %q: %w", entry, err)
		}

		fileProfiles := provisioning.BIOSProfiles{}

		err = yaml.Unmarshal(body, &fileProfiles)
		if err != nil {
			return Catalogue{}, fmt.Errorf("Failed to parse BIOS profiles from %q: %w", entry, err)
		}

		for _, profile := range fileProfiles {
			err = profile.Validate()
			if err != nil {
				return Catalogue{}, fmt.Errorf("Invalid BIOS profile in %q: %w", entry, err)
			}

			duplicateOf, ok := seenNames[profile.Name]
			if ok {
				return Catalogue{}, fmt.Errorf("Duplicate BIOS profile %q in %q, already defined in %q", profile.Name, entry, duplicateOf)
			}

			seenNames[profile.Name] = entry

			profiles = append(profiles, profile)
		}
	}

	profiles.Sort()

	return Catalogue{
		profiles: profiles,
	}, nil
}

func (c Catalogue) Resolve(_ context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
	resolution, err := c.profiles.Resolve(server.BMCData)
	if err != nil {
		return nil, fmt.Errorf("Failed to resolve the BIOS profiles for server %q: %w", server.Name, err)
	}

	return resolution, nil
}
