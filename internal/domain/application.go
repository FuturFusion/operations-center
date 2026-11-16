package domain

import "slices"

const (
	// ApplicationNameIncus is the stable (feature release) flavor of Incus.
	ApplicationNameIncus = "incus"

	// ApplicationNameIncusLTS70 is the 7.0 LTS flavor of Incus.
	ApplicationNameIncusLTS70 = "incus-lts-7.0"

	// DefaultIncusApplicationName is the Incus flavor used for new deployments,
	// if none is requested explicitly. IncusOS builds provide both flavors,
	// HypervisorOS builds provide the 7.0 LTS flavor only.
	DefaultIncusApplicationName = ApplicationNameIncusLTS70
)

func IsApplicationNameIncusKind(name string) bool {
	switch name {
	case ApplicationNameIncus, ApplicationNameIncusLTS70:
		return true

	default:
		return false
	}
}

var primaryApplications = []string{ApplicationNameIncus, ApplicationNameIncusLTS70, "operations-center", "migration-manager"}

func IsPrimaryApplication(name string) bool {
	return slices.Contains(primaryApplications, name)
}
