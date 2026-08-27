package bios

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (c Catalogue) GetAll(_ context.Context) (provisioning.BIOSProfiles, error) {
	profiles := make(provisioning.BIOSProfiles, 0, len(c.profiles))
	for _, profile := range c.profiles {
		profiles = append(profiles, profile.Clone())
	}

	return profiles, nil
}
