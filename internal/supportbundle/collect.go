package supportbundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

// systemResources are the IncusOS system resources included for every server.
var systemResources = []string{"resources", "network", "storage", "security", "update", "provider", "kernel", "logging"}

var errNoReachableServer = errors.New("No server of the cluster is reachable")

// collector gathers the sections of a support bundle for the resource of req.
type collector struct {
	client      SupportBundleClientPort
	b           *bundle
	req         api.SupportBundlePost
	scope       scope
	unreachable map[string]error
}

func (c *collector) collect(ctx context.Context) {
	c.b.json(ctx, "operations-center/version.json", sectionTimeout, static(map[string]string{
		"version":    version.Version,
		"go_version": runtime.Version(),
	}))

	if c.scope.cluster != nil {
		c.b.json(ctx, "operations-center/cluster.json", sectionTimeout, static(c.scope.cluster))
	}

	known := c.knownServers()
	for _, server := range known {
		c.b.json(ctx, path.Join("operations-center/servers", server.Name+".json"), sectionTimeout, static(server))
	}

	c.unreachable = probe(ctx, c.client, known)

	if c.scope.cluster != nil {
		c.collectClusterSummary(ctx)
	}

	var involved []string
	switch c.req.Type {
	case api.SupportBundleTypeCluster:
		c.collectClusterEnvironment(ctx)
		involved = serverNames(known)

	case api.SupportBundleTypeProject:
		involved = c.collectProject(ctx)

	case api.SupportBundleTypeInstance:
		involved = c.collectInstance(ctx)

	case api.SupportBundleTypeServer:
		involved = []string{c.req.Server}
	}

	for _, name := range involved {
		idx := slices.IndexFunc(known, func(s provisioning.Server) bool { return s.Name == name })
		if idx < 0 {
			c.b.note("Server %q is not known to Operations Center, its data is not included", name)
			continue
		}

		c.collectServer(ctx, known[idx])
	}
}

func (c *collector) knownServers() provisioning.Servers {
	if c.scope.cluster != nil {
		return c.scope.clusterServers
	}

	return provisioning.Servers{*c.scope.server}
}

// clusterEndpoint returns the first reachable server of the cluster, nil if there is none.
func (c *collector) clusterEndpoint() *provisioning.Server {
	for _, server := range c.scope.clusterServers {
		_, unreachable := c.unreachable[server.Name]
		if !unreachable {
			return &server
		}
	}

	return nil
}

// incus returns a section function, which queries the Incus API of endpoint.
func (c *collector) incus(endpoint *provisioning.Server, project string, get func(client provisioning.InstanceServer) (any, error)) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		client, err := c.instanceServer(ctx, endpoint, project)
		if err != nil {
			return nil, err
		}

		return get(client)
	}
}

func (c *collector) instanceServer(ctx context.Context, endpoint *provisioning.Server, project string) (provisioning.InstanceServer, error) {
	if endpoint == nil {
		return nil, errNoReachableServer
	}

	// The connection is bound to the context, so it is established for every section.
	client, err := c.client.IncusClient(ctx, *endpoint)
	if err != nil {
		return nil, err
	}

	if project != "" {
		client = client.UseProject(project)
	}

	return client, nil
}

func (c *collector) collectClusterSummary(ctx context.Context) {
	endpoint := c.clusterEndpoint()

	c.b.json(ctx, "cluster/server.json", sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		server, _, err := client.GetServer()
		return server, err
	}))

	c.b.json(ctx, "cluster/cluster.json", sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		cluster, _, err := client.GetCluster()
		return cluster, err
	}))

	var members []incusapi.ClusterMember
	c.b.json(ctx, "cluster/members.json", sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		var err error
		members, err = client.GetClusterMembers()
		return members, err
	}))

	for _, member := range members {
		c.b.json(ctx, path.Join("cluster/members", member.ServerName, "state.json"), sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
			state, _, err := client.GetClusterMemberState(member.ServerName)
			return state, err
		}))

		if !slices.Contains(serverNames(c.scope.clusterServers), member.ServerName) {
			c.b.note("Cluster member %q is not known to Operations Center", member.ServerName)
		}
	}

	c.b.json(ctx, "cluster/warnings.json", sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		return client.GetWarnings()
	}))
}

