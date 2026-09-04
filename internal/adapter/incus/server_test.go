package incus_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusclient "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClientServer(t *testing.T) {
	methods := []methodTestSet{
		{
			name: "IsReady",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return nil, c.IsReady(ctx, server)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "system_is_ready": true
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0"},
				},
				{
					name: "success - compatibility if system_not_ready not present",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "uptime": 3600
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0"},
				},
				{
					name: "error - resource data invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0"},
				},
				{
					name: "error - not ready",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "system_is_ready": false
    }
  }
}`),
							},
						},
					},

					assertErr: func(tt require.TestingT, err error, a ...any) {
						require.True(tt, domain.IsRetryableError(err))
					},
					wantPaths: []string{"GET /os/1.0"},
				},
				{
					name: "error - not ready (legacy check based on uptime)",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "uptime": 0
    }
  }
}`),
							},
						},
					},

					assertErr: func(tt require.TestingT, err error, a ...any) {
						require.True(tt, domain.IsRetryableError(err))
					},
					wantPaths: []string{"GET /os/1.0"},
				},
			},
		},

		{
			name: "GetResources",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetResources(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "cpu": {
      "architecture": "x86_64"
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						want := api.HardwareData{
							Resources: incusapi.Resources{
								CPU: incusapi.ResourcesCPU{
									Architecture: "x86_64",
								},
							},
						}

						require.Equal(t, want, res)
					},
					wantPaths: []string{"GET /os/1.0/system/resources"},
				},
				{
					name: "error - resource data unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusInternalServerError,
								responseBody: []byte(http.StatusText(http.StatusInternalServerError)),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/system/resources"},
				},
				{
					name: "error - resource data invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/system/resources"},
				},
			},
		},
		{
			name: "GetOSData",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSData(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "encryption_recovery_keys": [ "very secret recovery key" ]
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "pools": [
        {
          "name": "some pool"
        }
      ]
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.OSData{
							Network: incusosapi.SystemNetwork{
								Config: &incusosapi.SystemNetworkConfig{
									DNS: &incusosapi.SystemNetworkDNS{
										Hostname: "foobar",
										Domain:   "local",
									},
								},
							},
							Security: incusosapi.SystemSecurity{
								Config: incusosapi.SystemSecurityConfig{
									EncryptionRecoveryKeys: []string{"very secret recovery key"},
								},
							},
							Storage: incusosapi.SystemStorage{
								Config: incusosapi.SystemStorageConfig{
									Pools: []incusosapi.SystemStoragePool{
										{
											Name: "some pool",
										},
									},
								},
							},
						}

						require.Equal(t, wantResources, res)
					},
				},
				{
					name: "error - network data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
				{
					name: "error - network data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
				{
					name: "error - security data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security"},
					assertResult: noResult,
				},
				{
					name: "error - security data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security"},
					assertResult: noResult,
				},
				{
					name: "error - storage data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "encryption_recovery_keys": [ "very secret recovery key" ]
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
				{
					name: "error - storage data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "encryption_recovery_keys": [ "very secret recovery key" ]
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetVersionData",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetVersionData(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success - Evacuating",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "status": "Evacuating"
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "channel": "stable"
    },
    "state": {
      "needs_reboot": true
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01", "GET /os/1.0/system/update"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.ServerVersionData{
							OS: api.OSVersionData{
								Name:        "IncusOS",
								Version:     "202511041601",
								VersionNext: "202512210545",
								NeedsReboot: true,
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:          "incus",
									Version:       "202511041601",
									InMaintenance: api.InMaintenanceEvacuating,
								},
							},
							UpdateChannel: "stable",
						}

						require.Equal(t, wantResources, res)
					},
				},
				{
					name: "success - Evacuated",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "status": "Evacuated"
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "channel": "stable"
    },
    "state": {
      "needs_reboot": true
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01", "GET /os/1.0/system/update"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.ServerVersionData{
							OS: api.OSVersionData{
								Name:        "IncusOS",
								Version:     "202511041601",
								VersionNext: "202512210545",
								NeedsReboot: true,
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:          "incus",
									Version:       "202511041601",
									InMaintenance: api.InMaintenanceEvacuated,
								},
							},
							UpdateChannel: "stable",
						}

						require.Equal(t, wantResources, res)
					},
				},
				{
					name: "success - Restoring",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "status": "Restoring"
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "channel": "stable"
    },
    "state": {
      "needs_reboot": true
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01", "GET /os/1.0/system/update"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.ServerVersionData{
							OS: api.OSVersionData{
								Name:        "IncusOS",
								Version:     "202511041601",
								VersionNext: "202512210545",
								NeedsReboot: true,
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:          "incus",
									Version:       "202511041601",
									InMaintenance: api.InMaintenanceRestoring,
								},
							},
							UpdateChannel: "stable",
						}

						require.Equal(t, wantResources, res)
					},
				},

				{
					name: "error - os version unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0"},
					assertResult: noResult,
				},
				{
					name: "error - os version invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0"},
					assertResult: noResult,
				},
				{
					name: "error - applications unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - applications invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`), // object for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - application incus unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus"},
					assertResult: noResult,
				},
				{
					name: "error - application incus invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus"},
					assertResult: noResult,
				},
				{
					name: "error - update unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01"},
					assertResult: noResult,
				},
				{
					name: "error - update unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "status": "Online"
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01", "GET /os/1.0/system/update"},
					assertResult: noResult,
				},
				{
					name: "error - update invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /1.0/cluster/members/server01
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "status": "Online"
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /1.0/cluster/members/server01", "GET /os/1.0/system/update"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetServerType",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetServerType(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/incus"
  ]
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						require.Equal(t, api.ServerTypeIncus, res)
					},
				},
				{
					name: "success - multiple applications",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/other-application",
    "/os/1.0/applications/incus",
    "/os/1.0/applications/more-application"
  ]
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						require.Equal(t, api.ServerTypeIncus, res)
					},
				},
				{
					name: "error - network data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - network data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`), // object for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "invalid application",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/invalid"
  ]
}`), // invalid application
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "invalid application (empty)",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/"
  ]
}`), // invalid application (empty)
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetNodeSpecificConfigKeys",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetNodeSpecificConfigKeys(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "configs": {
      "server": {
        "core": {
          "keys": [
            {
              "core.https_address": {
                "scope": "local",
                "shortdesc": "Address to bind for the remote API (HTTPS)",
                "type": "string"
              }
            }
          ]
        }
      }
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						want := map[string]map[string]bool{
							"server": {
								"core.https_address": true,
							},
							// NOTE: compensate for the lack of information about "storage_lvmcluster" in /1.0/metadata/configuration.
							"storage_lvmcluster": nil,
						}

						require.Equal(t, want, res)
					},
					wantPaths: []string{"GET /1.0/metadata/configuration"},
				},
				{
					name: "error - resource data unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusInternalServerError,
								responseBody: []byte(http.StatusText(http.StatusInternalServerError)),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/metadata/configuration"},
				},
				{
					name: "error - resource data invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/metadata/configuration"},
				},
			},
		},
		{
			name: "AddApplication",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.AddApplication(ctx, target, "debug")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// POST /os/1.0/applications
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/applications"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// POST /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"POST /os/1.0/applications"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "RestartApplication",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.RestartApplication(ctx, target, "openfga")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// POST /os/1.0/applications/openfga/:restart
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/applications/openfga/:restart"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// POST /os/1.0/applications/openfga/:restart
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"POST /os/1.0/applications/openfga/:restart"},
					assertResult: noResult,
				},
			},
		},

		{
			name: "UpdateApplication",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateApplication(ctx, target, "openfga")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// POST /os/1.0/applications/openfga/:check-update
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/applications/openfga/:check-update"},
				},
				{
					// IncusOS answers 404 for an application it does not have
					// installed.
					name: "error - application not installed",
					response: []queue.Item[response]{
						// POST /os/1.0/applications/openfga/:check-update
						{
							Value: response{
								statusCode: http.StatusNotFound,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"POST /os/1.0/applications/openfga/:check-update"},
					assertResult: noResult,
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// POST /os/1.0/applications/openfga/:check-update
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"POST /os/1.0/applications/openfga/:check-update"},
					assertResult: noResult,
				},
			},
		},

		{
			name: "GetSystem",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetSystem(ctx, target, "kernel")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "blacklist_modules": [
        "bad-module"
      ],
      "network": {
        "buffer_size": 33554432
      },
      "pci": {
        "passthrough": [
          {
            "product_id": "1050"
          }
        ]
      }
    }
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/kernel"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantConfig := map[string]any{
							"config": map[string]any{
								"blacklist_modules": []any{"bad-module"},
								"network": map[string]any{
									"buffer_size": float64(33554432),
								},
								"pci": map[string]any{
									"passthrough": []any{
										map[string]any{
											"product_id": "1050",
										},
									},
								},
							},
						}

						require.Equal(t, wantConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/kernel"},
					assertResult: noResult,
				},
				{
					name: "error - kernel config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/kernel"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateSystem",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateSystem(ctx, target, "kernel", incusosapi.SystemKernel{})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/kernel
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/kernel"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"PUT /os/1.0/system/kernel"},
					assertResult: noResult,
				},
			},
		},

		{
			name: "GetSystemKernel",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetSystemKernel(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "blacklist_modules": [
        "bad-module"
      ],
      "network": {
        "buffer_size": 33554432
      },
      "pci": {
        "passthrough": [
          {
            "product_id": "1050"
          }
        ]
      }
    }
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/kernel"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantKernelConfig := provisioning.ServerSystemKernel{
							Config: incusosapi.SystemKernelConfig{
								BlacklistModules: []string{"bad-module"},
								Network: &incusosapi.SystemKernelConfigNetwork{
									BufferSize: 33554432,
								},
								PCI: &incusosapi.SystemKernelConfigPCI{
									Passthrough: []incusosapi.SystemKernelConfigPCIPassthrough{
										{
											ProductID: "1050",
										},
									},
								},
							},
						}

						require.Equal(t, wantKernelConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/kernel"},
					assertResult: noResult,
				},
				{
					name: "error - kernel config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/kernel"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateSystemKernel",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateSystemKernel(ctx, target, incusosapi.SystemKernel{})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/kernel
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/kernel"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/kernel
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"PUT /os/1.0/system/kernel"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetSystemLogging",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetSystemLogging(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/logging
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "syslog": {
        "address": "localhost"
      }
    },
    "state": {}
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/logging"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantLoggingConfig := provisioning.ServerSystemLogging{
							Config: incusosapi.SystemLoggingConfig{
								Syslog: incusosapi.SystemLoggingSyslog{
									Address: "localhost",
								},
							},
							State: incusosapi.SystemLoggingState{},
						}

						require.Equal(t, wantLoggingConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/logging
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/logging"},
					assertResult: noResult,
				},
				{
					name: "error - logging config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/logging
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/logging"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateSystemLogging",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateSystemLogging(ctx, target, incusosapi.SystemLogging{})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/logging
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/logging"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// PUT /os/1.0/system/logging
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"PUT /os/1.0/system/logging"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetSecurityConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetSecurityConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "custom_ca_certs": [
        "certificate"
      ]
    },
    "state": {}
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/security"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantLoggingConfig := provisioning.ServerSystemSecurity{
							Config: incusosapi.SystemSecurityConfig{
								CustomCACerts: []string{"certificate"},
							},
							State: incusosapi.SystemSecurityState{},
						}

						require.Equal(t, wantLoggingConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/security"},
					assertResult: noResult,
				},
				{
					name: "error - security config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/security"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetOSService",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSService(ctx, target, "lvm")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "system_id": 15
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := map[string]any{
							"config": map[string]any{
								"enabled":   true,
								"system_id": 15.0,
							},
							"state": map[string]any{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/lvm"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/lvm"},
				},
				{
					name: "error - iscsi service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/lvm
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/lvm"},
				},
			},
		},
		{
			name: "GetOSServiceCeph",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceCeph(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "clusters": {
        "one": {
          "fsid": "1"
        }
      }
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceCeph{
							Config: incusosapi.ServiceCephConfig{
								Enabled: true,
								Clusters: map[string]incusosapi.ServiceCephCluster{
									"one": {
										FSID: "1",
									},
								},
							},
							State: incusosapi.ServiceCephState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/ceph"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/ceph"},
				},
				{
					name: "error - ceph service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/ceph
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/ceph"},
				},
			},
		},
		{
			name: "GetOSServiceISCSI",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceISCSI(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "targets": [
        {
          "target": "target",
          "address": "address",
          "port": 1234
        }
      ]
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceISCSI{
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
							State: incusosapi.ServiceISCSIState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/iscsi"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/iscsi"},
				},
				{
					name: "error - iscsi service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/iscsi
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/iscsi"},
				},
			},
		},
		{
			name: "GetOSServiceLinstor",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceLinstor(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "listen_address": "address"
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceLinstor{
							Config: incusosapi.ServiceLinstorConfig{
								Enabled:       true,
								ListenAddress: "address",
							},
							State: incusosapi.ServiceLinstorState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/linstor"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/linstor"},
				},
				{
					name: "error - linstor service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/linstor
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/linstor"},
				},
			},
		},
		{
			name: "GetOSServiceLVM",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceLVM(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "system_id": 1
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceLVM{
							Config: incusosapi.ServiceLVMConfig{
								Enabled:  true,
								SystemID: 1.0,
							},
							State: incusosapi.ServiceLVMState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/lvm"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/lvm"},
				},
				{
					name: "error - lvm service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/lvm
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/lvm"},
				},
			},
		},
		{
			name: "GetOSServiceMultipath",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceMultipath(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "wwns": [ "one" ]
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceMultipath{
							Config: incusosapi.ServiceMultipathConfig{
								Enabled: true,
								WWNs:    []string{"one"},
							},
							State: incusosapi.ServiceMultipathState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/multipath"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/multipath"},
				},
				{
					name: "error - multipath service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/multipath
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/multipath"},
				},
			},
		},
		{
			name: "GetOSServiceNVME",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceNVME(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "targets": [
        {
          "transport": "transport",
          "address": "address",
          "port": 1234
        }
      ]
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceNVME{
							Config: incusosapi.ServiceNVMEConfig{
								Enabled: true,
								Targets: []incusosapi.ServiceNVMETarget{
									{
										Transport: "transport",
										Address:   "address",
										Port:      1234,
									},
								},
							},
							State: incusosapi.ServiceNVMEState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/nvme"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/nvme"},
				},
				{
					name: "error - nvme service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/nvme
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/nvme"},
				},
			},
		},
		{
			name: "GetOSServiceOVN",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceOVN(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "ic_chassis": true
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceOVN{
							Config: incusosapi.ServiceOVNConfig{
								Enabled:   true,
								ICChassis: true,
							},
							State: incusosapi.ServiceOVNState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/ovn"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/ovn"},
				},
				{
					name: "error - ovn service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/ovn
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/ovn"},
				},
			},
		},
		{
			name: "GetOSServiceTailscale",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceTailscale(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "login_server": "server"
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceTailscale{
							Config: incusosapi.ServiceTailscaleConfig{
								Enabled:     true,
								LoginServer: "server",
							},
							State: incusosapi.ServiceTailscaleState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/tailscale"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/tailscale"},
				},
				{
					name: "error - tailscale service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/tailscale
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/tailscale"},
				},
			},
		},
		{
			name: "GetOSServiceUSBIP",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetOSServiceUSBIP(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "enabled": true,
      "targets": [
        {
          "address": "address"
        }
      ]
    },
    "state": {}
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantOSService := incusosapi.ServiceUSBIP{
							Config: incusosapi.ServiceUSBIPConfig{
								Enabled: true,
								Targets: []incusosapi.ServiceUSBIPTarget{
									{
										Address: "address",
									},
								},
							},
							State: incusosapi.ServiceUSBIPState{},
						}

						require.Equal(t, wantOSService, res)
					},
					wantPaths: []string{"GET /os/1.0/services/usbip"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/usbip"},
				},
				{
					name: "error - usbip service config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/services/usbip
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/services/usbip"},
				},
			},
		},

		{
			name: "UpdateOSService - config map",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateOSService(ctx, target, "lvm", map[string]any{"enabled": true})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.JSONEq(t, `{
  "config": {
    "enabled": true
  }
}`, gotBodies[0])
					},
					wantPaths: []string{"PUT /os/1.0/services/lvm"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/services/lvm"},
				},
			},
		},
		{
			name: "UpdateOSService - service type",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateOSService(ctx, target, "iscsi", incusosapi.ServiceISCSI{
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
				})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.JSONEq(t, `{
  "config": {
    "enabled": true,
    "targets": [
      {
        "target": "target",
        "address": "address",
        "port": 1234
      }
    ]
  },
  "state": {
    "initiator_name": ""
  }
}`, gotBodies[0])
					},
					wantPaths: []string{"PUT /os/1.0/services/iscsi"},
				},
			},
		},
		{
			name: "SetServerConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.SetServerConfig(ctx, target, map[string]string{
					"key": "value",
				})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// PUT /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0", "PUT /1.0"},
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.Contains(t, gotBodies[1], `"key":"value"`)
					},
				},
				{
					name: "error - GetServer - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0"},
				},
				{
					name: "error - UpdateServer - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// PUT /1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0", "PUT /1.0"},
				},
			},
		},
		{
			name: "EnableCluster",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.EnableCluster(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "metadata":{
      "certificate": "certificate"
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "certificate", res)
					},
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "success - no certificate returned",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "metadata":{
    }
  }
}`), // no certificate returned
							},
						},
					},

					assertErr:    require.NoError,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "success - invalid type for certificate",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "metadata":{
      "certificate": {}
    }
  }
}`), // invalid type for certificate
							},
						},
					},

					assertErr:    require.NoError,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "error - UpdateCluster - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster"},
				},
				{
					name: "error - fail op.WaitContext",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusInternalServerError, // fail op.WaitContext
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
			},
		},
		{
			name: "GetNetworkConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetNetworkConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "interfaces": [
        {
          "name": "enp5s0"
        }
      ],
      "time": {
        "timezone": "UTC"
      }
    },
    "state": {
      "interfaces": {
        "enp5s0": {
          "type": "interface"
        }
      }
    }
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantNetworkConfig := provisioning.ServerSystemNetwork{
							Config: &incusosapi.SystemNetworkConfig{
								Interfaces: []incusosapi.SystemNetworkInterface{
									{
										Name: "enp5s0",
									},
								},
								Time: &incusosapi.SystemNetworkTime{
									Timezone: "UTC",
								},
							},
							State: incusosapi.SystemNetworkState{
								Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
									"enp5s0": {
										Type: "interface",
									},
								},
							},
						}

						require.Equal(t, wantNetworkConfig, res)
					},
					wantPaths: []string{"GET /os/1.0/system/network"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
				{
					name: "error - network config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateNetworkConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateNetworkConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/network"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/system/network"},
				},
			},
		},
		{
			name: "GetStorageConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetStorageConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "drives": [
        {
          "id": "/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_incus_root"
        }
      ],
      "pools": [
        {
          "name": "local"
        }
      ]
    }
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantStorageConfig := provisioning.ServerSystemStorage{
							Config: incusosapi.SystemStorageConfig{},
							State: incusosapi.SystemStorageState{
								Drives: []incusosapi.SystemStorageDrive{
									{
										ID: "/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_incus_root",
									},
								},
								Pools: []incusosapi.SystemStoragePool{
									{
										Name: "local",
									},
								},
							},
						}

						require.Equal(t, wantStorageConfig, res)
					},
					wantPaths: []string{"GET /os/1.0/system/storage"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
				{
					name: "error - storage config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateStorageConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateStorageConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/storage"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/system/storage"},
				},
			},
		},
		{
			name: "GetProviderConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetProviderConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":{"config":{"name":"operations-center","config":{"server_certificate":"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n","server_token":"5df55e4e-3bfb-46c8-94c3-b04b56f9e3ba","server_url":"https://192.168.1.200:8443"}},"state":{"registered":true}}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/provider"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantProviderConfig := provisioning.ServerSystemProvider{
							Config: incusosapi.SystemProviderConfig{
								Name: "operations-center",
								Config: map[string]string{
									"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
									"server_token":       "5df55e4e-3bfb-46c8-94c3-b04b56f9e3ba",
									"server_url":         "https://192.168.1.200:8443",
								},
							},
							State: incusosapi.SystemProviderState{
								Registered: true,
							},
						}

						require.Equal(t, wantProviderConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/provider"},
					assertResult: noResult,
				},
				{
					name: "error - provider config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/provider"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateProviderConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateProviderConfig(ctx, target, incusosapi.SystemProvider{
					Config: incusosapi.SystemProviderConfig{
						Config: map[string]string{
							"server_url": "https://new-operations-center:8443",
						},
					},
				})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/provider"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/system/provider"},
				},
			},
		},
		{
			name: "GetUpdateConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.GetUpdateConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":{"config":{"auto_reboot": false,"channel": "stable","check_frequency": "6h"},"state":{"last_check": "2025-11-04T16:21:34.929524792Z","needs_reboot": false,"status": "Update check completed"}}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/update"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantUpdateConfig := provisioning.ServerSystemUpdate{
							Config: incusosapi.SystemUpdateConfig{
								AutoReboot:     false,
								Channel:        "stable",
								CheckFrequency: "6h",
							},
							State: incusosapi.SystemUpdateState{
								LastCheck:   time.Date(2025, 11, 4, 16, 21, 34, 929524792, time.UTC),
								NeedsReboot: false,
								Status:      "Update check completed",
							},
						}

						require.Equal(t, wantUpdateConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/update"},
					assertResult: noResult,
				},
				{
					name: "error - update config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/update"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateUpdateConfig",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateUpdateConfig(ctx, target, incusosapi.SystemUpdate{
					Config: incusosapi.SystemUpdateConfig{
						Channel: "stable",
					},
				})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/update"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/system/update"},
				},
			},
		},
		{
			name: "Evacuate",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.Evacuate(ctx, target, func(ctx context.Context, err error) {})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
				},
			},
		},
		{
			name: "TriggerSystemAction",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.TriggerSystemAction(ctx, target, "", "poweroff", nil)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
				},
			},
		},
		{
			name: "Poweroff",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.Poweroff(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
				},
			},
		},
		{
			name: "Reboot",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.Reboot(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/system/:reboot"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"POST /os/1.0/system/:reboot"},
				},
			},
		},
		{
			name: "Restore - normal mode",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.Restore(ctx, target, false, func(ctx context.Context, err error) {})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.Contains(t, gotBodies[1], `"mode":""`)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
				},
			},
		},
		{
			name: "Restore - skip mode",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.Restore(ctx, target, true, func(ctx context.Context, err error) {})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.Contains(t, gotBodies[1], `"mode":"skip"`)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members/server01/state
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members/server01/state"},
				},
			},
		},
		{
			name: "UpdateOS",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.UpdateOS(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/system/update/:check"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"POST /os/1.0/system/update/:check"},
				},
			},
		},
		{
			name: "JoinCluster",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return nil, client.JoinCluster(ctx, target, "token", "10.10.10.10:8443", provisioning.ClusterEndpoint{}, nil)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "error - UpdateCluster - unexpected status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster"},
				},
				{
					name: "error - fail op.WaitContext",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusInternalServerError, // fail op.WaitContext
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
			},
		},

		{
			name: "IncusClient",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (any, error) {
				return client.IncusClient(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						require.Implements(t, (*incusclient.InstanceServer)(nil), res)
						require.NotNil(t, res)
					},
				},
			},
		},
	}

	runMethodTestSets(t, methods)
}
