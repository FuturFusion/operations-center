package incus

import (
	"context"

	incus "github.com/lxc/incus/v6/client"

	"github.com/FuturFusion/operations-center/internal/inventory"
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

func (s serverClient) getClient(ctx context.Context, connectionURL string) (incus.InstanceServer, error) {
	return incus.ConnectIncusWithContext(ctx, connectionURL, &incus.ConnectionArgs{
		TLSClientCert: s.clientCert,
		TLSClientKey:  s.clientKey,
		// FIXME: connection should be verified with the certificate received from the server/cluster.
		InsecureSkipVerify: true,
	})
}

func (s serverClient) HasExtension(ctx context.Context, connectionURL string, extension string) (exists bool) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return false
	}

	return client.HasExtension(extension)
}
