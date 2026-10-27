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
