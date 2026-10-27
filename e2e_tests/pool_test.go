package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_observeServers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		instances []poolInstance
		records   []serverRecord

		want map[string]observedServer
	}{
		{
			name: "nothing exists",

			want: map[string]observedServer{
				"IncusOS01": {},
			},
		},
		{
			name: "record with the name of the VM",
			instances: []poolInstance{
				{Name: "IncusOS01", Status: "Running", Hostname: "abc"},
				{Name: "OperationsCenter", Status: "Running", Hostname: "def"},
			},
			records: []serverRecord{
				{Name: "IncusOS01", Status: "ready"},
				{Name: "abc", Status: "ready"},
			},

			want: map[string]observedServer{
				"IncusOS01": {
					instance: poolInstance{Name: "IncusOS01", Status: "Running", Hostname: "abc"},
					record:   serverRecord{Name: "IncusOS01", Status: "ready"},
				},
			},
		},
		{
			name: "record with the hostname of the VM",
			instances: []poolInstance{
				{Name: "IncusOS01", Status: "Running", Hostname: "abc"},
			},
			records: []serverRecord{
				{Name: "abc", Status: "ready"},
			},

			want: map[string]observedServer{
				"IncusOS01": {
					instance: poolInstance{Name: "IncusOS01", Status: "Running", Hostname: "abc"},
					record:   serverRecord{Name: "abc", Status: "ready"},
				},
			},
		},
		{
			name: "record without VM",
			records: []serverRecord{
				{Name: "IncusOS01", Status: "offline", Cluster: "incus-os-cluster"},
			},

			want: map[string]observedServer{
				"IncusOS01": {
					record: serverRecord{Name: "IncusOS01", Status: "offline", Cluster: "incus-os-cluster"},
				},
			},
		},
		{
			name: "stopped VM without hostname does not match a record without name",
			instances: []poolInstance{
				{Name: "IncusOS01", Status: "Stopped"},
			},
			records: []serverRecord{
				{Name: "", Status: "ready"},
			},

			want: map[string]observedServer{
				"IncusOS01": {
					instance: poolInstance{Name: "IncusOS01", Status: "Stopped"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := observeServers([]string{"IncusOS01"}, tc.instances, tc.records)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_nextServerAction(t *testing.T) {
	t.Parallel()

	running := poolInstance{Name: "IncusOS01", Status: "Running", Hostname: "abc"}

	tests := []struct {
		name   string
		server observedServer
		dirty  bool

		want serverAction
	}{
		{
			name: "ready",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "IncusOS01", Status: "ready"},
			},

			want: serverActionNone,
		},
		{
			name: "registered with hostname",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "abc", Status: "ready"},
			},

			want: serverActionRename,
		},
		{
			name: "dirty",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "IncusOS01", Status: "ready"},
			},
			dirty: true,

			want: serverActionReinstall,
		},
		{
			name: "VM missing",
			server: observedServer{
				record: serverRecord{Name: "IncusOS01", Status: "ready"},
			},

			want: serverActionReinstall,
		},
		{
			name: "VM stopped",
			server: observedServer{
				instance: poolInstance{Name: "IncusOS01", Status: "Stopped"},
				record:   serverRecord{Name: "IncusOS01", Status: "ready"},
			},

			want: serverActionReinstall,
		},
		{
			name: "record missing",
			server: observedServer{
				instance: running,
			},

			want: serverActionReinstall,
		},
		{
			name: "server offline",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "IncusOS01", Status: "offline"},
			},

			want: serverActionReinstall,
		},
		{
			name: "clustered",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "IncusOS01", Status: "ready", Cluster: "incus-os-cluster"},
			},

			want: serverActionFactoryReset,
		},
		{
			name: "clustered with hostname",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "abc", Status: "ready", Cluster: "incus-os-cluster"},
			},

			want: serverActionFactoryReset,
		},
		{
			name: "clustered and offline",
			server: observedServer{
				instance: running,
				record:   serverRecord{Name: "IncusOS01", Status: "offline", Cluster: "incus-os-cluster"},
			},

			want: serverActionReinstall,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := nextServerAction("IncusOS01", tc.server, tc.dirty)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_planStandalone(t *testing.T) {
	t.Parallel()

	pool := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	ready := func(name string) observedServer {
		return observedServer{
			instance: poolInstance{Name: name, Status: "Running", Hostname: "host-" + name},
			record:   serverRecord{Name: name, Status: "ready"},
		}
	}

	clustered := func(name string) observedServer {
		server := ready(name)
		server.record.Cluster = "incus-os-cluster"

		return server
	}

	tests := []struct {
		name     string
		names    []string
		observed map[string]observedServer
		dirty    map[string]bool

		want poolPlan
	}{
		{
			name:  "all ready",
			names: pool,
			observed: map[string]observedServer{
				"IncusOS01": ready("IncusOS01"),
				"IncusOS02": ready("IncusOS02"),
				"IncusOS03": ready("IncusOS03"),
			},

			want: poolPlan{},
		},
		{
			name:     "empty pool installs all the servers",
			names:    []string{"IncusOS01"},
			observed: map[string]observedServer{},

			want: poolPlan{
				reinstall: pool,
			},
		},
		{
			name:  "existing VMs, which are not required, are kept",
			names: []string{"IncusOS01"},
			observed: map[string]observedServer{
				"IncusOS02": clustered("IncusOS02"),
			},

			want: poolPlan{
				reinstall: []string{"IncusOS01", "IncusOS03"},
			},
		},
		{
			name:  "missing VMs are only installed with a required install",
			names: []string{"IncusOS01"},
			observed: map[string]observedServer{
				"IncusOS01": ready("IncusOS01"),
			},

			want: poolPlan{},
		},
		{
			name:  "cluster is factory reset once with all its members",
			names: []string{"IncusOS01", "IncusOS02"},
			observed: map[string]observedServer{
				"IncusOS01": clustered("IncusOS01"),
				"IncusOS02": clustered("IncusOS02"),
				"IncusOS03": ready("IncusOS03"),
			},

			want: poolPlan{
				factoryReset: map[string][]string{
					"incus-os-cluster": {"IncusOS01", "IncusOS02"},
				},
			},
		},
		{
			name:  "dirty member removes the cluster",
			names: pool,
			observed: map[string]observedServer{
				"IncusOS01": clustered("IncusOS01"),
				"IncusOS02": clustered("IncusOS02"),
				"IncusOS03": clustered("IncusOS03"),
			},
			dirty: map[string]bool{
				"IncusOS02": true,
			},

			want: poolPlan{
				reinstall: pool,
			},
		},
		{
			name:  "registered with hostname after factory reset",
			names: []string{"IncusOS01", "IncusOS02"},
			observed: map[string]observedServer{
				"IncusOS01": {
					instance: poolInstance{Name: "IncusOS01", Status: "Running", Hostname: "host-IncusOS01"},
					record:   serverRecord{Name: "host-IncusOS01", Status: "ready"},
				},
				"IncusOS02": ready("IncusOS02"),
				"IncusOS03": ready("IncusOS03"),
			},

			want: poolPlan{
				rename: []string{"IncusOS01"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := planStandalone(tc.names, pool, tc.observed, tc.dirty)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_clusterReady(t *testing.T) {
	t.Parallel()

	member := func(name string) observedServer {
		return observedServer{
			instance: poolInstance{Name: name, Status: "Running"},
			record:   serverRecord{Name: name, Status: "ready", Cluster: "incus-os-cluster"},
		}
	}

	tests := []struct {
		name     string
		observed map[string]observedServer
		dirty    map[string]bool

		want bool
	}{
		{
			name: "ready",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": member("IncusOS02"),
			},

			want: true,
		},
		{
			name: "additional member",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": member("IncusOS02"),
				"IncusOS03": member("IncusOS03"),
			},

			want: false,
		},
		{
			name: "missing member",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": {
					instance: poolInstance{Name: "IncusOS02", Status: "Running"},
					record:   serverRecord{Name: "IncusOS02", Status: "ready"},
				},
			},

			want: false,
		},
		{
			name: "member of another cluster",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": {
					instance: poolInstance{Name: "IncusOS02", Status: "Running"},
					record:   serverRecord{Name: "IncusOS02", Status: "ready", Cluster: "other"},
				},
				"IncusOS03": member("IncusOS03"),
			},

			want: false,
		},
		{
			name: "member offline",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": {
					instance: poolInstance{Name: "IncusOS02", Status: "Running"},
					record:   serverRecord{Name: "IncusOS02", Status: "offline", Cluster: "incus-os-cluster"},
				},
			},

			want: false,
		},
		{
			name: "member stopped",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": {
					instance: poolInstance{Name: "IncusOS02", Status: "Stopped"},
					record:   serverRecord{Name: "IncusOS02", Status: "ready", Cluster: "incus-os-cluster"},
				},
			},

			want: false,
		},
		{
			name: "member dirty",
			observed: map[string]observedServer{
				"IncusOS01": member("IncusOS01"),
				"IncusOS02": member("IncusOS02"),
			},
			dirty: map[string]bool{
				"IncusOS02": true,
			},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := clusterReady("incus-os-cluster", []string{"IncusOS01", "IncusOS02"}, tc.observed, tc.dirty)

			require.Equal(t, tc.want, got)
		})
	}
}
