package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestServerService_GetChangelogByName(t *testing.T) {
	updateV1UUID := uuidgen.FromPattern(t, "1")
	updateV2UUID := uuidgen.FromPattern(t, "2")

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByName             []queue.Item[*provisioning.Server]
		updateSvcGetAllWithFilter []queue.Item[provisioning.Updates]
		updateSvcGetChangelog     api.UpdateChangelog
		updateSvcGetChangelogErr  error

		assertErr     require.ErrorAssertionFunc
		wantChangelog api.UpdateChangelog
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
				// updateSvc.GetAllWithFilter
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
			},
			updateSvcGetChangelog: api.UpdateChangelog{
				CurrentVersion: "2",
				PriorVersion:   "1",
				Components: map[string]images.ChangelogEntries{
					"IncusOS": {
						Updated: []string{"file version 1 to version 2"},
					},
					"incus": {
						Updated: []string{"file version 1 to version 2"},
					},
				},
			},

			assertErr: require.NoError,
			wantChangelog: images.Changelog{
				CurrentVersion: "2",
				PriorVersion:   "1",
				Channel:        "stable",
				Components: map[string]images.ChangelogEntries{
					"IncusOS": {
						Updated: []string{"file version 1 to version 2"},
					},
					"incus": {
						Updated: []string{"file version 1 to version 2"},
					},
				},
			},
		},
		{
			name:    "success - no update available",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						// No update available.
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
		},

		{
			name:    "error - GetByName",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - updateSvc.GetAllWithFitler",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
				// updateSvc.GetAllWithFilter
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - updateSvc.GetChangelog",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
				// updateSvc.GetAllWithFilter
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_20260610.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_202606100326.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
			},
			updateSvcGetChangelogErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return queue.Pop(t, &tc.updateSvcGetAllWithFilter)
				},
				GetChangelogFunc: func(ctx context.Context, currentID, priorID uuid.UUID, architecture images.UpdateFileArchitecture) (api.UpdateChangelog, error) {
					require.Equal(t, updateV2UUID, currentID)
					require.Equal(t, updateV1UUID, priorID)
					return tc.updateSvcGetChangelog, tc.updateSvcGetChangelogErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			changelog, err := serverSvc.GetChangelogByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantChangelog, changelog)
			require.Empty(t, tc.repoGetByName)
		})
	}
}

