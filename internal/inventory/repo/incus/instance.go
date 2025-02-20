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

func (s serverClient) GetImages(ctx context.Context, connectionURL string) ([]incusapi.Image, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverImages, err := client.GetImagesAllProjects()
	if err != nil {
		return nil, err
	}

	return serverImages, nil
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

func (s serverClient) GetNetworks(ctx context.Context, connectionURL string) ([]incusapi.Network, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverNetworks, err := client.GetNetworksAllProjects()
	if err != nil {
		return nil, err
	}

	return serverNetworks, nil
}

func (s serverClient) GetNetworkACLs(ctx context.Context, connectionURL string) ([]incusapi.NetworkACL, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverNetworkACLs, err := client.GetNetworkACLsAllProjects()
	if err != nil {
		return nil, err
	}

	return serverNetworkACLs, nil
}
