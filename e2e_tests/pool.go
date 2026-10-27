package e2e

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// serverPool is a set of IncusOS servers, which the scenarios of a suite share.
// Before a scenario runs, the pool observes the state of the servers and brings
// the servers, which the scenario requires, into the required state.
type serverPool struct {
	// suite is the test of the suite. The pool registers its cleanup there.
	suite *testing.T
	names []string

	// channel is the update channel of the cluster. It matches the channel of
	// the token, which installs the servers.
	channel string

	// prepareISO returns the name of the ISO storage volume, which installs a
	// server of the pool.
	prepareISO func(ctx context.Context, t *testing.T, tmpDir string) string

	// dirty holds the servers, which the pool installs again before their next
	// use. A failed scenario might have left them in a bad state.
	dirty             map[string]bool
	cleanupRegistered bool
}

func newServerPool(t *testing.T, names []string, channel string, prepareISO func(ctx context.Context, t *testing.T, tmpDir string) string) *serverPool {
	t.Helper()

	return &serverPool{
		suite:      t,
		names:      names,
		channel:    channel,
		prepareISO: prepareISO,
		dirty:      map[string]bool{},
	}
}

// markDirty makes the pool install the given servers again before their next
// use. The pool does not keep this state across runs.
func (p *serverPool) markDirty(names []string) {
	for _, name := range names {
		p.dirty[name] = true
	}
}

// ensureStandalone brings the given servers into the state "ready and not
// clustered". The server records carry the names of the VMs.
func (p *serverPool) ensureStandalone(ctx context.Context, t *testing.T, tmpDir string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	require.Subset(t, p.names, names, "expect the servers to be part of the pool")

	p.registerCleanup()

	observed := mustObserveServers(t, p.names)
	plan := planStandalone(names, p.names, observed, p.dirty)

	for cluster, members := range plan.factoryReset {
		t.Logf("Factory reset cluster %q to reuse %v", cluster, members)

		mustFactoryResetCluster(ctx, t, cluster, members)
	}

	if len(plan.reinstall) > 0 {
		t.Logf("Installing %v again", plan.reinstall)

		mustRemoveServers(ctx, t, plan.reinstall, observed)

		iso := p.prepareISO(ctx, t, tmpDir)

		createIncusOSInstances(ctx, t, iso, plan.reinstall)

		for _, name := range plan.reinstall {
			delete(p.dirty, name)
		}
	}

	if len(plan.rename) > 0 {
		renameServers(t, plan.rename)
	}

	observed = mustObserveServers(t, p.names)
	require.Equal(t, poolPlan{}, planStandalone(names, p.names, observed, p.dirty), "expect the servers %v to be ready and not clustered", names)

	printServerList(t)
}

// ensureCluster brings the given servers into the state "members of the
// cluster incus-os-cluster". The cluster has no other members and uses the
// update channel of the pool.
func (p *serverPool) ensureCluster(ctx context.Context, t *testing.T, tmpDir string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	require.Subset(t, p.names, names, "expect the servers to be part of the pool")

	p.registerCleanup()

	observed := mustObserveServers(t, p.names)
	if !clusterReady(incusOSClusterName, names, observed, p.dirty) {
		p.ensureStandalone(ctx, t, tmpDir, names)

		mustCreateCluster(t, tmpDir, p.channel, names)
	}

	mustAddIncusRemote(t, incusOSClusterName)
}

// registerCleanup registers the cleanup of the pool on the suite. The pool
// registers it lazily, so a suite, which runs no scenario, cleans up nothing.
func (p *serverPool) registerCleanup() {
	if p.cleanupRegistered {
		return
	}

	// The cleanups run in the reverse order. The servers can only be removed,
	// after their cluster is gone.
	p.suite.Cleanup(cleanupIncusOS(p.suite, p.names))
	p.suite.Cleanup(clusterCleanup(p.suite))

	p.cleanupRegistered = true
}

// mustFactoryResetCluster factory resets the cluster and gives the members the
// names of their VMs again.
func mustFactoryResetCluster(ctx context.Context, t *testing.T, cluster string, members []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	_, hostnames := mustGetInstanceIPAndNames(t, members)

	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset %s`, cpuArch, cluster)

	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(ctx, t, members)

	// The servers register with their hostname again.
	mustWaitInventoryReady(ctx, t, hostnames)

	renameServers(t, members)
}

// mustRemoveServers removes the VMs of the given servers together with their
// records in Operations Center. A clustered server is removed together with
// its cluster.
func mustRemoveServers(ctx context.Context, t *testing.T, names []string, observed map[string]observedServer) {
	t.Helper()

	removedClusters := map[string]bool{}

	for _, name := range names {
		server := observed[name]

		// Remove the VM first, so it does not update its record again.
		if server.instance.Status != "" {
			require.NoError(t, removeInstanceWithContext(ctx, t, name))
		}

		switch {
		case server.record.Cluster != "":
			if !removedClusters[server.record.Cluster] {
				mustRun(t, `../bin/operations-center.linux.%s provisioning cluster remove %s --force`, cpuArch, server.record.Cluster)
				removedClusters[server.record.Cluster] = true
			}

		case server.record.Name != "":
			mustRun(t, `../bin/operations-center.linux.%s provisioning server remove %s`, cpuArch, server.record.Name)
		}
	}
}

// poolInstance is an Incus instance as reported by "incus list".
type poolInstance struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Hostname string `json:"hostname"`
}

// serverRecord is a server as reported by "provisioning server list".
type serverRecord struct {
	Name    string `json:"name"`
	Status  string `json:"server_status"`
	Cluster string `json:"cluster"`
}

// observedServer is the state of a server of the pool. A zero instance means,
// that the VM does not exist. A zero record means, that Operations Center does
// not know the server.
type observedServer struct {
	instance poolInstance
	record   serverRecord
}

func mustObserveServers(t *testing.T, names []string) map[string]observedServer {
	t.Helper()

	instancesResp := mustRun(t, `incus list -f json | jq -c '[ .[] | { name: .name, status: .status, hostname: (.state.os_info.hostname // "") } ]'`)

	var instances []poolInstance
	err := json.Unmarshal([]byte(instancesResp.OutputTrimmed()), &instances)
	require.NoError(t, err)

	recordsResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -c '[ .[] | select(.server_type == "incus") ]'`, cpuArch)

	var records []serverRecord
	err = json.Unmarshal([]byte(recordsResp.OutputTrimmed()), &records)
	require.NoError(t, err)

	return observeServers(names, instances, records)
}

