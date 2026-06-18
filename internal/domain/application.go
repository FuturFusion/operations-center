package domain

import "slices"

func IsApplicationNameIncusKind(name string) bool {
	switch name {
	case "incus", "incus-lts-7.0":
		return true

	default:
		return false
	}
}

var primaryApplications = []string{"incus", "incus-lts-7.0", "operations-center", "migration-manager"}

func IsPrimaryApplication(name string) bool {
	return slices.Contains(primaryApplications, name)
}
