package incus

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
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

type getClientOpt func(*incus.ConnectionArgs)

func getClientWithInsecureSkipVerify() getClientOpt {
	return func(args *incus.ConnectionArgs) {
		args.InsecureSkipVerify = true
	}
}

func (c client) getClient(ctx context.Context, server provisioning.Server, opts ...getClientOpt) (incus.InstanceServer, error) {
	args := &incus.ConnectionArgs{
		TLSClientCert: c.clientCert,
		TLSClientKey:  c.clientKey,
		TLSServerCert: server.Certificate,
		SkipGetServer: true,
	}

	for _, opt := range opts {
		opt(args)
	}

	return incus.ConnectIncusWithContext(ctx, server.ConnectionURL, args)
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

func (c client) EnableCluster(ctx context.Context, server provisioning.Server, clusterName string) (clusterCertificate string, _ error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return "", err
	}

	// Ignore error, connection URL is already parsed in incus client.
	serverAddressURL, _ := url.Parse(server.ConnectionURL)

	// TODO: decide, if this should be its own command on the client or not.
	svr, etag, err := client.GetServer()
	if err != nil {
		return "", err
	}

	if svr.Config == nil {
		svr.Config = map[string]string{}
	}

	svr.Config["cluster.https_address"] = serverAddressURL.Host

	err = client.UpdateServer(svr.Writable(), etag)
	if err != nil {
		return "", err
	}

	req := incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
		},
		// FIXME: What is the right value here?
		// ClusterCertificate: "dummy",
		// ServerAddress:      serverAddressURL.Host,
	}

	fmt.Printf("Request: %#v\n", req)

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

func (c client) InsecureGetClusterCertificate(ctx context.Context, cluster provisioning.Server) (clusterCertificate string, _ error) {
	slog.WarnContext(ctx, "Cluster certificate is fetched insecurely", slog.String("cluster", cluster.Name), slog.String("connection_url", cluster.ConnectionURL))

	client, err := c.getClient(ctx, cluster, getClientWithInsecureSkipVerify())
	if err != nil {
		return "", err
	}

	clusterURL, err := url.Parse(cluster.ConnectionURL + "/1.0")
	if err != nil {
		return "", err
	}

	resp, err := client.DoHTTP(&http.Request{
		Method: http.MethodGet,
		URL:    clusterURL,
		Body:   http.NoBody,
	})
	if err != nil {
		return "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	certificate := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: resp.TLS.PeerCertificates[0].Raw,
	})

	return string(certificate), nil
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

func (c client) JoinCluster(ctx context.Context, server provisioning.Server, joinToken string, clusterCertificate string) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	// Ignore error, connection URL is already parsed in incus client.
	serverAddressURL, _ := url.Parse(server.ConnectionURL)

	op, err := client.UpdateCluster(incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName:   server.Name,
			Enabled:      true,
			MemberConfig: []incusapi.ClusterMemberConfigKey{},
		},
		ClusterAddress:     serverAddressURL.Host,
		ServerAddress:      serverAddressURL.Host,
		ClusterToken:       joinToken,
		ClusterCertificate: clusterCertificate,
	}, "")
	if err != nil {
		return fmt.Errorf("Failed to join cluster on %q: %w", server.ConnectionURL, err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("Failed to join cluster on %q: %w", server.ConnectionURL, err)
	}

	return nil
}
