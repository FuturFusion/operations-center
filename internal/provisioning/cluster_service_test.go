package provisioning_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	serviceMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterService_Create(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{}, nil)

	updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

	tests := []struct {
		name                                              string
		cluster                                           provisioning.Cluster
		repoExistsByName                                  bool
		repoExistsByNameErr                               error
		repoCreateErr                                     error
		repoUpdateErr                                     error
		localArtifactRepoCreateClusterArtifactFromPathErr error
		clientPingErr                                     error
		clientUpdateOSServiceErr                          error
		clientSetServerConfig                             []queue.Item[struct{}]
		clientEnableClusterCertificate                    string
		clientEnableClusterErr                            error
		clientGetClusterNodeNamesErr                      error
		clientGetClusterJoinToken                         string
		clientGetClusterJoinTokenErr                      error
		clientJoinClusterErr                              error
		clientGetOSData                                   api.OSData
		clientGetOSDataErr                                error
		clientGetRemoteCertificateErr                     error
		serverSvcGetByName                                []queue.Item[*provisioning.Server]
		serverSvcUpdateErr                                error
		serverSvcUpdateSystemUpdateErr                    error
		serverSvcPollServerErr                            error
		serverSvcGetAllWithFilter                         provisioning.Servers
		serverSvcGetAllWithFilterErr                      error
		provisionerApply                                  []queue.Item[struct{}]
		provisionerInitErr                                error
		provisionerSeedCertificateErr                     error
		inventorySyncerSyncClusterErr                     error

		assertErr     require.ErrorAssertionFunc
		signalHandler func(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage)
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
				ServerType:  api.ServerTypeIncus,
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetOSData:                api.OSData{},
			provisionerApply: []queue.Item[struct{}]{
				{}, // success
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:        "", // invalid
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - repo.ExistsByName cluster already exists",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByName: true, // cluster with the same name already exists

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Cluster with name "one" already exists`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - repo.ExistsByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - serverSvc.GetByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server already part of cluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // already part of cluster.
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" is already part of cluster "cluster-foo"`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server not in ready state",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusPending, // server not in ready state
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" is not in ready state and can therefore not be used for clustering`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server not in same update channel",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "testing", // channel does not match cluster's update channel
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" update channel "testing" does not match channel requested for cluster "stable"`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server requires update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
						VersionData: api.ServerVersionData{
							NeedsUpdate: ptr.To(true), // server requires update
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" not ready to be clustered (needs update: true, needs reboot: false, in maintenance: not in maintenance)`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - repo.Create",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientEnableClusterCertificate: "certificate",
			repoCreateErr:                  boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server has wrong type",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeMigrationManager, // wrong type, incus expected.
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" has type "migration-manager" but "incus" was expected`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.Ping",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientPingErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - invalid os service config",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": []string{}, // invalid, not a map[string]any

				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": config is not an object`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - lvm enabled not bool",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": "", // invalid, not bool
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": "enabled" is not a bool`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - invalid os service config",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						ID:      2001, // invalid, server ID must not be > 2000 for LVM system_id.
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": can not enable LVM on servers with internal ID > 2000`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.UpdateOSService",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientUpdateOSServiceErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.SetServerConfig",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				// Server 1
				{
					Err: boom.Error,
				},
			},

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.EnableCluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.GetClusterNodeNames",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetClusterNodeNamesErr:   boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.GetClusterJoinToken",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetClusterJoinTokenErr:   boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.JoinCluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientJoinClusterErr:           boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - serverSvc.GetByName - 2nd transaction",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Err: boom.Error,
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - server already part of cluster - 2nd transaction",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // added to a cluster since the first check.
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" was not part of a cluster, but is now part of "cluster-foo"`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - repo.Update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			repoUpdateErr:                  boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - serverSvc.Update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			serverSvcUpdateErr:             boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - client.GetOSData",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetOSDataErr:             boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Init",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerInitErr:             boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry three times",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
			},

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry - serverSvc.PollServer",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcPollServerErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry - serverSvc.GetAllWithFilter",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry - serverSvc.GetAllWithFilter - none nill certificate",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{
					ClusterCertificate: ptr.To("none nil"), // none nil certificate
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Cluster certificate is not nil after polling the server, but we expected a publicly valid certificate")
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry - client.GetRemoteCertificate",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
			},
			clientGetRemoteCertificateErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - provisioner.Apply - retry - serverSvc.SeedCertificate",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{
					Err: domain.NewRetryableErr(boom.Error), // retryable error
				},
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
			},
			provisionerSeedCertificateErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - localArtifactRepo.CreateClusterArtifactFromPath",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{}, // success
			},
			localArtifactRepoCreateClusterArtifactFromPathErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name: "error - inventory syncer error",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server1",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "server2",
						Type:    api.ServerTypeIncus,
						Status:  api.ServerStatusReady,
						Channel: "stable",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApply: []queue.Item[struct{}]{
				{}, // success
			},
			inventorySyncerSyncClusterErr: boom.Error,

			assertErr:     require.NoError, // inventory syncer error is just logged and does not fail cluster creation.
			signalHandler: requireCallSignalHandler,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				ExistsByNameFunc: func(ctx context.Context, name string) (bool, error) {
					return tc.repoExistsByName, tc.repoExistsByNameErr
				},
				CreateFunc: func(ctx context.Context, in provisioning.Cluster) (int64, error) {
					return 0, tc.repoCreateErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			localArtifactRepo := &mock.ClusterArtifactRepoMock{
				CreateClusterArtifactFromPathFunc: func(ctx context.Context, artifact provisioning.ClusterArtifact, path string, ignoredFiles []string) (int64, error) {
					return 0, tc.localArtifactRepoCreateClusterArtifactFromPathErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					return tc.clientUpdateOSServiceErr
				},
				SetServerConfigFunc: func(ctx context.Context, endpoint provisioning.Endpoint, config map[string]string) error {
					_, err := queue.Pop(t, &tc.clientSetServerConfig)
					return err
				},
				EnableClusterFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
					return tc.clientEnableClusterCertificate, tc.clientEnableClusterErr
				},
				GetClusterNodeNamesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) ([]string, error) {
					return []string{"one"}, tc.clientGetClusterNodeNamesErr
				},
				GetClusterJoinTokenFunc: func(ctx context.Context, endpoint provisioning.Endpoint, memberName string) (string, error) {
					return tc.clientGetClusterJoinToken, tc.clientGetClusterJoinTokenErr
				},
				JoinClusterFunc: func(ctx context.Context, server provisioning.Server, joinToken string, endpoint provisioning.Endpoint) error {
					return tc.clientJoinClusterErr
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return tc.clientGetOSData, tc.clientGetOSDataErr
				},
				GetRemoteCertificateFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (*x509.Certificate, error) {
					return &x509.Certificate{}, tc.clientGetRemoteCertificateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					server, err := queue.Pop(t, &tc.serverSvcGetByName)
					return server, err
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server, force bool, updateSystem bool) error {
					return tc.serverSvcUpdateErr
				},
				UpdateSystemUpdateFunc: func(ctx context.Context, name string, updateConfig provisioning.ServerSystemUpdate) error {
					return tc.serverSvcUpdateSystemUpdateErr
				},
				PollServerFunc: func(ctx context.Context, server provisioning.Server, updateServerConfiguration bool) error {
					return tc.serverSvcPollServerErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			provisioner := &adapterMock.ClusterProvisioningPortMock{
				InitFunc: func(ctx context.Context, clusterName string, config provisioning.ClusterProvisioningConfig) (string, func() error, error) {
					return "", func() error { return nil }, tc.provisionerInitErr
				},
				ApplyFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					_, err := queue.Pop(t, &tc.provisionerApply)
					return err
				},
				SeedCertificateFunc: func(ctx context.Context, clusterName string, certificate string) error {
					return tc.provisionerSeedCertificateErr
				},
			}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerSyncClusterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(
				repo,
				localArtifactRepo,
				client,
				serverSvc,
				nil,
				map[domain.ResourceType]provisioning.InventorySyncer{domain.ResourceTypeImage: inventorySyncer},
				provisioner,
				provisioning.WithClusterServiceCreateClusterRetryTimeout(0),
				provisioning.WithClusterServiceUpdateSignal(updateSignal),
			)

			var signalHandlerCalled bool
			updateSignal.AddListener(tc.signalHandler(t, &signalHandlerCalled))

			// Run test
			_, err := clusterSvc.Create(context.Background(), tc.cluster)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.clientSetServerConfig)
			require.Empty(t, tc.serverSvcGetByName)
			require.Empty(t, tc.provisionerApply)
			require.True(t, signalHandlerCalled, "expected signal handler to called, but it was not OR no call was expected, but it got called")
		})
	}
}

