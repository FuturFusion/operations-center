package provisioning_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	asyncActionsDelay   = 50 * time.Millisecond
	controlLoopInterval = 10 * time.Millisecond
)

func TestClusterService_ClusterUpdateCycleSingleNodeCluster(t *testing.T) {
	// Test data
	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	clusterA := provisioning.Cluster{
		Name:          "clusterA",
		ConnectionURL: "https://cluster-one/",
		Certificate:   ptr.To(string(certPEM)),
		Fingerprint:   fingerprint,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"serverA"},
		Channel:       "stable",
	}

	serverA := provisioning.Server{
		Name:          "one",
		Cluster:       ptr.To("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		Certificate:   string(certPEM),
		Fingerprint:   fingerprint,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   ptr.To(true),
			InMaintenance: ptr.To(api.NotInMaintenance),
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	serverVersionDataMu := sync.Mutex{}
	serverVersionData := api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "incusos",
			Version:     "1",
			VersionNext: "1",
			NeedsReboot: false,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:          "incus",
				Version:       "1",
				InMaintenance: api.NotInMaintenance,
			},
		},
	}
	serverRebooting := false

	// Setup
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	logBuf := &bytes.Buffer{}
	var logSink io.Writer = logBuf
	if testing.Verbose() {
		logSink = io.MultiWriter(os.Stdout, logBuf)
	}

	err = logger.InitLogger(logSink, "", false, true)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	clusterDB := sqlite.NewCluster(tx)
	serverDB := sqlite.NewServer(tx)

	_, err = clusterDB.Create(ctx, clusterA)
	require.NoError(t, err)
	_, err = serverDB.Create(ctx, serverA)
	require.NoError(t, err)

	channelSvc := &svcMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	updateSvc := &svcMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename:  "os",
							Component: images.UpdateFileComponentOS,
						},
						{
							Filename:  "incus",
							Component: images.UpdateFileComponentIncus,
						},
					},
				},
			}, nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
		UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
			return nil
		},
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			if serverRebooting {
				return domain.NewRetryableErr(errors.New("rebooting"))
			}

			return nil
		},
		GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
			return api.HardwareData{}, nil
		},
		GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
			return api.OSData{}, nil
		},
		GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			return serverVersionData, nil
		},
		GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
			return api.ServerTypeIncus, nil
		},
		UpdateOSFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "1",
						VersionNext: "2",
						NeedsReboot: true,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.NotInMaintenance,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "1",
					VersionNext: "1",
					NeedsReboot: false,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "1",
						InMaintenance: api.NotInMaintenance,
					},
				},
			}

			return nil
		},
		EvacuateFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "1",
						VersionNext: "2",
						NeedsReboot: true,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.InMaintenanceEvacuated,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "1",
					VersionNext: "2",
					NeedsReboot: true,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceEvacuating,
					},
				},
			}

			return nil
		},
		RebootFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "2",
						VersionNext: "2",
						NeedsReboot: false,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.InMaintenanceEvacuated,
						},
					},
				}

				serverRebooting = false
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "2",
					VersionNext: "2",
					NeedsReboot: true,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceEvacuated,
					},
				},
			}

			serverRebooting = true

			return nil
		},
		RestoreFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "2",
						VersionNext: "2",
						NeedsReboot: false,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.NotInMaintenance,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "2",
					VersionNext: "2",
					NeedsReboot: false,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceRestoring,
					},
				},
			}

			return nil
		},
	}

	serverSvc := provisioning.NewServerService(serverDB, serverClient, nil, nil, channelSvc, updateSvc, tls.Certificate{})

	clusterSvc := provisioning.NewClusterService(clusterDB, nil, nil, serverSvc, nil, nil, nil,
		provisioning.WithClusterServicePendingUpdateRecheckInterval(controlLoopInterval),
	)

	serverSvc.SetClusterService(clusterSvc)

	// Run test
	err = clusterSvc.LaunchClusterUpdate(ctx, "clusterA")
	require.NoError(t, err)

	success := false
	for range 100 {
		c, err := clusterSvc.GetByName(ctx, "clusterA")
		if !c.UpdateStatus.InProgressStatus.InProgress {
			success = true
			break
		}

		err = clusterSvc.ClusterUpdateControlLoop(ctx)
		require.NoError(t, err)

		time.Sleep(controlLoopInterval)
	}

	require.True(t, success)
	log.Contains(`[1/6] evacuation pending server "one"`)(t, logBuf)
	log.Contains(`[2/6] evacuating server "one"`)(t, logBuf)
	log.Contains(`[3/6] in maintenance, reboot pending server "one"`)(t, logBuf)
	log.Contains(`[4/6] in maintenance, rebooting server "one"`)(t, logBuf)
	log.Contains(`[5/6] in maintenance, restore pending server "one"`)(t, logBuf)
	log.Contains(`[6/6] restoring server "one"`)(t, logBuf)
}

