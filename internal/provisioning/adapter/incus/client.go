package incus

import (
	"context"
	"fmt"

	incus "github.com/lxc/incus/v6/client"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type client struct {
	clientCert string
	clientKey  string
}

var _ provisioning.ServerClientPort = client{}

func New(clientCert string, clientKey string) client {
	return client{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

func (c client) getClient(ctx context.Context, server provisioning.Server) (incus.InstanceServer, error) {
	return incus.ConnectIncusWithContext(ctx, server.ConnectionURL, &incus.ConnectionArgs{
		TLSClientCert: c.clientCert,
		TLSClientKey:  c.clientKey,
		TLSServerCert: server.Certificate,
		SkipGetServer: true,
	})
}

func (c client) Ping(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.GetServer()

	return err
}

func (c client) GetResources(ctx context.Context, server provisioning.Server) (api.HardwareData, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return api.HardwareData{}, err
	}

	resources, err := client.GetServerResources()
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Get resources from %q failed: %w", server.ConnectionURL, err)
	}

	return api.HardwareData{
		Resources: *resources,
	}, nil
}
