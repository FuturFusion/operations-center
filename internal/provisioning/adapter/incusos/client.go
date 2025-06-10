package incusos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	incustls "github.com/lxc/incus/v6/shared/tls"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
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
	// FIXME: Replace with incus client, use RawQuery for API, where there is not yet support in the incus client.
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

	resp, err := client.Get(apiPath(server.ConnectionURL, ""))
	if err != nil {
		return fmt.Errorf("Ping to %q failed: %w", server.ConnectionURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response for ping to %q: %d %s", server.ConnectionURL, resp.StatusCode, resp.Status)
	}

	return nil
}

func (c *client) GetResources(ctx context.Context, server provisioning.Server) (api.HardwareData, error) {
	client, err := c.getClient(server)
	if err != nil {
		return api.HardwareData{}, err
	}

	resp, err := client.Get(apiPath(server.ConnectionURL, "/1.0/resources"))
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Get resources from %q failed: %w", server.ConnectionURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return api.HardwareData{}, fmt.Errorf("Unexpected response while fetching resource information from %q: %d %s", server.ConnectionURL, resp.StatusCode, resp.Status)
	}

	var response api.Response
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&response)
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Unexpected response body while fetching resource information from %q: %w", server.ConnectionURL, err)
	}

	var resources api.HardwareData
	err = json.Unmarshal(response.Metadata, &resources)
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Unexpected response metadata while fetching resource information from %q: %w", server.ConnectionURL, err)
	}

	return resources, err
}

// apiPath concatenates connectionURL and resourcePath to an URL while ensuring
// there is only a single slash (/) between the two.
func apiPath(connectionURL string, resourcePath string) string {
	if resourcePath == "" {
		return connectionURL
	}

	connectionURL, _ = strings.CutSuffix(connectionURL, "/")
	resourcePath, _ = strings.CutPrefix(resourcePath, "/")

	return connectionURL + "/" + resourcePath
}