func TestClusterService_GetAll(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllClusters provisioning.Clusters
		repoGetAllErr      error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllClusters: provisioning.Clusters{
				provisioning.Cluster{
					Name:          "one",
					ServerNames:   []string{"server1", "server2"},
					ConnectionURL: "http://one/",
				},
				provisioning.Cluster{
					Name:          "one",
					ServerNames:   []string{"server1", "server2"},
					ConnectionURL: "http://one/",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllClusters, tc.repoGetAllErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil, nil, nil)

			// Run test
			clusters, err := clusterSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusters, tc.count)
		})
	}
}

func TestClusterService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                      string
		filter                    provisioning.ClusterFilter
		repoGetAllWithFilter      provisioning.Clusters
		repoGetAllWithFilterErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:   "success - no filter expression",
			filter: provisioning.ClusterFilter{},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
				provisioning.Cluster{
					Name: "two",
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				{
					Value: provisioning.Servers{
						{
							Name: "server1",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name: "server2",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
				{
					Value: provisioning.Servers{
						{
							Name: "serverA",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name: "serverB",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`name == "one"`),
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
				provisioning.Cluster{
					Name: "two",
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				{
					Value: provisioning.Servers{
						{
							Name: "server1",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name: "server2",
							VersionData: api.ServerVersionData{
								NeedsUpdate:   ptr.To(false),
								NeedsReboot:   ptr.To(false),
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name: "error - invalid filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(``), // the empty expression is an invalid expression.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to compile filter expression:")
			},
			count: 0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to execute filter expression:")
			},
			count: 0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
			count: 0,
		},
		{
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
		{
			name:   "error - serverSvc.GetAllWithFilter",
			filter: provisioning.ClusterFilter{},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
				provisioning.Cluster{
					Name: "two",
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetAllWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, cluster, tc.count)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
		})
	}
}

func TestClusterService_GetAllNames(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllNames    []string
		repoGetAllNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllNames: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:               "error - repo",
			repoGetAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil, nil, nil)

			// Run test
			clusterNames, err := clusterSvc.GetAllNames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterNames, tc.count)
		})
	}
}

func TestClusterService_GetAllIDsWithFilter(t *testing.T) {
	tests := []struct {
		name                         string
		filter                       provisioning.ClusterFilter
		repoGetAllNamesWithFilter    []string
		repoGetAllNamesWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:   "success - no filter expression",
			filter: provisioning.ClusterFilter{},
			repoGetAllNamesWithFilter: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`name matches "one"`),
			},
			repoGetAllNamesWithFilter: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name: "error - invalid filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(``), // the empty expression is an invalid expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to compile filter expression:")
			},
			count: 0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to execute filter expression:")
			},
			count: 0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
			count: 0,
		},
		{
			name:                         "error - repo",
			repoGetAllNamesWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNamesWithFilter, tc.repoGetAllNamesWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil, nil, nil)

			// Run test
			clusterIDs, err := clusterSvc.GetAllNamesWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterIDs, tc.count)
		})
	}
}

func TestClusterService_GetByName(t *testing.T) {
	tests := []struct {
		name                         string
		nameArg                      string
		repoGetByNameCluster         *provisioning.Cluster
		repoGetByNameErr             error
		serverSvcGetAllWithFilter    provisioning.Servers
		serverSvcGetAllWithFilterErr error

		assertErr   require.ErrorAssertionFunc
		wantCluster *provisioning.Cluster
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{
					Name: "server1",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(false),
						NeedsReboot:   ptr.To(false),
						InMaintenance: ptr.To(api.NotInMaintenance),
					},
				},
				{
					Name: "server2",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(false),
						NeedsReboot:   ptr.To(false),
						InMaintenance: ptr.To(api.NotInMaintenance),
					},
				},
			},

			assertErr: require.NoError,
			wantCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				UpdateStatus: &api.ClusterUpdateStatus{
					NeedsUpdate:   []string{},
					NeedsReboot:   []string{},
					InMaintenance: []string{},
				},
			},
		},
		{
			name:    "success - with update status",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2", "server3", "server4"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{
					Name: "server1",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(true),
						NeedsReboot:   ptr.To(false),
						InMaintenance: ptr.To(api.NotInMaintenance),
					},
				},
				{
					Name: "server2",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(false),
						NeedsReboot:   ptr.To(true),
						InMaintenance: ptr.To(api.NotInMaintenance),
					},
				},
				{
					Name: "server3",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(false),
						NeedsReboot:   ptr.To(false),
						InMaintenance: ptr.To(api.InMaintenanceEvacuated),
					},
				},
				{
					Name: "server4",
					VersionData: api.ServerVersionData{
						NeedsUpdate:   ptr.To(false),
						NeedsReboot:   ptr.To(false),
						InMaintenance: ptr.To(api.InMaintenanceRestoring),
					},
				},
			},

			assertErr: require.NoError,
			wantCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2", "server3", "server4"},
				ConnectionURL: "http://one/",
				UpdateStatus: &api.ClusterUpdateStatus{
					NeedsUpdate:   []string{"server1"},
					NeedsReboot:   []string{"server2"},
					InMaintenance: []string{"server3", "server4"},
				},
			},
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:             "error - repo",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - serverSvc.GetAllWithFilter",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantCluster, cluster)
		})
	}
}

