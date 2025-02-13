package incus

import (
	"context"

	incus "github.com/lxc/incus/v6/client"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
)

type serverClient struct {
	client incus.InstanceServer
}

func ServerClientProvider(clientCert string, clientKey string) inventory.ServerClientProvider {
	return func(ctx context.Context, connectionURL string) (inventory.ServerClient, error) {
		client, err := incus.ConnectIncusWithContext(ctx, connectionURL, &incus.ConnectionArgs{
			TLSClientCert:      clientCert,
			TLSClientKey:       clientKey,
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, err
		}

		return serverClient{
			client: client,
		}, nil
	}
}

func (s serverClient) GetInstances(ctx context.Context) ([]incusapi.InstanceFull, error) {
	serverInstances, err := s.client.GetInstancesFullAllProjects(incusapi.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	return serverInstances, nil
}
