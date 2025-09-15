package terraform

import (
	"github.com/FuturFusion/operations-center/shared/api"
)

// detectClusterInterface returns the first interface that has the role
// "cluster" and at least one IP address assigned.
func detectClusterInterface(network api.ServerSystemNetwork) string {
	clusteringInterfaces := network.State.GetInterfaceNamesByRole("cluster")
	for _, name := range clusteringInterfaces {
		iface := network.State.Interfaces[name]
		if len(iface.Addresses) > 0 {
			return name
		}
	}

	return ""
}