func TestClusterService_Update(t *testing.T) {
	tests := []struct {
		name                         string
		cluster                      provisioning.Cluster
		repoUpdateErr                error
		serverSvcGetAllWithFilter    []provisioning.Server
		serverSvcGetAllWithFilterErr error
		serverSvcUpdateErr           error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:          "one",
				ConnectionURL: "http://one/",
				Channel:       "stable",
			},
			serverSvcGetAllWithFilter: []provisioning.Server{
				{
					Name: "one",
				},
				{
					Name: "two",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:          "one",
				ConnectionURL: ":|\\", // invalid
				Channel:       "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Update",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
				Channel:       "stable",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetAllNamesWithFilter",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
				Channel:       "stable",
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetAllNamesWithFilter",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
				Channel:       "stable",
			},
			serverSvcGetAllWithFilter: []provisioning.Server{
				{
					Name: "one",
				},
				{
					Name: "two",
				},
			},
			serverSvcUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server, force, updateSystem bool) error {
					return tc.serverSvcUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			err := clusterSvc.Update(context.Background(), tc.cluster)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_Rename(t *testing.T) {
	updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

	tests := []struct {
		name          string
		oldName       string
		newName       string
		repoRenameErr error
		signalHandler func(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage)

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			oldName: "one",
			newName: "one new",

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name:    "error - old name empty",
			oldName: "", // invalid
			newName: "one new",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - new name empty",
			oldName: "one",
			newName: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:          "error - repo.Rename",
			oldName:       "one",
			newName:       "one new",
			repoRenameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil, nil, nil,
				provisioning.WithClusterServiceUpdateSignal(updateSignal),
			)

			var signalHandlerCalled bool
			updateSignal.AddListener(tc.signalHandler(t, &signalHandlerCalled))

			// Run test
			err := clusterSvc.Rename(context.Background(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
			require.True(t, signalHandlerCalled, "expected signal handler to called, but it was not OR no call was expected, but it got called")
		})
	}
}

func TestClusterService_DeleteByName(t *testing.T) {
	updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

	tests := []struct {
		name                                string
		nameArg                             string
		force                               bool
		repoGetByNameCluster                *provisioning.Cluster
		repoGetByNameErr                    error
		repoDeleteByNameErr                 error
		serverSvcGetAllNamesWithFilterNames []string
		serverSvcGetAllNamesWithFilterErr   error

		assertErr     require.ErrorAssertionFunc
		signalHandler func(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage)
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name:    "success - force",
			nameArg: "one",
			force:   true,
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:                "error - force - repo.DeleteByName",
			nameArg:             "one",
			force:               true,
			repoDeleteByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - cluster state ready",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, `Delete for cluster in state "ready":`)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:                 "error - cluster state not set",
			nameArg:              "one",
			repoGetByNameCluster: &provisioning.Cluster{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, "Delete for cluster with invalid state:")
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - cluster with linked servers",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllNamesWithFilterNames: []string{"one"},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, "Delete for cluster with 1 linked servers ([one])")
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - serverSvc.GetallNamesWithFilter",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllNamesWithFilterErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - repo.DeleteByID",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			repoDeleteByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllNamesWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
					return tc.serverSvcGetAllNamesWithFilterNames, tc.serverSvcGetAllNamesWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil,
				provisioning.WithClusterServiceUpdateSignal(updateSignal),
			)

			var signalHandlerCalled bool
			updateSignal.AddListener(tc.signalHandler(t, &signalHandlerCalled))

			// Run test
			err := clusterSvc.DeleteByName(context.Background(), tc.nameArg, tc.force)

			// Assert
			tc.assertErr(t, err)
			require.True(t, signalHandlerCalled, "expected signal handler to called, but it was not OR no call was expected, but it got called")
		})
	}
}

func TestDeleteAndFactoryResetByName(t *testing.T) {
	updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

	tests := []struct {
		name             string
		nameArg          string
		tokenArg         *uuid.UUID
		tokenSeedNameArg *string

		serverSvcGetAllWithFilter         provisioning.Servers
		serverSvcGetAllWithFilterErr      error
		clientPingErr                     error
		clientSystemFactoryResetErr       error
		tokenSvcGetTokenSeedByName        *provisioning.TokenSeed
		tokenSvcGetTokenSeedByNameErr     error
		tokenSvcCreate                    provisioning.Token
		tokenSvcCreateErr                 error
		tokenSvcGetTokenProviderConfig    *api.TokenProviderConfig
		tokenSvcGetTokenProviderConfigErr error
		repoDeleteByNameErr               error

		assertErr     require.ErrorAssertionFunc
		signalHandler func(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage)
	}{
		{
			name:    "success",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcCreate: provisioning.Token{
				UUID: uuidgen.FromPattern(t, "1"),
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{
				Version: "1",
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":   "https://1.2.3.4:8443",
						"server_token": uuidgen.FromPattern(t, "1").String(),
					},
				},
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name:     "success - with token",
			nameArg:  "one",
			tokenArg: ptr.To(uuidgen.FromPattern(t, "1")),

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{
				Version: "1",
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":   "https://1.2.3.4:8443",
						"server_token": uuidgen.FromPattern(t, "1").String(),
					},
				},
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},
		{
			name:             "success - with token and tokenSeedName",
			nameArg:          "one",
			tokenArg:         ptr.To(uuidgen.FromPattern(t, "1")),
			tokenSeedNameArg: ptr.To("token-seed-name"),

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcGetTokenSeedByName: &provisioning.TokenSeed{
				Token: uuidgen.FromPattern(t, "1"),
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{
				Version: "1",
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":   "https://1.2.3.4:8443",
						"server_token": uuidgen.FromPattern(t, "1").String(),
					},
				},
			},

			assertErr:     require.NoError,
			signalHandler: requireCallSignalHandler,
		},

		{
			name:    "error - name empty",
			nameArg: "", // invalid

			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - serverSvc.GetAllWithFilter",
			nameArg: "one",

			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - no servers",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
			},
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - client.Ping",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			clientPingErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:             "error - tokenSvc.GetTokenSeedByName",
			nameArg:          "one",
			tokenArg:         ptr.To(uuidgen.FromPattern(t, "1")),
			tokenSeedNameArg: ptr.To("token-seed-name"),

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcGetTokenSeedByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - tokenSvc.Create",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcCreateErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - tokenSvc.GetTokenProviderConfig",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcCreate: provisioning.Token{
				UUID: uuidgen.FromPattern(t, "1"),
			},
			tokenSvcGetTokenProviderConfigErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - client.SystemFactoryReset",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcCreate: provisioning.Token{
				UUID: uuidgen.FromPattern(t, "1"),
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{
				Version: "1",
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":   "https://1.2.3.4:8443",
						"server_token": uuidgen.FromPattern(t, "1").String(),
					},
				},
			},
			clientSystemFactoryResetErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
		{
			name:    "error - repo.DeleteByName",
			nameArg: "one",

			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
			},
			tokenSvcCreate: provisioning.Token{
				UUID: uuidgen.FromPattern(t, "1"),
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{
				Version: "1",
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":   "https://1.2.3.4:8443",
						"server_token": uuidgen.FromPattern(t, "1").String(),
					},
				},
			},
			repoDeleteByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			signalHandler: requireNoCallSignalHandler,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			tokenSvc := &serviceMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return tc.tokenSvcGetTokenSeedByName, tc.tokenSvcGetTokenSeedByNameErr
				},
				CreateFunc: func(ctx context.Context, token provisioning.Token) (provisioning.Token, error) {
					return tc.tokenSvcCreate, tc.tokenSvcCreateErr
				},
				GetTokenProviderConfigFunc: func(ctx context.Context, id uuid.UUID) (*api.TokenProviderConfig, error) {
					return tc.tokenSvcGetTokenProviderConfig, tc.tokenSvcGetTokenProviderConfigErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				SystemFactoryResetFunc: func(ctx context.Context, endpoint provisioning.Endpoint, allowTPMResetFailure bool, seeds provisioning.TokenImageSeedConfigs, providerConfig api.TokenProviderConfig) error {
					return tc.clientSystemFactoryResetErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, tokenSvc, nil, nil,
				provisioning.WithClusterServiceUpdateSignal(updateSignal),
			)

			var signalHandlerCalled bool
			updateSignal.AddListener(tc.signalHandler(t, &signalHandlerCalled))

			// Run test
			err := clusterSvc.DeleteAndFactoryResetByName(context.Background(), tc.nameArg, tc.tokenArg, tc.tokenSeedNameArg)

			// Assert
			tc.assertErr(t, err)
			require.True(t, signalHandlerCalled, "expected signal handler to called, but it was not OR no call was expected, but it got called")
		})
	}
}

