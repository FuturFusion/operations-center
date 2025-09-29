package incus

import (
	"context"
	"net/http"
	"net/url"

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

func (s serverClient) getClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	return incus.ConnectIncusWithContext(ctx, endpoint.GetConnectionURL(), &incus.ConnectionArgs{
		TLSClientCert: s.clientCert,
		TLSClientKey:  s.clientKey,
		TLSServerCert: endpoint.GetCertificate(),

		// Bypass system proxy for communication to IncusOS servers.
		Proxy: func(r *http.Request) (*url.URL, error) {
			return nil, nil
		},
	})
}

func (s serverClient) HasExtension(ctx context.Context, endpoint provisioning.Endpoint, extension string) (exists bool) {
	client, err := s.getClient(ctx, endpoint)
	if err != nil {
		return false
	}

	return client.HasExtension(extension)
}