func (c *collector) collectClusterEnvironment(ctx context.Context) {
	endpoint := c.clusterEndpoint()

	sections := []struct {
		name string
		get  func(client provisioning.InstanceServer) (any, error)
	}{
		{name: "projects", get: func(client provisioning.InstanceServer) (any, error) { return client.GetProjects() }},
		{name: "profiles", get: func(client provisioning.InstanceServer) (any, error) { return client.GetProfilesAllProjects() }},
		{name: "networks", get: func(client provisioning.InstanceServer) (any, error) { return client.GetNetworksAllProjects() }},
		{name: "network-acls", get: func(client provisioning.InstanceServer) (any, error) { return client.GetNetworkACLsAllProjects() }},
		{name: "storage-pools", get: func(client provisioning.InstanceServer) (any, error) { return client.GetStoragePools() }},
		{name: "storage-volumes", get: func(client provisioning.InstanceServer) (any, error) {
			return storageVolumes(client, client.GetStoragePoolVolumesAllProjects)
		}},
		{name: "images", get: func(client provisioning.InstanceServer) (any, error) { return client.GetImagesAllProjects() }},
		{name: "instances", get: func(client provisioning.InstanceServer) (any, error) {
			return client.GetInstancesFullAllProjects(incusapi.InstanceTypeAny)
		}},
	}

	for _, section := range sections {
		c.b.json(ctx, path.Join("cluster", section.name+".json"), sectionTimeout, c.incus(endpoint, "", section.get))
	}
}

// collectProject returns the servers hosting the instances of the project.
func (c *collector) collectProject(ctx context.Context) []string {
	endpoint := c.clusterEndpoint()
	project := c.req.Project
	base := path.Join("projects", project)

	c.b.json(ctx, path.Join(base, "project.json"), sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		project, _, err := client.GetProject(project)
		return project, err
	}))

	c.b.json(ctx, path.Join(base, "state.json"), sectionTimeout, c.incus(endpoint, "", func(client provisioning.InstanceServer) (any, error) {
		return client.GetProjectState(project)
	}))

	var instances []incusapi.InstanceFull
	c.b.json(ctx, path.Join(base, "instances.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
		var err error
		instances, err = client.GetInstancesFull(incusapi.InstanceTypeAny)
		return instances, err
	}))

	sections := []struct {
		name string
		get  func(client provisioning.InstanceServer) (any, error)
	}{
		{name: "profiles", get: func(client provisioning.InstanceServer) (any, error) { return client.GetProfiles() }},
		{name: "networks", get: func(client provisioning.InstanceServer) (any, error) { return client.GetNetworks() }},
		{name: "storage-volumes", get: func(client provisioning.InstanceServer) (any, error) {
			return storageVolumes(client, client.GetStoragePoolVolumes)
		}},
		{name: "images", get: func(client provisioning.InstanceServer) (any, error) { return client.GetImages() }},
	}

	for _, section := range sections {
		c.b.json(ctx, path.Join(base, section.name+".json"), sectionTimeout, c.incus(endpoint, project, section.get))
	}

	if instances == nil {
		c.b.note("The instances of project %q are unknown, the data of all servers of the cluster is included", project)
		return serverNames(c.scope.clusterServers)
	}

	locations := map[string]struct{}{}
	for _, instance := range instances {
		if instance.Location != "" && instance.Location != "none" {
			locations[instance.Location] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(locations))
}