func TestClusterService_ResyncInventory(t *testing.T) {
	tests := []struct {
		name               string
		ctx                context.Context
		repoGetAllClusters provisioning.Clusters
		repoGetAllErr      error
		inventorySyncerErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success - empty cluster list",
			ctx:                context.Background(),
			repoGetAllClusters: provisioning.Clusters{},

			assertErr: require.NoError,
		},
		{
			name: "success",
			ctx:  context.Background(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:          "error - GetAll",
			ctx:           context.Background(),
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Context done",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // context cancelled
				return ctx
			}(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - ResyncInventoryByName",
			ctx:  context.Background(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			inventorySyncerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllClusters, tc.repoGetAllErr
				},
			}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil, nil, nil)
			clusterSvc.SetInventorySyncers(map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventory(tc.ctx)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_ResyncInventoryByName(t *testing.T) {
	tests := []struct {
		name               string
		nameArg            string
		inventorySyncerErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:               "error - sync cluster",
			nameArg:            "one",
			inventorySyncerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, nil, nil, nil, nil, nil, nil)
			clusterSvc.SetInventorySyncers(map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventoryByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_AddServerSystemNetworkVLANTags(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		interfaceNameArg          string
		vlanTagsArg               []int
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]
		clientUpdateNetworkConfig []queue.Item[*incusosapi.SystemNetworkConfig] // Value is the expected value.

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:             "success",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50}, // vlan tag 10 already present
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50}, // vlan tag 10 already present
											},
										},
									},
								},
							},
						},
					},
				},
			},
			clientUpdateNetworkConfig: []queue.Item[*incusosapi.SystemNetworkConfig]{
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50, 20, 100}, // Expect the updated set of VLAN tags.
							},
						},
					},
				},
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50, 20, 100}, // Expect the updated set of VLAN tags.
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:             "error - GetByName error",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - cluster Status not ready",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusPending, // not ready
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.PollServers",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcPollServersErr: boom.Error,
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - cluster without members",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter - no servers found
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.GetAllWithFilter",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - server status not ready",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusOffline, // server offline
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - server without network config",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: nil, // no network config present
								},
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, `does not have any network config`)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - network interface missing on server",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{}, // no network interfaces
									},
								},
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, `does not have interface "uplink"`)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.AddSystemNetworkVLAN - revert serverSvc.ReomveSystemNetworkVLAN",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 20, 100},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			clientUpdateNetworkConfig: []queue.Item[*incusosapi.SystemNetworkConfig]{
				// Update first server.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50, 20, 100}, // Expect the updated set of VLANTags
							},
						},
					},
				},
				// Update second server fails.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50, 20, 100}, // Expect the updated set of VLANTags
							},
						},
					},
					Err: errors.New("error"),
				},
				// Revert of update on first server fails.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50}, // Expect only the original set of VLANTags
							},
						},
					},
					Err: boom.Error,
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated network configuration.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					wantConfig, err := queue.Pop(t, &tc.clientUpdateNetworkConfig)

					require.Equal(t, wantConfig, server.OSData.Network.Config)

					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.AddServerSystemNetworkVLANTags(context.Background(), tc.nameArg, tc.interfaceNameArg, tc.vlanTagsArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientUpdateNetworkConfig)
		})
	}
}

func TestClusterService_RemoveServerSystemNetworkVLANTags(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		interfaceNameArg          string
		vlanTagsArg               []int
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]
		clientUpdateNetworkConfig []queue.Item[*incusosapi.SystemNetworkConfig] // Value is the expected value.

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:             "success",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 30},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			clientUpdateNetworkConfig: []queue.Item[*incusosapi.SystemNetworkConfig]{
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{50}, // Expect the updated set of VLAN tags.
							},
						},
					},
				},
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{50}, // Expect the updated set of VLAN tags.
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			interfaceNameArg:        "uplink",
			vlanTagsArg:             []int{10},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - server without network config",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: nil, // no network config present
								},
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, `does not have any network config`)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - network interface missing on server",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{}, // no network interfaces
									},
								},
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, `does not have interface "uplink"`)
			},
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.ReomveSystemNetworkVLAN - revert serverSvc.AddSystemNetworkVLAN",
			nameArg:          "one",
			interfaceNameArg: "uplink",
			vlanTagsArg:      []int{10, 30},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										Interfaces: []incusosapi.SystemNetworkInterface{
											{
												Name:     "uplink",
												VLANTags: []int{10, 50},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			clientUpdateNetworkConfig: []queue.Item[*incusosapi.SystemNetworkConfig]{
				// Update on first server.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{50}, // Expect the updated set of VLAN tags.
							},
						},
					},
				},
				// Update on second server fails.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{50}, // Expect the updated set of VLAN tags.
							},
						},
					},
					Err: errors.New("error"),
				},
				// Revert on first server fails.
				{
					Value: &incusosapi.SystemNetworkConfig{
						Interfaces: []incusosapi.SystemNetworkInterface{
							{
								Name:     "uplink",
								VLANTags: []int{10, 50}, // Expect the original set of VLAN tags.
							},
						},
					},
					Err: boom.Error,
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated network configuration.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					_, err := queue.Pop(t, &tc.clientUpdateNetworkConfig)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.RemoveServerSystemNetworkVLANTags(context.Background(), tc.nameArg, tc.interfaceNameArg, tc.vlanTagsArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientUpdateNetworkConfig)
		})
	}
}

