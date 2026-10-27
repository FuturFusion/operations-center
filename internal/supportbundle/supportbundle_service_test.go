package supportbundle_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusclient "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/supportbundle"
	adapterMock "github.com/FuturFusion/operations-center/internal/supportbundle/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/supportbundle/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSupportBundleService_Create(t *testing.T) {
	servers := provisioning.Servers{
		{Name: "reachable", Cluster: new("one"), Type: api.ServerTypeIncus},
		{Name: "unreachable", Cluster: new("one"), Type: api.ServerTypeIncus},
	}

	clusterSvc := &mock.ProvisioningClusterServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
			return &provisioning.Cluster{Name: name}, nil
		},
	}

	serverSvc := &mock.ProvisioningServerServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
			return servers, nil
		},
	}

	client := &adapterMock.SupportBundleClientPortMock{
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			if endpoint.GetName() == "unreachable" {
				return boom.Error
			}

			return nil
		},
		IncusClientFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (provisioning.InstanceServer, error) {
			return nil, boom.Error
		},
		GetOSFunc: func(ctx context.Context, server provisioning.Server) (json.RawMessage, error) {
			return json.RawMessage(`{"environment":{"os_name":"IncusOS"}}`), nil
		},
		GetSystemFunc: func(ctx context.Context, server provisioning.Server, resource string) (map[string]any, error) {
			return map[string]any{"config": map[string]any{"encryption_recovery_keys": []any{"secret"}}}, nil
		},
		GetOSApplicationNamesFunc: func(ctx context.Context, server provisioning.Server) ([]string, error) {
			return []string{"incus"}, nil
		},
		GetOSApplicationFunc: func(ctx context.Context, server provisioning.Server, name string) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
		GetOSDebugLogFunc: func(ctx context.Context, server provisioning.Server, entries int) ([]map[string]any, error) {
			return []map[string]any{{"__REALTIME_TIMESTAMP": "1760600000000000", "SYSLOG_IDENTIFIER": "incus-osd", "_PID": "1", "MESSAGE": "System is ready"}}, nil
		},
		GetOSDebugKernelFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.DebugKernel, error) {
			return incusosapi.DebugKernel{}, boom.Error
		},
		GetOSDebugProcessesFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
			return "PID COMMAND\n", nil
		},
		GetOSApplicationDebugLogsFunc: func(ctx context.Context, server provisioning.Server, application string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("logs")), nil
		},
	}

	svc := supportbundle.New(clusterSvc, serverSvc, client)

	_, _, err := svc.Create(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeInstance, Cluster: "one", Project: "default", Name: "../../evil"})
	require.ErrorContains(t, err, `name "../../evil" is not a valid name`)

	filename, rc, err := svc.Create(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeCluster, Cluster: "one"})
	require.NoError(t, err)

	files := readBundle(t, filename, rc)

	require.Equal(t, "logs", files["servers/reachable/os/debug/incus-logs.tar.gz"])
	require.Contains(t, files["servers/reachable/os/debug/journal.txt"], "incus-osd[1]: System is ready")
	require.Contains(t, files["servers/reachable/os/system/security.json"], `"[REDACTED]"`)
	require.Contains(t, files["servers/reachable/os/debug/kernel.json.error.txt"], boom.Error.Error())
	require.Contains(t, files["cluster/members.json.error.txt"], boom.Error.Error())
	require.Contains(t, files["servers/unreachable/unreachable.error.txt"], boom.Error.Error())
	require.NotContains(t, files, "servers/unreachable/os/os.json")

	var m struct {
		Sections []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"sections"`
	}

	err = json.Unmarshal([]byte(files["manifest.json"]), &m)
	require.NoError(t, err)

	statuses := map[string]string{}
	for _, section := range m.Sections {
		statuses[section.Path] = section.Status
	}

	require.Equal(t, "ok", statuses["servers/reachable/os/os.json"])
	require.Equal(t, "error", statuses["servers/reachable/os/debug/kernel.json"])
	require.Equal(t, "skipped", statuses["servers/unreachable/unreachable"])
}

func TestSupportBundleService_Create_projectAndInstance(t *testing.T) {
	servers := provisioning.Servers{
		{Name: "reachable", Cluster: new("one"), Type: api.ServerTypeIncus},
		{Name: "unreachable", Cluster: new("one"), Type: api.ServerTypeIncus},
	}

	clusterSvc := &mock.ProvisioningClusterServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
			return &provisioning.Cluster{Name: name}, nil
		},
	}

	serverSvc := &mock.ProvisioningServerServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
			return servers, nil
		},
	}

	// The instance is located on the unreachable server, so no server data is collected.
	instance := incusapi.InstanceFull{
		Instance: incusapi.Instance{
			Name:     "c1",
			Type:     string(incusapi.InstanceTypeVM),
			Location: "unreachable",
			ExpandedDevices: map[string]map[string]string{
				"root": {"type": "disk", "pool": "default", "path": "/"},
				"data": {"type": "disk", "pool": "default", "source": "missing"},
				"eth0": {"type": "nic", "network": "incusbr0"},
				"eth1": {"type": "nic", "nictype": "macvlan", "parent": "enp5s0"},
			},
		},
	}

	incusClient := &provisioningMock.InstanceServerMock{
		GetServerFunc: func() (*incusapi.Server, string, error) {
			return nil, "", boom.Error
		},
		GetClusterFunc: func() (*incusapi.Cluster, string, error) {
			return nil, "", boom.Error
		},
		GetClusterMembersFunc: func() ([]incusapi.ClusterMember, error) {
			return []incusapi.ClusterMember{{ServerName: "reachable"}, {ServerName: "foreign"}, {ServerName: "../../../evil"}}, nil
		},
		GetClusterMemberStateFunc: func(name string) (*incusapi.ClusterMemberState, string, error) {
			return &incusapi.ClusterMemberState{}, "", nil
		},
		GetWarningsFunc: func() ([]incusapi.Warning, error) {
			return nil, nil
		},
		GetProjectFunc: func(name string) (*incusapi.Project, string, error) {
			return &incusapi.Project{Name: name}, "", nil
		},
		GetProjectStateFunc: func(name string) (*incusapi.ProjectState, error) {
			return &incusapi.ProjectState{}, nil
		},
		GetInstancesFullFunc: func(instanceType incusapi.InstanceType) ([]incusapi.InstanceFull, error) {
			return []incusapi.InstanceFull{instance}, nil
		},
		GetProfilesFunc: func() ([]incusapi.Profile, error) {
			return nil, nil
		},
		GetNetworksFunc: func() ([]incusapi.Network, error) {
			return nil, nil
		},
		GetStoragePoolsFunc: func() ([]incusapi.StoragePool, error) {
			return []incusapi.StoragePool{{Name: "default"}}, nil
		},
		GetStoragePoolVolumesFunc: func(pool string) ([]incusapi.StorageVolume, error) {
			return nil, boom.Error
		},
		GetImagesFunc: func() ([]incusapi.Image, error) {
			return nil, nil
		},
		GetInstanceFunc: func(name string) (*incusapi.Instance, string, error) {
			if name != instance.Name {
				return nil, "", incusapi.StatusErrorf(http.StatusNotFound, "Instance not found")
			}

			return &instance.Instance, "", nil
		},
		GetInstanceFullFunc: func(name string) (*incusapi.InstanceFull, string, error) {
			return &instance, "", nil
		},
		GetInstanceSnapshotsFunc: func(instanceName string) ([]incusapi.InstanceSnapshot, error) {
			return nil, nil
		},
		GetStoragePoolVolumeFunc: func(pool string, volType string, name string) (*incusapi.StorageVolume, string, error) {
			if name == "missing" {
				return nil, "", boom.Error
			}

			return &incusapi.StorageVolume{Name: name, Type: volType}, "", nil
		},
		GetNetworkFunc: func(name string) (*incusapi.Network, string, error) {
			return &incusapi.Network{Name: name}, "", nil
		},
		GetInstanceLogfilesFunc: func(name string) ([]string, error) {
			return []string{"/1.0/instances/c1/logs/qemu.log"}, nil
		},
	}

	incusClient.UseProjectFunc = func(name string) incusclient.InstanceServer { return incusClient }
	incusClient.UseTargetFunc = func(name string) incusclient.InstanceServer { return incusClient }

	client := &adapterMock.SupportBundleClientPortMock{
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			if endpoint.GetName() == "unreachable" {
				return boom.Error
			}

			return nil
		},
		IncusClientFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (provisioning.InstanceServer, error) {
			return incusClient, nil
		},
		GetInstanceLogfileFunc: func(ctx context.Context, server provisioning.Server, project string, name string, filename string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("qemu log")), nil
		},
		GetInstanceConsoleLogFunc: func(ctx context.Context, server provisioning.Server, project string, name string) (io.ReadCloser, error) {
			return nil, boom.Error
		},
	}

	svc := supportbundle.New(clusterSvc, serverSvc, client)

	t.Run("project", func(t *testing.T) {
		filename, rc, err := svc.Create(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeProject, Cluster: "one", Project: "default"})
		require.NoError(t, err)

		files := readBundle(t, filename, rc)

		require.Contains(t, files["cluster/members/foreign/state.json"], "{")
		require.Contains(t, files["manifest.json"], `Cluster member \"foreign\" is not known to Operations Center`)
		require.Contains(t, files["manifest.json"], "Invalid section path")
		require.Contains(t, files["projects/default/project.json"], `"name": "default"`)
		require.Contains(t, files["projects/default/instances.json"], `"name": "c1"`)
		require.Contains(t, files["projects/default/storage-volumes.json.error.txt"], `Failed to get volumes of storage pool "default"`)
		require.Contains(t, files["servers/unreachable/unreachable.error.txt"], boom.Error.Error())
		require.NotContains(t, files, "servers/reachable/os/os.json")
	})

	t.Run("instance", func(t *testing.T) {
		filename, rc, err := svc.Create(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeInstance, Cluster: "one", Project: "default", Name: "c1"})
		require.NoError(t, err)

		files := readBundle(t, filename, rc)

		require.Contains(t, files["instances/default/c1/instance.json"], `"name": "c1"`)
		require.Contains(t, files["instances/default/c1/volumes.json"], `"type": "virtual-machine"`)
		require.Contains(t, files["instances/default/c1/volumes.json"], `"error": "boom!"`)
		require.Contains(t, files["instances/default/c1/networks.json"], `"name": "incusbr0"`)
		require.NotContains(t, files["instances/default/c1/networks.json"], "eth1")
		require.Equal(t, "qemu log", files["instances/default/c1/logs/qemu.log"])
		require.Contains(t, files["instances/default/c1/console.log.error.txt"], boom.Error.Error())
		require.Contains(t, files["servers/unreachable/unreachable.error.txt"], boom.Error.Error())
		require.NotContains(t, files, "servers/reachable/os/os.json")
	})

	t.Run("instance not found", func(t *testing.T) {
		_, _, err := svc.Create(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeInstance, Cluster: "one", Project: "default", Name: "c2"})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

// readBundle returns the content of the files of a support bundle by their path below the root of the archive.
func readBundle(t *testing.T, filename string, rc io.ReadCloser) map[string]string {
	t.Helper()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	files := map[string]string{}
	for _, f := range archive.File {
		require.True(t, strings.HasPrefix(f.Name, strings.TrimSuffix(filename, ".zip")+"/"))
		_, name, _ := strings.Cut(f.Name, "/")

		r, err := f.Open()
		require.NoError(t, err)

		data, err := io.ReadAll(r)
		require.NoError(t, err)

		files[name] = string(data)
	}

	return files
}
