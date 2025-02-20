package incus

import (
	"context"

	incus "github.com/lxc/incus/v6/client"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
)

type serverClient struct {
	clientCert string
	clientKey  string
}

func ServerClientProvider(clientCert string, clientKey string) inventory.ServerClient {
	return serverClient{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

func (s serverClient) getClient(ctx context.Context, connectionURL string) (incus.InstanceServer, error) {
	return incus.ConnectIncusWithContext(ctx, connectionURL, &incus.ConnectionArgs{
		TLSClientCert:      s.clientCert,
		TLSClientKey:       s.clientKey,
		InsecureSkipVerify: true,
	})
}

func (s serverClient) GetInstances(ctx context.Context, connectionURL string) ([]incusapi.InstanceFull, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverInstances, err := client.GetInstancesFullAllProjects(incusapi.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	return serverInstances, nil
}