func TestClusterService_ClusterUpdateCycleMultiNodeCluster(t *testing.T) {
	// Test data
	certPEMA, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	certPEMB, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprintA, err := incustls.CertFingerprintStr(string(certPEMA))
	require.NoError(t, err)

	fingerprintB, err := incustls.CertFingerprintStr(string(certPEMA))
	require.NoError(t, err)

	clusterA := provisioning.Cluster{
		Name:          "clusterA",
		ConnectionURL: "https://cluster-one/",
		Certificate:   ptr.To(string(certPEMA)),
		Fingerprint:   fingerprintA,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"serverA"},
		Channel:       "stable",
	}

	serverA := provisioning.Server{
		Name:          "one",
		Cluster:       ptr.To("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		Certificate:   string(certPEMA),
		Fingerprint:   fingerprintA,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   ptr.To(true),
			InMaintenance: ptr.To(api.NotInMaintenance),
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	serverB := provisioning.Server{
		Name:          "two",
		Cluster:       ptr.To("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://two/",
		Certificate:   string(certPEMB),
		Fingerprint:   fingerprintB,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   ptr.To(true),
			InMaintenance: ptr.To(api.NotInMaintenance),
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	serverVersionDataMu := sync.Mutex{}
	serverVersionData := map[string]api.ServerVersionData{
		"one": {
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: false,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
		},
		"two": {
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: false,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
		},
	}
	serverRebooting := map[string]bool{
		"one": false,
		"two": false,
	}

	// Setup
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	logBuf := &bytes.Buffer{}
	var logSink io.Writer = logBuf
	if testing.Verbose() {
		logSink = io.MultiWriter(os.Stdout, logBuf)
	}

	err = logger.InitLogger(logSink, "", false, true)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	clusterDB := sqlite.NewCluster(tx)
	serverDB := sqlite.NewServer(tx)

	_, err = clusterDB.Create(ctx, clusterA)
	require.NoError(t, err)
	_, err = serverDB.Create(ctx, serverA)
	require.NoError(t, err)
	_, err = serverDB.Create(ctx, serverB)
	require.NoError(t, err)

	channelSvc := &svcMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	updateSvc := &svcMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename:  "os",
							Component: images.UpdateFileComponentOS,
						},
						{
							Filename:  "incus",
							Component: images.UpdateFileComponentIncus,
						},
					},
				},
			}, nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
		UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
			return nil
		},
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			if serverRebooting[endpoint.GetName()] {
				return domain.NewRetryableErr(errors.New("rebooting"))
			}

			return nil
		},
		GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
			return api.HardwareData{}, nil
		},
		GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
			return api.OSData{}, nil
		},
		GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			return serverVersionData[server.Name], nil
		},
		GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
			return api.ServerTypeIncus, nil
		},
		UpdateOSFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData[server.Name] = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "1",
						VersionNext: "2",
						NeedsReboot: true,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.NotInMaintenance,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData[server.Name] = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "1",
					VersionNext: "1",
					NeedsReboot: false,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "1",
						InMaintenance: api.NotInMaintenance,
					},
				},
			}

			return nil
		},
		EvacuateFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData[server.Name] = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "1",
						VersionNext: "2",
						NeedsReboot: true,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.InMaintenanceEvacuated,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData[server.Name] = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "1",
					VersionNext: "2",
					NeedsReboot: true,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceEvacuating,
					},
				},
			}

			return nil
		},
		RebootFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData[server.Name] = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "2",
						VersionNext: "2",
						NeedsReboot: false,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.InMaintenanceEvacuated,
						},
					},
				}

				serverRebooting[server.Name] = false
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData[server.Name] = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "2",
					VersionNext: "2",
					NeedsReboot: true,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceEvacuated,
					},
				},
			}

			serverRebooting[server.Name] = true

			return nil
		},
		RestoreFunc: func(ctx context.Context, server provisioning.Server) error {
			go func() {
				time.Sleep(asyncActionsDelay)

				serverVersionDataMu.Lock()
				defer serverVersionDataMu.Unlock()

				serverVersionData[server.Name] = api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "incusos",
						Version:     "2",
						VersionNext: "2",
						NeedsReboot: false,
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "2",
							InMaintenance: api.NotInMaintenance,
						},
					},
				}
			}()

			serverVersionDataMu.Lock()
			defer serverVersionDataMu.Unlock()

			serverVersionData[server.Name] = api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "2",
					VersionNext: "2",
					NeedsReboot: false,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "2",
						InMaintenance: api.InMaintenanceRestoring,
					},
				},
			}

			return nil
		},
	}

	serverSvc := provisioning.NewServerService(serverDB, serverClient, nil, nil, channelSvc, updateSvc, tls.Certificate{})

	clusterSvc := provisioning.NewClusterService(clusterDB, nil, nil, serverSvc, nil, nil, nil,
		provisioning.WithClusterServicePendingUpdateRecheckInterval(controlLoopInterval),
	)

	serverSvc.SetClusterService(clusterSvc)

	// Run test
	err = clusterSvc.LaunchClusterUpdate(ctx, "clusterA")
	require.NoError(t, err)

	success := false
	for range 100 {
		c, err := clusterSvc.GetByName(ctx, "clusterA")
		if !c.UpdateStatus.InProgressStatus.InProgress {
			success = true
			break
		}

		err = clusterSvc.ClusterUpdateControlLoop(ctx)
		require.NoError(t, err)

		time.Sleep(controlLoopInterval)
	}

	require.True(t, success)
	log.Contains(`[1/12] evacuation pending server "one"`)(t, logBuf)
	log.Contains(`[2/12] evacuating server "one"`)(t, logBuf)
	log.Contains(`[3/12] in maintenance, reboot pending server "one"`)(t, logBuf)
	log.Contains(`[4/12] in maintenance, rebooting server "one"`)(t, logBuf)
	log.Contains(`[5/12] in maintenance, restore pending server "one"`)(t, logBuf)
	log.Contains(`[6/12] restoring server "one"`)(t, logBuf)
	log.Contains(`[7/12] evacuation pending server "two"`)(t, logBuf)
	log.Contains(`[8/12] evacuating server "two"`)(t, logBuf)
	log.Contains(`[9/12] in maintenance, reboot pending server "two"`)(t, logBuf)
	log.Contains(`[10/12] in maintenance, rebooting server "two"`)(t, logBuf)
	log.Contains(`[11/12] in maintenance, restore pending server "two"`)(t, logBuf)
	log.Contains(`[12/12] restoring server "two"`)(t, logBuf)
}