// collectInstance returns the server hosting the instance.
func (c *collector) collectInstance(ctx context.Context) []string {
	endpoint := c.clusterEndpoint()
	project := c.req.Project
	name := c.req.Name
	base := path.Join("instances", project, name)

	var instance *incusapi.InstanceFull
	c.b.json(ctx, path.Join(base, "instance.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
		var err error
		instance, _, err = client.GetInstanceFull(name)
		return instance, err
	}))

	c.b.json(ctx, path.Join(base, "snapshots.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
		return client.GetInstanceSnapshots(name)
	}))

	location := c.scope.instanceLocation
	if instance != nil {
		location = instance.Location

		c.b.json(ctx, path.Join(base, "volumes.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
			return instanceVolumes(client, instance), nil
		}))

		c.b.json(ctx, path.Join(base, "networks.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
			return instanceNetworks(client, instance), nil
		}))
	}

	var logfiles []string
	c.b.json(ctx, path.Join(base, "logs.json"), sectionTimeout, c.incus(endpoint, project, func(client provisioning.InstanceServer) (any, error) {
		var err error
		logfiles, err = client.GetInstanceLogfiles(name)
		return logfiles, err
	}))

	for _, logfile := range logfiles {
		c.b.section(ctx, path.Join(base, "logs", path.Base(logfile)), sectionTimeout, instanceLogLimit, func(ctx context.Context) (io.ReadCloser, error) {
			if endpoint == nil {
				return nil, errNoReachableServer
			}

			return c.client.GetInstanceLogfile(ctx, *endpoint, project, name, path.Base(logfile))
		})
	}

	c.b.section(ctx, path.Join(base, "console.log"), sectionTimeout, instanceLogLimit, func(ctx context.Context) (io.ReadCloser, error) {
		if endpoint == nil {
			return nil, errNoReachableServer
		}

		return c.client.GetInstanceConsoleLog(ctx, *endpoint, project, name)
	})

	if location == "" || location == "none" {
		c.b.note("The location of instance %q is unknown, the data of all servers of the cluster is included", name)
		return serverNames(c.scope.clusterServers)
	}

	return []string{location}
}

func (c *collector) collectServer(ctx context.Context, server provisioning.Server) {
	base := path.Join("servers", server.Name)

	err, unreachable := c.unreachable[server.Name]
	if unreachable {
		c.b.skip(path.Join(base, "unreachable"), err)
		return
	}

	if server.Type.IsIncus() {
		c.b.json(ctx, path.Join(base, "incus/server.json"), sectionTimeout, c.incus(&server, "", func(client provisioning.InstanceServer) (any, error) {
			info, _, err := client.GetServer()
			return info, err
		}))

		c.b.json(ctx, path.Join(base, "incus/resources.json"), sectionTimeout, c.incus(&server, "", func(client provisioning.InstanceServer) (any, error) {
			return client.GetServerResources()
		}))
	}

	c.b.json(ctx, path.Join(base, "os/os.json"), sectionTimeout, func(ctx context.Context) (any, error) {
		return c.client.GetOS(ctx, server)
	})

	for _, resource := range systemResources {
		c.b.json(ctx, path.Join(base, "os/system", resource+".json"), sectionTimeout, func(ctx context.Context) (any, error) {
			return c.client.GetSystem(ctx, server, resource)
		})
	}

	var applications []string
	c.b.json(ctx, path.Join(base, "os/applications.json"), sectionTimeout, func(ctx context.Context) (any, error) {
		var err error
		applications, err = c.client.GetOSApplicationNames(ctx, server)
		return applications, err
	})

	for _, application := range applications {
		c.b.json(ctx, path.Join(base, "os/applications", application+".json"), sectionTimeout, func(ctx context.Context) (any, error) {
			return c.client.GetOSApplication(ctx, server, application)
		})
	}

	var journal []map[string]any
	c.b.json(ctx, path.Join(base, "os/debug/journal.json"), journalTimeout, func(ctx context.Context) (any, error) {
		var err error
		journal, err = c.client.GetOSDebugLog(ctx, server, journalEntries)
		return journal, err
	})

	if journal != nil {
		c.b.text(ctx, path.Join(base, "os/debug/journal.txt"), sectionTimeout, func(ctx context.Context) (string, error) {
			return formatJournal(journal), nil
		})
	}

	c.b.json(ctx, path.Join(base, "os/debug/kernel.json"), sectionTimeout, func(ctx context.Context) (any, error) {
		return c.client.GetOSDebugKernel(ctx, server)
	})

	c.b.text(ctx, path.Join(base, "os/debug/processes.txt"), sectionTimeout, func(ctx context.Context) (string, error) {
		return c.client.GetOSDebugProcesses(ctx, server)
	})

	if server.Type.IsIncus() {
		c.b.section(ctx, path.Join(base, "os/debug/incus-logs.tar.gz"), incusLogsTimeout, incusLogsSizeLimit, func(ctx context.Context) (io.ReadCloser, error) {
			return c.client.GetOSApplicationDebugLogs(ctx, server, server.Type.String())
		})
	}
}

func static(v any) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		return v, nil
	}
}

func serverNames(servers provisioning.Servers) []string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names
}

