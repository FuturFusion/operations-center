package mock

import (
	"context"
	"net/http"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// IncusClientWithoutMeshNetwork returns the incus client of a server, whose cluster does not have an internal mesh network.
func IncusClientWithoutMeshNetwork(ctx context.Context, endpoint provisioning.Endpoint) (provisioning.InstanceServer, error) {
	var incusClient *InstanceServerMock

	incusClient = &InstanceServerMock{
		UseTargetFunc: func(name string) incus.InstanceServer {
			return incusClient
		},
		GetNetworkFunc: func(name string) (*api.Network, string, error) {
			return nil, "", api.StatusErrorf(http.StatusNotFound, "Network not found")
		},
	}

	return incusClient, nil
}