func TestServerService_EvacuateSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argClusterUpdate                                bool
		argForce                                        bool
		repoGetByName                                   []queue.Item[*provisioning.Server]
		repoUpdateErrs                                  queue.Errs
		clientEvacuateErr                               error
		clusterSvcGetByName                             *provisioning.Cluster
		clusterSvcGetByNameErr                          error
		clusterSvcUpdateErr                             error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		doCallback                                      func(f func(ctx context.Context, err error))

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						StatusInternal: provisioning.ServerStatusInternal{
							Update: &provisioning.ServerUpdate{
								StartedAt:       time.Now(),
								Step:            provisioning.ServerUpdateStepEvacuate,
								StepTriggeredAt: time.Now(),
							},
						},
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},

			assertErr: errassert.RetryableErrorContains(`Step "evacuate" for server "one" is in flight`),
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - attempt limit reached",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						StatusInternal: provisioning.ServerStatusInternal{
							Update: &provisioning.ServerUpdate{
								StartedAt: time.Now(),
								Step:      provisioning.ServerUpdateStepEvacuate,
								Retries:   3,
								LastError: "boom!",
							},
						},
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},

			assertErr: errassert.TerminalErrorContains(`Failed to evacuate server "one" in 3 attempts`),
			assertLog: log.Noop,
		},
		{
			name: "error - callback error",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error - status update successful",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error - GetByName",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Err: boom.Error,
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to record the failure of a rolling update step server=one step=evacuate err="Failed to get server \"one\" by name:`)(t, logBuf)
			},
		},
		{
			name:             "error - cluster update - callback error - repo.Update",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clusterSvcGetByName: &provisioning.Cluster{},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to record the failure of a rolling update step server=one step=evacuate err=boom!`)(t, logBuf)
			},
		},
		{
			name: "error - repo.GetByName",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeOperationsCenter,
					},
				},
			},
			name: "error - not type incus",

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operations for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Evacuate",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Evacuate - reverter error",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Contains("Failed to restore previous server state after failed to trigger evacuation server=one err=boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// if tc.name != "error - cluster update - callback error" {
			// 	t.SkipNow()
			// }
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
					tc.doCallback(callback)
					return tc.clientEvacuateErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster, updateServers bool) error {
					return tc.clusterSvcUpdateErr
				},
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.EvacuateSystemByName(t.Context(), "one", tc.argClusterUpdate, tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoGetByName)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_PoweroffSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientPoweroffErr                               error
		clusterSvcIsInstanceLifecycleOperationPermitted bool

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operations for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Poweroff",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientPoweroffErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Poweroff and reverter error",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientPoweroffErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger poweroff server=one err=boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PoweroffFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientPoweroffErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.PoweroffSystemByName(t.Context(), "one", tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_RebootSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientRebootErr                                 error
		clusterSvcIsInstanceLifecycleOperationPermitted bool

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - operation in flight",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						StartedAt:       time.Now(),
						Step:            provisioning.ServerUpdateStepReboot,
						StepTriggeredAt: time.Now(),
					},
				},
			},

			assertErr: errassert.RetryableErrorContains(`Step "reboot" for server "operations-center" is in flight`),
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operations for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Reboot",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRebootErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Reboot and reverter error",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientRebootErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger reboot server=one err=boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RebootFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientRebootErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.RebootSystemByName(t.Context(), "one", tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_RestoreSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argClusterUpdate                                bool
		argForce                                        bool
		argRestoreModeSkip                              bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientRestoreErr                                error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		doCallback                                      func(f func(ctx context.Context, err error))

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						StartedAt:       time.Now(),
						Step:            provisioning.ServerUpdateStepRestore,
						StepTriggeredAt: time.Now(),
					},
				},
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},

			assertErr: errassert.RetryableErrorContains(`Step "restore" for server "one" is in flight`),
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - attempt limit reached",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						StartedAt: time.Now(),
						Step:      provisioning.ServerUpdateStepRestore,
						Retries:   3,
						LastError: "boom!",
					},
				},
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},

			assertErr: errassert.TerminalErrorContains(`Failed to restore server "one" in 3 attempts`),
			assertLog: log.Noop,
		},
		{
			name: "error - callback error",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to restore system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to restore system name=one err=boom!"),
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - not type incus",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operations for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Restore",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Restore and reverter error",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger restore server=one err=boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RestoreFunc: func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
					tc.doCallback(callback)
					return tc.clientRestoreErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.RestoreSystemByName(t.Context(), "one", tc.argClusterUpdate, tc.argForce, tc.argRestoreModeSkip)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_PostRestoreSystemDoneByName(t *testing.T) {
	tests := []struct {
		name               string
		argRestoreModeSkip bool
		repoGetByName      provisioning.Server
		repoGetByNameErr   error
		repoUpdateErr      error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - not type incus",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err := serverSvc.PostRestoreSystemDoneByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argUpdateRequest                                api.ServerUpdatePost
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientUpdateOSErr                               error
		clientUpdateApplicationErr                      error
		channelSvcGetByNameErr                          error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		registerUnreachableBMCClient                    bool

		updateSvcGetAllWithFilter provisioning.Updates

		assertErr               require.ErrorAssertionFunc
		assertLog               log.MatcherFunc
		wantUpdatedApplications []string
		wantStatusDetail        *api.ServerStatusDetail
		wantTriggeredUpdate     *provisioning.ServerTriggeredUpdate
		wantOSOnly              *bool
	}{
		{
			name: "success - no update triggered",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name: "success - trigger OS update - lifecycle operation permitted",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - trigger OS update - force",
			argForce: true,
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - server not ready",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusPending, // server not ready
			},

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operations for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - UpdateSystemUpdate - channelSvc.GetByName",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			channelSvcGetByNameErr:                          boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.UpdateOS",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientUpdateOSErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.UpdateOS and reverter error",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientUpdateOSErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to update the system server=one err=.*boom!"),
		},
		{
			name: "success - trigger OS update - unreachable BMC",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			registerUnreachableBMCClient:                    true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name: "success - trigger application update only",
			argUpdateRequest: api.ServerUpdatePost{
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "incus",
						TriggerUpdate: true,
					},
					{
						Name:          "openfga",
						TriggerUpdate: false,
					},
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{Name: "incus"},
						{Name: "openfga"},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               require.NoError,
			assertLog:               log.Noop,
			wantUpdatedApplications: []string{"incus"},
			wantStatusDetail:        new(api.ServerStatusDetailReadyUpdatingApplication),
		},
		{
			// The versions the triggered components are expected to reach are taken
			// from the update channel and recorded, so that polling can tell which of
			// them the update is waiting for.
			name: "success - trigger application update, expected versions are recorded",
			argUpdateRequest: api.ServerUpdatePost{
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "incus",
						TriggerUpdate: true,
					},
					{
						Name:          "openfga",
						TriggerUpdate: false,
					},
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:    "IncusOS",
						Version: "1",
					},
					Applications: []api.ApplicationVersionData{
						{Name: "incus", Version: "1"},
						{Name: "openfga", Version: "1"},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/openfga.raw.gz",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               require.NoError,
			assertLog:               log.Noop,
			wantUpdatedApplications: []string{"incus"},
			wantStatusDetail:        new(api.ServerStatusDetailReadyUpdatingApplication),
			wantTriggeredUpdate: &provisioning.ServerTriggeredUpdate{
				Applications: map[string]string{
					"incus": "2",
				},
			},
		},
		{
			name: "success - trigger OS update, covered applications are recorded",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:    "IncusOS",
						Version: "1",
					},
					Applications: []api.ApplicationVersionData{
						{Name: "incus", Version: "1"},
						{Name: "openfga", Version: "2"},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/openfga.raw.gz",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               require.NoError,
			assertLog:               log.Noop,
			wantUpdatedApplications: nil,
			wantStatusDetail:        new(api.ServerStatusDetailReadyUpdatingOS),
			wantTriggeredUpdate: &provisioning.ServerTriggeredUpdate{
				OS: "2",
				Applications: map[string]string{
					"incus": "2",
				},
			},
		},
		{
			name: "success - trigger OS only update, no application is recorded",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
				OSOnly: true,
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:    "IncusOS",
						Version: "1",
					},
					Applications: []api.ApplicationVersionData{
						{Name: "incus", Version: "1"},
						{Name: "openfga", Version: "2"},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/openfga.raw.gz",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               require.NoError,
			assertLog:               log.Noop,
			wantUpdatedApplications: nil,
			wantStatusDetail:        new(api.ServerStatusDetailReadyUpdatingOS),
			wantTriggeredUpdate: &provisioning.ServerTriggeredUpdate{
				OS: "2",
			},
			wantOSOnly: new(true),
		},
		{
			name: "error - OS only update without an OS update",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: false,
				},
				OSOnly: true,
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: errassert.ValidationErrorContains(`An update restricted to the OS requires an update of the OS`),
			assertLog: log.Noop,
		},
		{
			name: "error - OS update combined with application update",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "incus",
						TriggerUpdate: true,
					},
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{Name: "incus"},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               errassert.ValidationErrorContains(`An update of the OS covers the applications as well`),
			assertLog:               log.Noop,
			wantUpdatedApplications: nil,
		},
		{
			name: "error - application not installed on server",
			argUpdateRequest: api.ServerUpdatePost{
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "not-installed",
						TriggerUpdate: true,
					},
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{Name: "incus"},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr:               errassert.ValidationErrorContains(`Application "not-installed" is not installed on server "one"`),
			assertLog:               log.Noop,
			wantUpdatedApplications: nil,
		},
		{
			name: "error - client.UpdateApplication",
			argUpdateRequest: api.ServerUpdatePost{
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "incus",
						TriggerUpdate: true,
					},
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{Name: "incus"},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientUpdateApplicationErr:                      boom.Error,

			assertErr:               boom.ErrorIs,
			assertLog:               log.Noop,
			wantUpdatedApplications: []string{"incus"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			var gotStatusDetail *api.ServerStatusDetail
			var gotTriggeredUpdate *provisioning.ServerTriggeredUpdate

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					// Return a copy, since the server is also queried by the background
					// poll, which would otherwise share the same instance.
					server := tc.repoGetByName
					return &server, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					// The background poll marks the server as unresponsive, since the
					// connection test is mocked to fail. Such updates are not subject of
					// this test and must not consume from the queue.
					if server.StatusDetail == api.ServerStatusDetailOfflineUnresponsive {
						return nil
					}

					// Record the status detail the update has been started with. Later
					// updates are the reverter restoring the previous state.
					if gotStatusDetail == nil {
						gotStatusDetail = &server.StatusDetail

						if server.StatusInternal.Update != nil {
							gotTriggeredUpdate = server.StatusInternal.Update.Triggered
						}
					}

					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateOSFunc: func(ctx context.Context, server provisioning.Server, osOnly bool) error {
					return tc.clientUpdateOSErr
				},
				UpdateApplicationFunc: func(ctx context.Context, server provisioning.Server, application string) error {
					return tc.clientUpdateApplicationErr
				},
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return errors.New("") // short circuit pollServer, since we don't care about this part in this test.
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
					return nil
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, tc.channelSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilter, nil
				},
			}

			opts := []provisioningServer.Option{
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			}
			if tc.registerUnreachableBMCClient {
				bmcClient := &adapterMock.BMCServerClientPortMock{
					ConnectionTestFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
						return "", boom.Error
					},
				}

				opts = append(opts, provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient))
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, channelSvc, updateSvc, tls.Certificate{},
				opts...,
			)

			// Run test
			err = serverSvc.UpdateSystemByName(t.Context(), "one", tc.argUpdateRequest, tc.argForce)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoUpdateErrs)

			var updatedApplications []string
			for _, call := range client.UpdateApplicationCalls() {
				updatedApplications = append(updatedApplications, call.Application)
			}

			require.Equal(t, tc.wantUpdatedApplications, updatedApplications)

			if tc.wantOSOnly != nil {
				updateOSCalls := client.UpdateOSCalls()
				require.Len(t, updateOSCalls, 1)
				require.Equal(t, *tc.wantOSOnly, updateOSCalls[0].OsOnly)
			}

			if tc.wantStatusDetail != nil {
				require.Equal(t, *tc.wantStatusDetail, ptr.From(gotStatusDetail))
			}

			if tc.wantTriggeredUpdate != nil {
				require.NotNil(t, gotTriggeredUpdate)
				require.Equal(t, tc.wantTriggeredUpdate.OS, gotTriggeredUpdate.OS)
				require.Equal(t, tc.wantTriggeredUpdate.Applications, gotTriggeredUpdate.Applications)
				require.False(t, gotTriggeredUpdate.TriggeredAt.IsZero())
			}
		})
	}
}

func TestServerService_FactoryResetByName(t *testing.T) {
	tests := []struct {
		name                              string
		argName                           string
		argTokenID                        *uuid.UUID
		argTokenSeedName                  *string
		repoGetByName                     provisioning.Server
		repoGetByNameErr                  error
		clientPingErr                     error
		clientSystemFactoryResetErr       error
		tokenSvcGetTokenSeedByName        *provisioning.TokenSeed
		tokenSvcGetTokenSeedByNameErr     error
		tokenSvcCreate                    provisioning.Token
		tokenSvcCreateErr                 error
		tokenSvcGetTokenProviderConfig    *api.TokenProviderConfig
		tokenSvcGetTokenProviderConfigErr error
		repoDeleteByNameErr               error
		trustedClientCertificates         []string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success - without tokenID and without tokenSeedName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "debug",
						},
						{
							Name: "incus-lts-7.0",
						},
					},
				},
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},

			assertErr: require.NoError,
		},
		{
			name:             "success - with tokenID and tokenSeedName",
			argName:          "one",
			argTokenID:       new(uuidgen.FromPattern(t, "1")),
			argTokenSeedName: new("some_seed"),
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenSeedByName:     &provisioning.TokenSeed{},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},

			assertErr: require.NoError,
		},
		{
			name:    "success - with trusted client certificates",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus-lts-7.0",
						},
					},
				},
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			trustedClientCertificates:      []string{testcert.ClientCertificate},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			argName: "",

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - repo.GetByName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - operations center",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "operations-center",
				Type: api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - incus clustered",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name:    "server01",
				Cluster: new("cluster"),
				Type:    api.ServerTypeIncus,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - client.Ping",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - tokenSvc.GetTokenSeedByName",
			argName:          "one",
			argTokenID:       new(uuidgen.FromPattern(t, "1")),
			argTokenSeedName: new("some_seed"),
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - tokenSvc.Create",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			tokenSvcCreateErr:              boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - tokenSvc.GetTokenProviderConfig",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.SystemFactoryReset",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			clientSystemFactoryResetErr:    boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.DeleteByName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			repoDeleteByNameErr:            boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool { return false },
			}, nil)

			err := config.UpdateSecurity(t.Context(), system.SecurityPut{
				TrustedTLSClientCertificates: tc.trustedClientCertificates,
			})
			require.NoError(t, err)

			t.Cleanup(func() {
				config.InitTest(t, &envMock.EnvironmentMock{}, nil)
			})

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				SystemFactoryResetFunc: func(ctx context.Context, endpoint provisioning.Endpoint, allowTPMResetFailure bool, seeds provisioning.TokenImageSeedConfigs, providerConfig api.TokenProviderConfig) error {
					// The server is deployed again, so it trusts the same clients as
					// any other system deployed by Operations Center.
					require.Equal(t, tc.trustedClientCertificates, seeds.OperationsCenter.TrustedClientCertificates)
					require.Equal(t, tc.trustedClientCertificates, seeds.MigrationManager.TrustedClientCertificates)

					for i, certificate := range tc.trustedClientCertificates {
						require.Equal(t, certificate, seeds.Incus.Preseed.Certificates[i].Certificate)
					}

					return tc.clientSystemFactoryResetErr
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
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

			serverSvc := provisioningServer.New(repo, client, nil, tokenSvc, nil, nil, nil, tls.Certificate{})

			// Run test
			err = serverSvc.FactoryResetByName(t.Context(), tc.argName, tc.argTokenID, tc.argTokenSeedName, false)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