func TestClusterService_UpdateSystemLogging(t *testing.T) {
	tests := []struct {
		name                         string
		nameArg                      string
		loggingConfigArg             provisioning.ServerSystemLogging
		repoGetByName                *provisioning.Cluster
		repoGetByNameErr             error
		serverSvcPollServersErr      error
		serverSvcGetAllWithFilter    []queue.Item[provisioning.Servers]
		serverSvcGetSystemLogging    []queue.Item[provisioning.ServerSystemLogging]
		serverSvcUpdateSystemLogging []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:             "success",
			nameArg:          "one",
			loggingConfigArg: incusosapi.SystemLogging{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemLogging: []queue.Item[provisioning.ServerSystemLogging]{
				{},
				{},
			},
			serverSvcUpdateSystemLogging: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			loggingConfigArg:        incusosapi.SystemLogging{},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.GetSystemLogging",
			nameArg:          "one",
			loggingConfigArg: incusosapi.SystemLogging{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemLogging: []queue.Item[provisioning.ServerSystemLogging]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:             "error - serverSvc.UpdateSystemLogging - revert",
			nameArg:          "one",
			loggingConfigArg: incusosapi.SystemLogging{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemLogging: []queue.Item[provisioning.ServerSystemLogging]{
				{},
				{},
			},
			serverSvcUpdateSystemLogging: []queue.Item[struct{}]{
				// Update server one
				{},
				// Update server two
				{
					Err: errors.New("error"),
				},
				// Revert server one
				{
					Err: boom.Error,
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated logging config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
				GetSystemLoggingFunc: func(ctx context.Context, name string) (provisioning.ServerSystemLogging, error) {
					return queue.Pop(t, &tc.serverSvcGetSystemLogging)
				},
				UpdateSystemLoggingFunc: func(ctx context.Context, name string, config provisioning.ServerSystemLogging) error {
					_, err := queue.Pop(t, &tc.serverSvcUpdateSystemLogging)
					return err
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.UpdateSystemLogging(context.Background(), tc.nameArg, tc.loggingConfigArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.serverSvcGetSystemLogging)
			require.Empty(t, tc.serverSvcUpdateSystemLogging)
		})
	}
}

func TestClusterService_UpdateSystemKernel(t *testing.T) {
	tests := []struct {
		name                        string
		nameArg                     string
		kernelConfigArg             provisioning.ServerSystemKernel
		repoGetByName               *provisioning.Cluster
		repoGetByNameErr            error
		serverSvcPollServersErr     error
		serverSvcGetAllWithFilter   []queue.Item[provisioning.Servers]
		serverSvcGetSystemKernel    []queue.Item[provisioning.ServerSystemKernel]
		serverSvcUpdateSystemKernel []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:            "success",
			nameArg:         "one",
			kernelConfigArg: incusosapi.SystemKernel{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemKernel: []queue.Item[provisioning.ServerSystemKernel]{
				{},
				{},
			},
			serverSvcUpdateSystemKernel: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			kernelConfigArg:         incusosapi.SystemKernel{},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:            "error - serverSvc.GetSystemKernel",
			nameArg:         "one",
			kernelConfigArg: incusosapi.SystemKernel{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemKernel: []queue.Item[provisioning.ServerSystemKernel]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:            "error - serverSvc.UpdateSystemKernel - revert",
			nameArg:         "one",
			kernelConfigArg: incusosapi.SystemKernel{},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcGetSystemKernel: []queue.Item[provisioning.ServerSystemKernel]{
				{},
				{},
			},
			serverSvcUpdateSystemKernel: []queue.Item[struct{}]{
				// Update server one
				{},
				// Update server two
				{
					Err: errors.New("error"),
				},
				// Revert server one
				{
					Err: boom.Error,
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated kernel config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
				GetSystemKernelFunc: func(ctx context.Context, name string) (provisioning.ServerSystemKernel, error) {
					return queue.Pop(t, &tc.serverSvcGetSystemKernel)
				},
				UpdateSystemKernelFunc: func(ctx context.Context, name string, config provisioning.ServerSystemKernel) error {
					_, err := queue.Pop(t, &tc.serverSvcUpdateSystemKernel)
					return err
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.UpdateSystemKernel(context.Background(), tc.nameArg, tc.kernelConfigArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.serverSvcGetSystemKernel)
			require.Empty(t, tc.serverSvcUpdateSystemKernel)
		})
	}
}

func TestClusterService_AddApplication(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		applicationNameArg        string
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]
		serverSvcAddApplication   []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:               "success",
			nameArg:            "one",
			applicationNameArg: "debug",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcAddApplication: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			applicationNameArg:      "debug",
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:               "error - serverSvc.AddApplication",
			nameArg:            "one",
			applicationNameArg: "debug",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
							OSData: api.OSData{
								Network: incusosapi.SystemNetwork{
									Config: &incusosapi.SystemNetworkConfig{
										VLANs: []incusosapi.SystemNetworkVLAN{
											{
												Name: "first",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			serverSvcAddApplication: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
				AddApplicationFunc: func(ctx context.Context, name, applicationName string) error {
					_, err := queue.Pop(t, &tc.serverSvcAddApplication)
					return err
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, serverSvc, nil, nil, nil)

			// Run test
			err := clusterSvc.AddApplication(context.Background(), tc.nameArg, tc.applicationNameArg)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.serverSvcAddApplication)
		})
	}
}

func TestClusterService_AddStorageTargetISCSI(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		targetArg                 incusosapi.ServiceISCSITarget
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		clientGetOSServiceISCSI   []queue.Item[incusosapi.ServiceISCSI]
		clientUpdateOSService     []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:    "success",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceISCSITarget{},
						},
					},
				},
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:    "error - GetByName error",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - client.GetOSServiceISCSI",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - iscsi service target already present",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{
								{
									Target:  "target",
									Address: "address",
									Port:    1234,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service iscsi target "target" (address:1234) already defined on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:    "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceISCSITarget{},
						},
					},
				},
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: false,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated iscsi service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceISCSIFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceISCSI, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceISCSI)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceISCSI).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.AddStorageTargetISCSI(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceISCSI)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_RemoveStorageTargetISCSI(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		targetArg                 incusosapi.ServiceISCSITarget
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		clientGetOSServiceISCSI   []queue.Item[incusosapi.ServiceISCSI]
		clientUpdateOSService     []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:    "success",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{
								{
									Target:  "target",
									Address: "address",
									Port:    1234,
								},
								{
									Target:  "keep",
									Address: "keep",
									Port:    1234,
								},
							},
						},
					},
				},
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{
								{
									Target:  "target",
									Address: "address",
									Port:    1234,
								},
								{
									Target:  "keep",
									Address: "keep",
									Port:    1234,
								},
							},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:    "error - GetByName error",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - client.GetOSServiceISCSI",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - iscsi service target missing",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{}, // target missing
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service iscsi target "target" (address:1234) does not exist on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:    "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg: "one",
			targetArg: incusosapi.ServiceISCSITarget{
				Target:  "target",
				Address: "address",
				Port:    1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceISCSI: []queue.Item[incusosapi.ServiceISCSI]{
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{
								{
									Target:  "target",
									Address: "address",
									Port:    1234,
								},
							},
						},
					},
				},
				{
					Value: incusosapi.ServiceISCSI{
						Config: incusosapi.ServiceISCSIConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceISCSITarget{
								{
									Target:  "target",
									Address: "address",
									Port:    1234,
								},
							},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: true,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated iscsi service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceISCSIFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceISCSI, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceISCSI)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceISCSI).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.RemoveStorageTargetISCSI(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceISCSI)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_AddStorageTargetMultipath(t *testing.T) {
	tests := []struct {
		name                        string
		nameArg                     string
		targetArg                   string
		repoGetByName               *provisioning.Cluster
		repoGetByNameErr            error
		clientGetOSServiceMultipath []queue.Item[incusosapi.ServiceMultipath]
		clientUpdateOSService       []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr     error
		serverSvcGetAllWithFilter   []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:      "success",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: false, // false on purpose
							WWNs:    []string{},
						},
					},
				},
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			targetArg:               "target",
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:      "error - client.GetOSServiceMultipath",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:      "error - multipath service target already present",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{"target"},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service multipath target "target" already defined on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:      "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: false, // false on purpose
							WWNs:    []string{},
						},
					},
				},
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: false,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated multipath service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceMultipathFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceMultipath, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceMultipath)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceMultipath).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.AddStorageTargetMultipath(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceMultipath)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_RemoveStorageTargetMultipath(t *testing.T) {
	tests := []struct {
		name                        string
		nameArg                     string
		targetArg                   string
		repoGetByName               *provisioning.Cluster
		repoGetByNameErr            error
		clientGetOSServiceMultipath []queue.Item[incusosapi.ServiceMultipath]
		clientUpdateOSService       []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr     error
		serverSvcGetAllWithFilter   []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:      "success",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: false, // false on purpose
							WWNs:    []string{"target", "keep"},
						},
					},
				},
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{"target", "keep"},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                    "error - GetByName error",
			nameArg:                 "one",
			targetArg:               "target",
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:      "error - client.GetOSServiceMultipath",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:      "error - multipath service target missing",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{}, // target missing
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service multipath target "target" does not exist on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:      "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg:   "one",
			targetArg: "target",
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceMultipath: []queue.Item[incusosapi.ServiceMultipath]{
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: false, // false on purpose
							WWNs:    []string{"target"},
						},
					},
				},
				{
					Value: incusosapi.ServiceMultipath{
						Config: incusosapi.ServiceMultipathConfig{
							Enabled: true,
							WWNs:    []string{"target"},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: false,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated multipath service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceMultipathFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceMultipath, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceMultipath)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceMultipath).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.RemoveStorageTargetMultipath(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceMultipath)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_AddStorageTargetNVME(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		targetArg                 incusosapi.ServiceNVMETarget
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		clientGetOSServiceNVM     []queue.Item[incusosapi.ServiceNVME]
		clientUpdateOSService     []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:    "success",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVM: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceNVMETarget{},
						},
					},
				},
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:    "error - GetByName error",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - client.GetOSServiceNVME",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVM: []queue.Item[incusosapi.ServiceNVME]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - nvme service target already present",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVM: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{
								{
									Transport: "target",
									Address:   "address",
									Port:      1234,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service nvme transport "target" (address:1234) already defined on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:    "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVM: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceNVMETarget{},
						},
					},
				},
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: false,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated nvme service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceNVMEFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceNVME, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceNVM)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceNVME).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.AddStorageTargetNVME(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceNVM)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_RemoveStorageTargetNVME(t *testing.T) {
	tests := []struct {
		name                      string
		nameArg                   string
		targetArg                 incusosapi.ServiceNVMETarget
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		clientGetOSServiceNVME    []queue.Item[incusosapi.ServiceNVME]
		clientUpdateOSService     []queue.Item[bool] // bool is the expected value for the enabled flag of the service.
		serverSvcPollServersErr   error
		serverSvcGetAllWithFilter []queue.Item[provisioning.Servers]

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:    "success",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVME: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceNVMETarget{
								{
									Transport: "target",
									Address:   "address",
									Port:      1234,
								},
								{
									Transport: "keep",
									Address:   "keep",
									Port:      1234,
								},
							},
						},
					},
				},
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{
								{
									Transport: "target",
									Address:   "address",
									Port:      1234,
								},
								{
									Transport: "keep",
									Address:   "keep",
									Port:      1234,
								},
							},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				{
					Value: true,
				},
				{
					Value: true,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:    "error - GetByName error",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByNameErr:        boom.Error,
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - client.GetOSServiceNVME",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVME: []queue.Item[incusosapi.ServiceNVME]{
				{
					Err: boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:    "error - nvme service target missing",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVME: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{}, // target missing
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Service nvme transport "target" (address:1234) does not exist on server`)
			},
			assertLog: log.Empty,
		},
		{
			name:    "error - client.UpdateOSService - revert client.UpdateOSService",
			nameArg: "one",
			targetArg: incusosapi.ServiceNVMETarget{
				Transport: "target",
				Address:   "address",
				Port:      1234,
			},
			repoGetByName: &provisioning.Cluster{
				Name:   "one",
				Status: api.ClusterStatusReady,
			},
			clientGetOSServiceNVME: []queue.Item[incusosapi.ServiceNVME]{
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: false, // false on purpose
							Targets: []incusosapi.ServiceNVMETarget{
								{
									Transport: "target",
									Address:   "address",
									Port:      1234,
								},
							},
						},
					},
				},
				{
					Value: incusosapi.ServiceNVME{
						Config: incusosapi.ServiceNVMEConfig{
							Enabled: true,
							Targets: []incusosapi.ServiceNVMETarget{
								{
									Transport: "target",
									Address:   "address",
									Port:      1234,
								},
							},
						},
					},
				},
			},
			clientUpdateOSService: []queue.Item[bool]{
				// First update successful.
				{
					Value: true,
				},
				// Second update error.
				{
					Value: true,
					Err:   errors.New("error"),
				},
				// Revert of first update error.
				{
					Value: false,
					Err:   boom.Error,
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// GetByName
				{},
				// serverSvc.GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "one",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
						{
							Name:         "two",
							Cluster:      ptr.To("one"),
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								InMaintenance: ptr.To(api.NotInMaintenance),
							},
						},
					},
				},
			},

			assertErr: require.Error,
			assertLog: log.Match("Failed to revert previously updated nvme service config.*" + boom.Error.Error()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				GetOSServiceNVMEFunc: func(ctx context.Context, server provisioning.Server) (incusosapi.ServiceNVME, error) {
					config, err := queue.Pop(t, &tc.clientGetOSServiceNVME)
					return config, err
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					wantEnabled, err := queue.Pop(t, &tc.clientUpdateOSService)
					require.Equal(t, wantEnabled, config.(incusosapi.ServiceNVME).Config.Enabled)
					return err
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err = clusterSvc.RemoveStorageTargetNVME(context.Background(), tc.nameArg, tc.targetArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.clientGetOSServiceNVME)
			require.Empty(t, tc.clientUpdateOSService)
		})
	}
}

func TestClusterService_StartLifecycleEventsMonitor(t *testing.T) {
	doneChannel := func() chan struct{} {
		t.Helper()
		return make(chan struct{})
	}

	doneNonBlocking := func() chan struct{} {
		t.Helper()
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                           string
		initDone                       func() chan struct{}
		repoGetAllClusters             provisioning.Clusters
		repoGetAllErr                  error
		serverSvcGetAllWithFilterErr   error
		clientSubscribeLifecycleEvent  []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]
		inventorySyncerResyncByNameErr error

		assertErr           require.ErrorAssertionFunc
		wantProcessedEvents int
		assertLog           func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:     "success - one cluster and one event",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           log.Noop,
		},
		{
			name:          "error - GetAll",
			initDone:      doneNonBlocking,
			repoGetAllErr: boom.Error,

			assertErr: require.Error,
			assertLog: log.Noop,
		},
		{
			name:     "error - GetEndpoint",
			initDone: doneNonBlocking,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to start lifecycle monitor"),
		},
		{
			name:     "error - client.SubscribeLifecycleEvents - ctx.Done",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						return nil, nil, boom.Error
					},
				},
				{
					Value: func(cancel func()) (chan domain.LifecycleEvent, chan error, error) {
						cancel()

						return nil, nil, boom.Error
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to re-establish event stream"),
		},
		{
			name:     "error - client.SubscribeLifecycleEvents - retry",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						return nil, nil, boom.Error
					},
				},
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           log.Contains("Failed to re-establish event stream"),
		},
		{
			name:     "error - unavailable inventory syncer",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 2)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceType("unavailable"), // unavailable inventory syncer
						}

						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},
			inventorySyncerResyncByNameErr: boom.Error,

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           log.Contains("No inventory syncer available for the resource type"),
		},
		{
			name:     "error - inventorySyncer.ResyncByName",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},
			inventorySyncerResyncByNameErr: boom.Error,

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           log.Contains("Failed to resync"),
		},
		{
			name:     "error - Lifecycle subscription ended",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						errChan := make(chan error, 1)
						errChan <- boom.Error

						return nil, errChan, nil
					},
				},
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           log.Contains("Lifecycle events subscription ended"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cancableCtx, cancel := context.WithCancel(t.Context())
			defer cancel()

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			done := tc.initDone()

			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllClusters, tc.repoGetAllErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{}, tc.serverSvcGetAllWithFilterErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				SubscribeLifecycleEventsFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
					call, _ := queue.PopRetainLast(t, &tc.clientSubscribeLifecycleEvent)
					return call(cancel)
				},
			}

			processedEvents := 0
			processedEventsMu := sync.Mutex{}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				ResyncByNameFunc: func(ctx context.Context, clusterName string, sourceDetails domain.LifecycleEvent) error {
					processedEventsMu.Lock()
					defer processedEventsMu.Unlock()

					processedEvents++

					if processedEvents == tc.wantProcessedEvents {
						defer close(done)
					}

					return tc.inventorySyncerResyncByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, map[domain.ResourceType]provisioning.InventorySyncer{domain.ResourceTypeImage: inventorySyncer}, nil)

			// Run test
			err = clusterSvc.StartLifecycleEventsMonitor(cancableCtx)

			select {
			case <-done:
				cancel()

			case <-cancableCtx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")

			case <-time.After(1000 * time.Millisecond):
				cancel()
				t.Error("Test timeout reached before test ended")
			}

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantProcessedEvents, processedEvents)
			tc.assertLog(t, logBuf)
		})
	}
}

func TestClusterService_StartLifecycleEventsMonitor_AddListener(t *testing.T) {
	tests := []struct {
		name                         string
		serverSvcGetAllWithFilterErr error
		updateMessage                provisioning.ClusterUpdateMessage

		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name: "success register cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationCreate,
				Name:      "new",
			},

			assertLog: log.Noop,
		},
		{
			name: "error - startLifecycleEventHandler",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationCreate,
				Name:      "new",
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertLog: log.Contains("Failed to start lifecycle monitor"),
		},
		{
			name: "success delete cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationDelete,
				Name:      "existing",
			},

			assertLog: log.Noop,
		},
		{
			name: "success delete unknown cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationDelete,
				Name:      "unknown",
			},

			assertLog: log.Noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cancableCtx, cancel := context.WithCancel(t.Context())
			defer cancel()

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return provisioning.Clusters{
						{
							Name: "existing",
						},
					}, nil
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{}, tc.serverSvcGetAllWithFilterErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				SubscribeLifecycleEventsFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
					return nil, nil, nil
				},
			}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				ResyncByNameFunc: func(ctx context.Context, clusterName string, sourceDetails domain.LifecycleEvent) error {
					return nil
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer}, nil)

			// Run test
			err = clusterSvc.StartLifecycleEventsMonitor(cancableCtx)

			clusterSvc.GetClusterUpdateSignal().Emit(cancableCtx, tc.updateMessage)

			cancel()

			select {
			case <-cancableCtx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")

			case <-time.After(1000 * time.Millisecond):
				cancel()
				t.Error("Test timeout reached before test ended")
			}

			// Assert
			require.NoError(t, err)
			tc.assertLog(t, logBuf)
		})
	}
}

func TestClusterService_UpdateCertificate(t *testing.T) {
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name                              string
		certificatePEM                    string
		keyPEM                            string
		serverSvcGetAllWithFilter         provisioning.Servers
		serverSvcGetAllWithFilterErr      error
		repoGetByName                     *provisioning.Cluster
		repoGetByNameErr                  error
		clientUpdateClusterCertificateErr error
		repoUpdateErr                     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:           "success",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			serverSvcGetAllWithFilter: provisioning.Servers{
				{
					ConnectionURL: "http://one/",
					Certificate:   "cert",
				},
			},
			repoGetByName: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - invalid certificate pair",
			certificatePEM: "invalid", // invalid
			keyPEM:         "invalid", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to validate key pair:")
			},
		},
		{
			name:                         "error - serverSvc.GetAllWithFilter",
			certificatePEM:               string(certPEM),
			keyPEM:                       string(keyPEM),
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                              "error - client.UpdateClusterCertificate",
			certificatePEM:                    string(certPEM),
			keyPEM:                            string(keyPEM),
			clientUpdateClusterCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - repo.GetByName",
			certificatePEM:   string(certPEM),
			keyPEM:           string(keyPEM),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - repo.Update",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				UpdateClusterCertificateFunc: func(ctx context.Context, endpoint provisioning.Endpoint, certificatePEM, keyPEM string) error {
					return tc.clientUpdateClusterCertificateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, client, serverSvc, nil, nil, nil)

			// Run test
			err := clusterSvc.UpdateCertificate(context.Background(), "cluster", tc.certificatePEM, tc.keyPEM)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_GetClusterArtifactAll(t *testing.T) {
	tests := []struct {
		name                                  string
		argClusterName                        string
		artifactsRepoGetClusterArtifactAll    provisioning.ClusterArtifacts
		artifactsRepoGetClusterArtifactAllErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:           "success",
			argClusterName: "cluster",
			artifactsRepoGetClusterArtifactAll: provisioning.ClusterArtifacts{
				{
					ID:      1,
					Cluster: "cluster",
					Name:    "one",
				},
				{
					ID:      2,
					Cluster: "cluster",
					Name:    "two",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:           "error - clusterName empty",
			argClusterName: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			count: 0,
		},
		{
			name:                                  "error - artifactRepo.GetClusterArtifactAll",
			argClusterName:                        "cluster",
			artifactsRepoGetClusterArtifactAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			artifactsRepo := &mock.ClusterArtifactRepoMock{
				GetClusterArtifactAllFunc: func(ctx context.Context, clusterName string) (provisioning.ClusterArtifacts, error) {
					return tc.artifactsRepoGetClusterArtifactAll, tc.artifactsRepoGetClusterArtifactAllErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, artifactsRepo, nil, nil, nil, nil, nil)

			// Run test
			artifacts, err := clusterSvc.GetClusterArtifactAll(context.Background(), tc.argClusterName)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, artifacts, tc.count)
		})
	}
}

func TestClusterService_GetClusterArtifactAllNames(t *testing.T) {
	tests := []struct {
		name                                       string
		argClusterName                             string
		artifactsRepoGetClusterArtifactAllNames    []string
		artifactsRepoGetClusterArtifactAllNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:           "success",
			argClusterName: "cluster",
			artifactsRepoGetClusterArtifactAllNames: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:           "error - clusterName empty",
			argClusterName: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			count: 0,
		},
		{
			name:           "error - artifactRepo.GetClusterArtifactAllNames",
			argClusterName: "cluster",
			artifactsRepoGetClusterArtifactAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			artifactsRepo := &mock.ClusterArtifactRepoMock{
				GetClusterArtifactAllNamesFunc: func(ctx context.Context, clusterName string) ([]string, error) {
					return tc.artifactsRepoGetClusterArtifactAllNames, tc.artifactsRepoGetClusterArtifactAllNamesErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, artifactsRepo, nil, nil, nil, nil, nil)

			// Run test
			names, err := clusterSvc.GetClusterArtifactAllNames(context.Background(), tc.argClusterName)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, names, tc.count)
		})
	}
}

func TestClusterService_GetClusterArtifactByName(t *testing.T) {
	tests := []struct {
		name                                     string
		argClusterName                           string
		argArtifactName                          string
		artifactsRepoGetClusterArtifactByName    *provisioning.ClusterArtifact
		artifactsRepoGetClusterArtifactByNameErr error

		assertErr require.ErrorAssertionFunc
		want      *provisioning.ClusterArtifact
	}{
		{
			name:            "success",
			argClusterName:  "cluster",
			argArtifactName: "one",
			artifactsRepoGetClusterArtifactByName: &provisioning.ClusterArtifact{
				ID:      1,
				Cluster: "cluster",
				Name:    "one",
			},

			assertErr: require.NoError,
			want: &provisioning.ClusterArtifact{
				ID:      1,
				Cluster: "cluster",
				Name:    "one",
			},
		},
		{
			name:            "error - clusterName empty",
			argClusterName:  "", // empty
			argArtifactName: "one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:            "error - artifactName empty",
			argClusterName:  "cluster",
			argArtifactName: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:                                     "error - artifactsRepo.GetClusterArtifactByName",
			argClusterName:                           "cluster",
			argArtifactName:                          "one",
			artifactsRepoGetClusterArtifactByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			artifactsRepo := &mock.ClusterArtifactRepoMock{
				GetClusterArtifactByNameFunc: func(ctx context.Context, clusterName, artifactName string) (*provisioning.ClusterArtifact, error) {
					return tc.artifactsRepoGetClusterArtifactByName, tc.artifactsRepoGetClusterArtifactByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, artifactsRepo, nil, nil, nil, nil, nil)

			// Run test
			got, err := clusterSvc.GetClusterArtifactByName(context.Background(), tc.argClusterName, tc.argArtifactName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestClusterService_GetClusterArtifactFileByName(t *testing.T) {
	tests := []struct {
		name                                     string
		argClusterName                           string
		argArtifactName                          string
		argFilename                              string
		artifactsRepoGetClusterArtifactByName    *provisioning.ClusterArtifact
		artifactsRepoGetClusterArtifactByNameErr error

		assertErr require.ErrorAssertionFunc
		want      *provisioning.ClusterArtifactFile
	}{
		{
			name:            "success",
			argClusterName:  "cluster",
			argArtifactName: "one",
			argFilename:     "somefile.txt",
			artifactsRepoGetClusterArtifactByName: &provisioning.ClusterArtifact{
				ID:      1,
				Cluster: "cluster",
				Name:    "one",
				Files: provisioning.ClusterArtifactFiles{
					{
						Name:     "somefile.txt",
						MimeType: "text/plain",
						Size:     10,
					},
				},
			},

			assertErr: require.NoError,
			want: &provisioning.ClusterArtifactFile{
				Name:     "somefile.txt",
				MimeType: "text/plain",
				Size:     10,
			},
		},
		{
			name:            "error - filename empty",
			argClusterName:  "cluster",
			argArtifactName: "one",
			argFilename:     "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:                                     "error - artifactRepo.GetClusterArtifactByName",
			argClusterName:                           "cluster",
			argArtifactName:                          "one",
			argFilename:                              "somefile.txt",
			artifactsRepoGetClusterArtifactByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:            "error - file not found",
			argClusterName:  "cluster",
			argArtifactName: "one",
			argFilename:     "somefile.txt",
			artifactsRepoGetClusterArtifactByName: &provisioning.ClusterArtifact{
				ID:      1,
				Cluster: "cluster",
				Name:    "one",
				Files: provisioning.ClusterArtifactFiles{
					{
						Name:     "otherfile.txt", // filename does not match
						MimeType: "text/plain",
						Size:     10,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			artifactsRepo := &mock.ClusterArtifactRepoMock{
				GetClusterArtifactByNameFunc: func(ctx context.Context, clusterName, artifactName string) (*provisioning.ClusterArtifact, error) {
					return tc.artifactsRepoGetClusterArtifactByName, tc.artifactsRepoGetClusterArtifactByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, artifactsRepo, nil, nil, nil, nil, nil)

			// Run test
			got, err := clusterSvc.GetClusterArtifactFileByName(context.Background(), tc.argClusterName, tc.argArtifactName, tc.argFilename)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestClusterService_GetClusterArtifactArchiveByName(t *testing.T) {
	tests := []struct {
		name                                             string
		argClusterName                                   string
		argArtifactName                                  string
		argFilename                                      string
		artifactsRepoGetClusterArtifactArchiveByNameRC   io.ReadCloser
		artifactsRepoGetClusterArtifactArchiveByNameSize int
		artifactsRepoGetClusterArtifactArchiveByNameErr  error

		assertErr require.ErrorAssertionFunc
		assert    func(t *testing.T, rc io.ReadCloser, size int)
	}{
		{
			name:            "success",
			argClusterName:  "cluster",
			argArtifactName: "one",
			argFilename:     "somefile.txt",
			artifactsRepoGetClusterArtifactArchiveByNameRC:   io.NopCloser(bytes.NewBufferString(`foobar`)),
			artifactsRepoGetClusterArtifactArchiveByNameSize: 6,

			assertErr: require.NoError,
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				body, err := io.ReadAll(rc)
				require.NoError(t, err)
				require.Equal(t, []byte(`foobar`), body)
				require.Equal(t, 6, size)
			},
		},
		{
			name: "error - artifactsRepo.GetClusterArtifactArchiveByNameFunc",
			artifactsRepoGetClusterArtifactArchiveByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				require.Nil(t, rc)
				require.Zero(t, size)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			artifactsRepo := &mock.ClusterArtifactRepoMock{
				GetClusterArtifactArchiveByNameFunc: func(ctx context.Context, clusterName, artifactName string, archiveType provisioning.ClusterArtifactArchiveType) (io.ReadCloser, int, error) {
					return tc.artifactsRepoGetClusterArtifactArchiveByNameRC, tc.artifactsRepoGetClusterArtifactArchiveByNameSize, tc.artifactsRepoGetClusterArtifactArchiveByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, artifactsRepo, nil, nil, nil, nil, nil)

			zipArchiveType, ok := provisioning.ClusterArtifactArchiveTypes[provisioning.ClusterArtifactArchiveTypeExtZip]
			require.True(t, ok)

			// Run test
			rc, size, err := clusterSvc.GetClusterArtifactArchiveByName(context.Background(), tc.argClusterName, tc.argArtifactName, zipArchiveType)

			// Assert
			tc.assertErr(t, err)
			tc.assert(t, rc, size)
		})
	}
}

func requireNoCallSignalHandler(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage) {
	t.Helper()

	*called = true

	return func(ctx context.Context, cum provisioning.ClusterUpdateMessage) {
		// No call was expected. If we get called anyway, reset called.
		*called = false
	}
}

func requireCallSignalHandler(t *testing.T, called *bool) func(ctx context.Context, cum provisioning.ClusterUpdateMessage) {
	t.Helper()

	return func(ctx context.Context, cum provisioning.ClusterUpdateMessage) {
		*called = true
	}
}
