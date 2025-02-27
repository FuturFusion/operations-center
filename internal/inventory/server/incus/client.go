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
		TLSClientCert:      s.clientCert,
		TLSClientKey:       s.clientKey,
		InsecureSkipVerify: true,
	})
}
