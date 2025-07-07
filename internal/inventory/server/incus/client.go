package incus

import (
	"context"

	incus "github.com/lxc/incus/v6/client"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type serverClient struct {
	clientCert string
	clientKey  string
}

func New(clientCert string, clientKey string) inventory.ServerClient {
	return serverClient{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

func (s serverClient) getClient(ctx context.Context, cluster provisioning.Cluster) (incus.InstanceServer, error) {
	return incus.ConnectIncusWithContext(ctx, cluster.ConnectionURL, &incus.ConnectionArgs{
		TLSClientCert: s.clientCert,
		TLSClientKey:  s.clientKey,
		TLSServerCert: cluster.Certificate,
	})
}

func (s serverClient) HasExtension(ctx context.Context, cluster provisioning.Cluster, extension string) (exists bool) {
	client, err := s.getClient(ctx, cluster)
	if err != nil {
		return false
	}

	return client.HasExtension(extension)
}
