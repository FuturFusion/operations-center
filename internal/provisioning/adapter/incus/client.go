package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incus "github.com/lxc/incus/v6/client"
	incusapi "github.com/lxc/incus/v6/shared/api"

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
	serverCertificate := server.Certificate
	if server.ClusterCertificate != nil {
		serverCertificate = *server.ClusterCertificate
	}

	args := &incus.ConnectionArgs{
		TLSClientCert: c.clientCert,
		TLSClientKey:  c.clientKey,
		TLSServerCert: serverCertificate,
		SkipGetServer: true,
	}

	return incus.ConnectIncusWithContext(ctx, server.ConnectionURL, args)
}

func (c client) Ping(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to ping %q: %w", server.ConnectionURL, err)
	}

	return nil
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

func (c client) UpdateNetworkConfig(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/network", server.OSData.Network, "")
	if err != nil {
		return fmt.Errorf("Put OS network data to %q failed: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) EnableOSServiceLVM(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	serviceLVMConfig := map[string]any{
		"enabled":   true,
		"system_id": server.ID,
	}

	_, _, err = client.RawQuery(http.MethodGet, "/os/1.0/services/lvm", serviceLVMConfig, "")
	if err != nil {
		return fmt.Errorf("Enable OS service LVM on %q failed: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) SetServerConfig(ctx context.Context, server provisioning.Server, config map[string]string) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	svr, etag, err := client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to get current server config from %q: %w", server.ConnectionURL, err)
	}

	if svr.Config == nil {
		svr.Config = map[string]string{}
	}

	for key, value := range config {
		svr.Config[key] = value
	}

	err = client.UpdateServer(svr.Writable(), etag)
	if err != nil {
		return fmt.Errorf("Failed to set server config on %q: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) EnableCluster(ctx context.Context, server provisioning.Server) (clusterCertificate string, _ error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return "", err
	}

	req := incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
		},
	}

	op, err := client.UpdateCluster(req, "")
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.ConnectionURL, err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.ConnectionURL, err)
	}

	anyClusterCertificate, ok := op.Get().Metadata["certificate"]
	if !ok {
		return "", nil
	}

	clusterCertificate, ok = anyClusterCertificate.(string)
	if !ok {
		return "", nil
	}

	return clusterCertificate, nil
}

func (c client) GetClusterNodeNames(ctx context.Context, server provisioning.Server) ([]string, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return nil, err
	}

	nodeNames, err := client.GetClusterMemberNames()
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster node names: %w", err)
	}

	return nodeNames, nil
}

func (c client) GetClusterJoinToken(ctx context.Context, server provisioning.Server, memberName string) (joinToken string, _ error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return "", err
	}

	op, err := client.CreateClusterMember(incusapi.ClusterMembersPost{
		ServerName: memberName,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to get cluster join token on %q: %w", server.ConnectionURL, err)
	}

	opAPI := op.Get()
	token, err := opAPI.ToClusterJoinToken()
	if err != nil {
		return "", fmt.Errorf("Failed converting token operation to join token: %w", err)
	}

	return token.String(), nil
}

func (c client) JoinCluster(ctx context.Context, server provisioning.Server, joinToken string, cluster provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	// Ignore error, connection URL has been parsed by incus client already.
	serverAddressURL, _ := url.Parse(server.ConnectionURL)
	clusterAddressURL, _ := url.Parse(cluster.ConnectionURL)

	op, err := client.UpdateCluster(incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
			// TODO: Add storage pool config?
			MemberConfig: []incusapi.ClusterMemberConfigKey{},
		},
		ClusterCertificate: cluster.Certificate,
		ServerAddress:      serverAddressURL.Host,
		ClusterToken:       joinToken,
		ClusterAddress:     clusterAddressURL.Host,
	}, "")
	if err != nil {
		return fmt.Errorf("Failed to update cluster during cluster join on %q: %w", server.ConnectionURL, err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("Failed to wait for update operation during cluster join on %q: %w", server.ConnectionURL, err)
	}

	return nil
}