// storageVolumes returns the volumes of every storage pool by the name of the pool.
func storageVolumes(client provisioning.InstanceServer, getVolumes func(pool string) ([]incusapi.StorageVolume, error)) (map[string][]incusapi.StorageVolume, error) {
	pools, err := client.GetStoragePools()
	if err != nil {
		return nil, err
	}

	volumes := make(map[string][]incusapi.StorageVolume, len(pools))
	for _, pool := range pools {
		volumes[pool.Name], err = getVolumes(pool.Name)
		if err != nil {
			return nil, fmt.Errorf("Failed to get volumes of storage pool %q: %w", pool.Name, err)
		}
	}

	return volumes, nil
}

type instanceVolume struct {
	Device string                  `json:"device"`
	Pool   string                  `json:"pool"`
	Type   string                  `json:"type"`
	Name   string                  `json:"name"`
	Volume *incusapi.StorageVolume `json:"volume,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

// instanceVolumes returns the storage volumes attached to the instance.
func instanceVolumes(client provisioning.InstanceServer, instance *incusapi.InstanceFull) []instanceVolume {
	if instance.Location != "" && instance.Location != "none" {
		client = client.UseTarget(instance.Location)
	}

	volumes := []instanceVolume{}
	for _, device := range slices.Sorted(maps.Keys(instance.ExpandedDevices)) {
		config := instance.ExpandedDevices[device]
		if config["type"] != "disk" || config["pool"] == "" {
			continue
		}

		volume := instanceVolume{
			Device: device,
			Pool:   config["pool"],
			Type:   "custom",
			Name:   config["source"],
		}

		if config["path"] == "/" {
			volume.Type = "container"
			if instance.Type == string(incusapi.InstanceTypeVM) {
				volume.Type = "virtual-machine"
			}

			volume.Name = instance.Name
		}

		var err error
		volume.Volume, _, err = client.GetStoragePoolVolume(volume.Pool, volume.Type, volume.Name)
		if err != nil {
			volume.Error = err.Error()
		}

		volumes = append(volumes, volume)
	}

	return volumes
}

type instanceNetwork struct {
	Device  string            `json:"device"`
	Name    string            `json:"name"`
	Network *incusapi.Network `json:"network,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// instanceNetworks returns the managed networks the instance is connected to.
func instanceNetworks(client provisioning.InstanceServer, instance *incusapi.InstanceFull) []instanceNetwork {
	networks := []instanceNetwork{}
	for _, device := range slices.Sorted(maps.Keys(instance.ExpandedDevices)) {
		config := instance.ExpandedDevices[device]
		if config["type"] != "nic" || config["network"] == "" {
			continue
		}

		network := instanceNetwork{
			Device: device,
			Name:   config["network"],
		}

		var err error
		network.Network, _, err = client.GetNetwork(network.Name)
		if err != nil {
			network.Error = err.Error()
		}

		networks = append(networks, network)
	}

	return networks
}

// formatJournal renders the journal entries the way journalctl prints them by default.
func formatJournal(entries []map[string]any) string {
	var sb strings.Builder

	for _, entry := range entries {
		timestamp := ""
		microseconds, err := strconv.ParseInt(fmt.Sprint(entry["__REALTIME_TIMESTAMP"]), 10, 64)
		if err == nil {
			timestamp = time.UnixMicro(microseconds).UTC().Format(time.RFC3339Nano)
		}

		identifier := fmt.Sprint(entry["SYSLOG_IDENTIFIER"])
		if entry["SYSLOG_IDENTIFIER"] == nil {
			identifier = fmt.Sprint(entry["_SYSTEMD_UNIT"])
		}

		pid := ""
		if entry["_PID"] != nil {
			pid = fmt.Sprintf("[%v]", entry["_PID"])
		}

		fmt.Fprintf(&sb, "%s %s%s: %s\n", timestamp, identifier, pid, journalMessage(entry["MESSAGE"]))
	}

	return sb.String()
}

// journalMessage decodes a message, which journalctl returns as array of bytes, if it is not valid UTF-8.
func journalMessage(message any) string {
	raw, ok := message.([]any)
	if !ok {
		return fmt.Sprint(message)
	}

	content := make([]byte, 0, len(raw))
	for _, b := range raw {
		value, ok := b.(float64)
		if ok {
			content = append(content, byte(value))
		}
	}

	return string(content)
}
