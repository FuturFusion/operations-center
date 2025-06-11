package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
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

func (c client) GetOSData(ctx context.Context, server provisioning.Server) (api.OSData, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return api.OSData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/network", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS network data from %q failed: %w", server.ConnectionURL, err)
	}

	var network incusosapi.SystemNetwork
	err = json.Unmarshal(resp.Metadata, &network)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS network information from %q: %w", server.ConnectionURL, err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/encryption", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS encryption data from %q failed: %w", server.ConnectionURL, err)
	}

	var encryption incusosapi.SystemEncryption
	err = json.Unmarshal(resp.Metadata, &encryption)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS encryption information from %q: %w", server.ConnectionURL, err)
	}

	return api.OSData{
		Network:    network,
		Encryption: encryption,
	}, nil
}
