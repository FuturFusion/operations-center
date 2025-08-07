package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"slices"

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

var (
	_ provisioning.ServerClientPort  = client{}
	_ provisioning.ClusterClientPort = client{}
)

func New(clientCert string, clientKey string) client {
	return client{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

func (c client) getClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	args := &incus.ConnectionArgs{
		TLSClientCert: c.clientCert,
		TLSClientKey:  c.clientKey,
		TLSServerCert: endpoint.GetCertificate(),
		SkipGetServer: true,
	}

	return incus.ConnectIncusWithContext(ctx, endpoint.GetConnectionURL(), args)
}

func (c client) Ping(ctx context.Context, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	_, _, err = client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to ping %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c client) GetResources(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.HardwareData{}, err
	}

	resources, err := client.GetServerResources()
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Get resources from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	return api.HardwareData{
		Resources: *resources,
	}, nil
}

func (c client) GetOSData(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.OSData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/network", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS network data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var network incusosapi.SystemNetwork
	err = json.Unmarshal(resp.Metadata, &network)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS network information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/security", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS security data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var security incusosapi.SystemSecurity
	err = json.Unmarshal(resp.Metadata, &security)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS security information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	return api.OSData{
		Network:  network,
		Security: security,
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

func (c client) SetServerConfig(ctx context.Context, endpoint provisioning.Endpoint, config map[string]string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	svr, etag, err := client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to get current config from %q: %w", endpoint.GetConnectionURL(), err)
	}

	if svr.Config == nil {
		svr.Config = map[string]string{}
	}

	for key, value := range config {
		svr.Config[key] = value
	}

	err = client.UpdateServer(svr.Writable(), etag)
	if err != nil {
		return fmt.Errorf("Failed to set config on %q: %w", endpoint.GetConnectionURL(), err)
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
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.GetConnectionURL(), err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.GetConnectionURL(), err)
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

func (c client) GetClusterNodeNames(ctx context.Context, endpoint provisioning.Endpoint) ([]string, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	nodeNames, err := client.GetClusterMemberNames()
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster node names on %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nodeNames, nil
}

func (c client) GetClusterJoinToken(ctx context.Context, endpoint provisioning.Endpoint, memberName string) (joinToken string, _ error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return "", err
	}

	op, err := client.CreateClusterMember(incusapi.ClusterMembersPost{
		ServerName: memberName,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to get cluster join token on %q: %w", endpoint.GetConnectionURL(), err)
	}

	opAPI := op.Get()
	token, err := opAPI.ToClusterJoinToken()
	if err != nil {
		return "", fmt.Errorf("Failed converting token operation to join token: %w", err)
	}

	return token.String(), nil
}

func (c client) JoinCluster(ctx context.Context, server provisioning.Server, joinToken string, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	// Ignore error, connection URL has been parsed by incus client already.
	serverAddressURL, _ := url.Parse(server.ConnectionURL)
	clusterAddressURL, _ := url.Parse(endpoint.GetConnectionURL())

	op, err := client.UpdateCluster(incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
			// TODO: Add storage pool config?
			MemberConfig: []incusapi.ClusterMemberConfigKey{},
		},
		ClusterCertificate: endpoint.GetCertificate(),
		ServerAddress:      serverAddressURL.Host,
		ClusterToken:       joinToken,
		ClusterAddress:     clusterAddressURL.Host,
	}, "")
	if err != nil {
		return fmt.Errorf("Failed to update cluster during cluster join on %q: %w", server.GetConnectionURL(), err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("Failed to wait for update operation during cluster join on %q: %w", server.GetConnectionURL(), err)
	}

	return nil
}

func (c client) CreateProject(ctx context.Context, endpoint provisioning.Endpoint, name string, description string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	err = client.CreateProject(incusapi.ProjectsPost{
		Name: name,
		ProjectPut: incusapi.ProjectPut{
			Description: description,
			Config:      map[string]string{},
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create project on %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}

// InitializeDefaultStorage performs the post-clustering initialization for the storage:
//   - Create local storage pool on each server and finalize it for the cluster.
//   - Create two volumes on that pool on each server named images and backups.
//   - Set storage.images_volume and storage.backups_volume on each server to point to the volumes.
//   - Update the default profile in the default project to use the local storage pool.
//   - Update the default profile in the internal project to use the local storage pool.
func (c client) InitializeDefaultStorage(ctx context.Context, servers []provisioning.Server) error {
	if len(servers) == 0 {
		return fmt.Errorf("Server list cannot be empty for default storage initialization")
	}

	// Use the first server of the cluster for communication.
	firstServer := servers[0]

	client, err := c.getClient(ctx, firstServer)
	if err != nil {
		return fmt.Errorf(`Failed to connect to %q: %w`, firstServer.ConnectionURL, err)
	}

	profileDefault, profileDefaultEtag, err := client.GetProfile("default")
	if err != nil {
		return fmt.Errorf(`Failed to get profile "default" on %q: %w`, firstServer.GetConnectionURL(), err)
	}

	if profileDefault.Devices == nil {
		profileDefault.Devices = map[string]map[string]string{}
	}

	internalProfileDefault, internalProfileDefaultEtag, err := client.UseProject("internal").GetProfile("default")
	if err != nil {
		return fmt.Errorf(`Failed to get profile "default" in project "internal" on %q: %w`, firstServer.GetConnectionURL(), err)
	}

	if internalProfileDefault.Devices == nil {
		internalProfileDefault.Devices = map[string]map[string]string{}
	}

	// Check for storage pools.
	storagePools, err := client.GetStoragePoolNames()
	if err != nil {
		return fmt.Errorf(`Failed to get storage pool names on %q: %w`, firstServer.GetConnectionURL(), err)
	}

	if len(storagePools) != 0 {
		return fmt.Errorf(`Server on %q has already %d storage pools defined`, firstServer.ConnectionURL, len(storagePools))
	}

	// Create local storage pool.
	for _, server := range servers {
		// Create local storage pool on each server.
		err = client.UseTarget(server.Name).CreateStoragePool(incusapi.StoragePoolsPost{
			Name:   "local",
			Driver: "zfs",
			StoragePoolPut: incusapi.StoragePoolPut{
				Config: map[string]string{
					"source": "local/incus",
				},
				Description: "Local storage pool (on system drive)",
			},
		})
		if err != nil {
			return fmt.Errorf(`Failed to create storage pool "local" on %q (target: %s): %w`, firstServer.ConnectionURL, server.Name, err)
		}
	}

	// Finalize storage pool creation on the cluster.
	err = client.CreateStoragePool(incusapi.StoragePoolsPost{
		Name:   "local",
		Driver: "zfs",
		StoragePoolPut: incusapi.StoragePoolPut{
			Description: "Local storage pool (on system drive)",
		},
	})
	if err != nil {
		return fmt.Errorf(`Failed to finalize storage pool on %q: %w`, firstServer.ConnectionURL, err)
	}

	// Create storage volumes and update server config for backups and images.
	for _, server := range servers {
		// Create the default volumes.
		for _, volName := range []string{"backups", "images"} {
			// Create default volumes (backups and images), on every server.
			err = client.UseTarget(server.Name).CreateStoragePoolVolume("local", incusapi.StorageVolumesPost{
				Name:        volName,
				Type:        "custom",
				ContentType: "filesystem",
				StorageVolumePut: incusapi.StorageVolumePut{
					Description: "Volume holding system " + volName,
				},
			})
			if err != nil {
				return fmt.Errorf(`Failed to create storage volume "local/%s" on %q (target: %s): %w`, volName, firstServer.ConnectionURL, server.Name, err)
			}

			// Set server config on each server.
			err = c.SetServerConfig(ctx, server, map[string]string{
				fmt.Sprintf("storage.%s_volume", volName): "local/" + volName,
			})
			if err != nil {
				return fmt.Errorf(`Failed to set server configuration "storage.%s_volume" on %q (target: %s): %w`, volName, firstServer.ConnectionURL, server.Name, err)
			}
		}
	}

	// Add local storage pool to the default profile.
	profileDefault.Devices["root"] = map[string]string{
		"type": "disk",
		"path": "/",
		"pool": "local",
	}

	err = client.UpdateProfile("default", profileDefault.Writable(), profileDefaultEtag)
	if err != nil {
		return fmt.Errorf(`Failed to update profile "default" on %q: %w`, firstServer.ConnectionURL, err)
	}

	// Add local storage pool to the default profile of the internal project.
	internalProfileDefault.Devices["root"] = map[string]string{
		"type": "disk",
		"path": "/",
		"pool": "local",
	}

	err = client.UseProject("internal").UpdateProfile("default", internalProfileDefault.Writable(), internalProfileDefaultEtag)
	if err != nil {
		return fmt.Errorf(`Failed to update profile "default" in project "internal" on %q: %w`, firstServer.ConnectionURL, err)
	}

	return nil
}

// InitializeDefaultNetworking performs the post-clustering initialization for the networking:
//   - Create local network bridge "incusbr0" on each server.
//   - Create an "meshbr0" network bridge on each server.
//   - Update the default profile in the default project to use incusbr0 for networking.
//   - Update the default profile in the internal project to use meshbr0 for networking.
func (c client) InitializeDefaultNetworking(ctx context.Context, servers []provisioning.Server) error {
	if len(servers) == 0 {
		return fmt.Errorf("Server list cannot be empty for default storage initialization")
	}

	// Use the first server of the cluster for communication.
	firstServer := servers[0]

	client, err := c.getClient(ctx, servers[0])
	if err != nil {
		return fmt.Errorf(`Failed to connect to %q: %w`, firstServer.ConnectionURL, err)
	}

	profileDefault, profileDefaultEtag, err := client.GetProfile("default")
	if err != nil {
		return fmt.Errorf(`Failed to get profile "default" on %q: %w`, firstServer.GetConnectionURL(), err)
	}

	if profileDefault.Devices == nil {
		profileDefault.Devices = map[string]map[string]string{}
	}

	internalProfileDefault, internalProfileDefaultEtag, err := client.UseProject("internal").GetProfile("default")
	if err != nil {
		return fmt.Errorf(`Failed to get profile "default" in project "internal" on %q: %w`, firstServer.GetConnectionURL(), err)
	}

	if internalProfileDefault.Devices == nil {
		internalProfileDefault.Devices = map[string]map[string]string{}
	}

	// Check for networks.
	allNetworks, err := client.GetNetworks()
	if err != nil {
		return fmt.Errorf(`Failed to get networks on %q: %w`, firstServer.ConnectionURL, err)
	}

	networks := []incusapi.Network{}
	for _, network := range allNetworks {
		if !network.Managed {
			continue
		}

		networks = append(networks, network)
	}

	if len(networks) != 0 {
		return fmt.Errorf(`Server on %q has already %d networks defined`, firstServer.ConnectionURL, len(networks))
	}

	// Create local network bridges "incusbr0" and "meshbr0" on each server.
	for _, server := range servers {
		// Create the bridge networks.
		for _, bridge := range []struct {
			name        string
			description string
		}{
			{
				name:        "incusbr0",
				description: "Local network bridge (NAT)",
			},
			{
				name:        "meshbr0",
				description: "Internal mesh network bridge",
			},
		} {
			err = client.UseTarget(server.Name).CreateNetwork(incusapi.NetworksPost{
				Name: bridge.name,
				Type: "bridge",
				NetworkPut: incusapi.NetworkPut{
					Description: bridge.description,
				},
			})
			if err != nil {
				return fmt.Errorf(`Failed to create network bridge %q on %q (target: %s): %w`, bridge.name, firstServer.ConnectionURL, server.Name, err)
			}
		}
	}

	// Finalize network bridges on the cluster.
	for _, name := range []string{"incusbr0", "meshbr0"} {
		err = client.CreateNetwork(incusapi.NetworksPost{
			Name: name,
		})
		if err != nil {
			return fmt.Errorf(`Failed to finalize network bridge %q on %q: %w`, name, firstServer.ConnectionURL, err)
		}
	}

	// Set network config for meshbr0 on each server.
	clusterIPv6Prefix, err := randomSubnetV6()
	if err != nil {
		return fmt.Errorf(`Failed to generate random IPv6 subnet: %w`, err)
	}

	meshbr0, meshbr0ETag, err := client.GetNetwork("meshbr0")
	if err != nil {
		return fmt.Errorf(`Failed to get network "meshbr0": %w`, err)
	}

	meshbr0.Config["ipv4.address"] = "none"
	meshbr0.Config["ipv6.address"] = clusterIPv6Prefix.String() + "/64"
	meshbr0.Config["tunnel.mesh.id"] = "1000"
	meshbr0.Config["tunnel.mesh.protocol"] = "vxlan"

	err = client.UpdateNetwork("meshbr0", meshbr0.Writable(), meshbr0ETag)
	if err != nil {
		return fmt.Errorf(`Failed to update network "meshbr0" on %q: %w`, firstServer.ConnectionURL, err)
	}

	for _, server := range servers {
		meshbr0, meshbr0ETag, err := client.UseTarget(server.Name).GetNetwork("meshbr0")
		if err != nil {
			return fmt.Errorf(`Failed to get network "meshbr0" on %q (target: %s): %w`, firstServer.ConnectionURL, server.Name, err)
		}

		meshbr0.Config["tunnel.mesh.interface"] = detectClusteringInterface(server.OSData.Network)

		err = client.UseTarget(server.Name).UpdateNetwork("meshbr0", meshbr0.Writable(), meshbr0ETag)
		if err != nil {
			return fmt.Errorf(`Failed to update network "meshbr0" on %q (target: %s): %w`, firstServer.ConnectionURL, server.Name, err)
		}
	}

	// Add incusbr0 to the default profile.
	profileDefault.Devices["eth0"] = map[string]string{
		"type":    "nic",
		"network": "incusbr0",
		"name":    "eth0",
	}

	err = client.UpdateProfile("default", profileDefault.Writable(), profileDefaultEtag)
	if err != nil {
		return fmt.Errorf(`Failed to update profile "default" on %q: %w`, firstServer.ConnectionURL, err)
	}

	// Add meshbr0 to the default profile of internal project.
	internalProfileDefault.Devices["eth0"] = map[string]string{
		"type":    "nic",
		"network": "meshbr0",
		"name":    "eth0",
	}

	err = client.UseProject("internal").UpdateProfile("default", internalProfileDefault.Writable(), internalProfileDefaultEtag)
	if err != nil {
		return fmt.Errorf(`Failed to update profile "default" in project "internal" on %q: %w`, firstServer.ConnectionURL, err)
	}

	return nil
}

func randomSubnetV6() (net.IP, error) {
	for range 100 {
		cidr := fmt.Sprintf("fd42:%x:%x:%x::/64", rand.IntN(65535), rand.IntN(65535), rand.IntN(65535))
		addr, _, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}

		return addr, nil
	}

	return nil, errors.New("Failed to automatically find an IPv6 subnet")
}

// detectClusteringInterface returns the first interface that has the role
// "clustering" and at least one IP address assigned.
func detectClusteringInterface(network api.ServerSystemNetwork) string {
	for name, iface := range network.State.Interfaces {
		// TODO: use constant from incus-osd/api instead of string "clustering".
		if slices.Contains(iface.Roles, "clustering") && len(iface.Addresses) > 0 {
			return name
		}
	}

	// TODO: Once incus-osd ensures the correct setting of the interface roles,
	// the can be set to empty string.
	return "enp5s0"
}

func (c client) UpdateClusterCertificate(ctx context.Context, endpoint provisioning.Endpoint, certificatePEM string, keyPEM string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	return client.UpdateClusterCertificate(incusapi.ClusterCertificatePut{
		ClusterCertificate:    certificatePEM,
		ClusterCertificateKey: keyPEM,
	}, "")
}