// observeServers matches the VMs of the given servers with their records. A
// server, which registered again after a factory reset, has a record with the
// hostname of its VM.
func observeServers(names []string, instances []poolInstance, records []serverRecord) map[string]observedServer {
	observed := make(map[string]observedServer, len(names))

	for _, name := range names {
		var server observedServer

		i := slices.IndexFunc(instances, func(instance poolInstance) bool { return instance.Name == name })
		if i >= 0 {
			server.instance = instances[i]
		}

		i = slices.IndexFunc(records, func(record serverRecord) bool { return record.Name == name })
		if i < 0 && server.instance.Hostname != "" {
			i = slices.IndexFunc(records, func(record serverRecord) bool { return record.Name == server.instance.Hostname })
		}

		if i >= 0 {
			server.record = records[i]
		}

		observed[name] = server
	}

	return observed
}

type serverAction int

const (
	serverActionNone serverAction = iota
	serverActionRename
	serverActionFactoryReset
	serverActionReinstall
)

// nextServerAction returns the action, which brings the server name into the
// state "ready and not clustered".
func nextServerAction(name string, server observedServer, dirty bool) serverAction {
	switch {
	case dirty,
		server.instance.Status != instanceStatusRunning,
		// Removing a cluster from Operations Center also removes the records
		// of its members, but their VMs stay clustered in Incus.
		server.record.Name == "",
		server.record.Status != "ready":
		return serverActionReinstall

	case server.record.Cluster != "":
		return serverActionFactoryReset

	case server.record.Name != name:
		return serverActionRename

	default:
		return serverActionNone
	}
}

// poolPlan lists the steps, which bring servers into the state "ready and not
// clustered". The steps run in the order of the fields.
type poolPlan struct {
	// factoryReset maps the clusters to reset to their members in the pool.
	factoryReset map[string][]string
	reinstall    []string
	rename       []string
}

// planStandalone plans the steps, which bring the given servers into the
// state "ready and not clustered". pool lists all the servers of the pool.
func planStandalone(names []string, pool []string, observed map[string]observedServer, dirty map[string]bool) poolPlan {
	actions := make(map[string]serverAction, len(pool))
	reinstall := false

	for _, name := range names {
		actions[name] = nextServerAction(name, observed[name], dirty[name])
		if actions[name] == serverActionReinstall {
			reinstall = true
		}
	}

	// Concurrent installs take about as long as a single one, so the missing
	// VMs of the pool are installed together with the first install.
	if reinstall {
		for _, name := range pool {
			if observed[name].instance.Status == "" {
				actions[name] = serverActionReinstall
			}
		}
	}

	// Installing a clustered server again removes its cluster, so the other
	// members of the cluster can not be factory reset anymore.
	removedClusters := map[string]bool{}
	for name, action := range actions {
		if action == serverActionReinstall && observed[name].record.Cluster != "" {
			removedClusters[observed[name].record.Cluster] = true
		}
	}

	var plan poolPlan

	for _, name := range pool {
		action, ok := actions[name]
		if !ok {
			continue
		}

		cluster := observed[name].record.Cluster
		if action == serverActionFactoryReset && removedClusters[cluster] {
			action = serverActionReinstall
		}

		switch action {
		case serverActionReinstall:
			plan.reinstall = append(plan.reinstall, name)

		case serverActionFactoryReset:
			_, ok := plan.factoryReset[cluster]
			if ok {
				continue
			}

			if plan.factoryReset == nil {
				plan.factoryReset = map[string][]string{}
			}

			for _, member := range pool {
				if observed[member].record.Cluster == cluster {
					plan.factoryReset[cluster] = append(plan.factoryReset[cluster], member)
				}
			}

		case serverActionRename:
			plan.rename = append(plan.rename, name)

		case serverActionNone:
		}
	}

	return plan
}

// clusterReady reports, whether the cluster consists of exactly the given
// servers and all of them are ready.
func clusterReady(cluster string, names []string, observed map[string]observedServer, dirty map[string]bool) bool {
	members := 0
	for _, server := range observed {
		if server.record.Cluster == cluster {
			members++
		}
	}

	if members != len(names) {
		return false
	}

	for _, name := range names {
		server := observed[name]

		if dirty[name] ||
			server.instance.Status != instanceStatusRunning ||
			server.record.Name != name ||
			server.record.Status != "ready" ||
			server.record.Cluster != cluster {
			return false
		}
	}

	return true
}
