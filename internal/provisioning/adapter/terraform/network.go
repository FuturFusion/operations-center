package terraform

import (
	"slices"

	"github.com/FuturFusion/operations-center/shared/api"
)

// detectClusteringInterface returns the first interface that has the role
// "clustering" and at least one IP address assigned.
func detectClusteringInterface(network api.ServerSystemNetwork) string {
	for name, iface := range network.State.Interfaces {
		// TODO: use constant from incus-osd/api instead of string "clustering".
		if slices.Contains(iface.Roles, "clustering") && len(iface.Addresses) > 0 {
			return name
		}
	}

	// TODO: Once incus-osd ensures the correct setting of the interface roles,
	// the can be set to empty string.
	return "enp5s0"
}
