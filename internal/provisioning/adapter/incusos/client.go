package incusos

import (
	"context"
	"fmt"
	"net/http"

	incustls "github.com/lxc/incus/v6/shared/tls"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type client struct {
	clientCert string
	clientKey  string
}

var _ provisioning.ServerClientPort = &client{}

func New(clientCert string, clientKey string) *client {
	return &client{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

type serverClient struct {
	http.Client
}

func (c client) getClient(server provisioning.Server) (*serverClient, error) {
	tlsConfig, err := incustls.GetTLSConfigMem(c.clientCert, c.clientKey, "", server.Certificate, false)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &serverClient{
		Client: http.Client{
			Transport: transport,
		},
	}, nil
}

func (c *client) Ping(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(server)
	if err != nil {
		return err
	}

	resp, err := client.Get(server.ConnectionURL)
	if err != nil {
		return fmt.Errorf("Ping to %q failed: %w", server.ConnectionURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response for ping to %q: %d %s", server.ConnectionURL, resp.StatusCode, resp.Status)
	}

	return nil
}
