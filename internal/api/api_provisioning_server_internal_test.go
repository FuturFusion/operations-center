package api

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_serverHandler_serverPut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		ifMatch             string
		serviceGetByNameErr error
		serviceUpdateErr    error

		wantUpdateCalls int
		wantStatusCode  int
	}{
		{
			name: "success",

			wantUpdateCalls: 1,
			wantStatusCode:  http.StatusOK,
		},
		{
			name:                "error - service.GetByName",
			serviceGetByNameErr: boom.Error,

			wantUpdateCalls: 0,
			wantStatusCode:  http.StatusInternalServerError,
		},
		{
			name:    "error - ETag mismatch",
			ifMatch: `"invalid"`,

			wantUpdateCalls: 0,
			wantStatusCode:  http.StatusPreconditionFailed,
		},
		{
			name:             "error - service.Update",
			serviceUpdateErr: boom.Error,

			wantUpdateCalls: 1,
			wantStatusCode:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			service := &provisioningMock.ServerServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					require.False(t, transaction.IsActive(ctx), "the handler must not open a transaction, the update calls the BMC and the server API")

					return &provisioning.Server{Name: name}, tc.serviceGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server, force bool, updateSystem bool, bmcConnectionTest bool) error {
					require.False(t, transaction.IsActive(ctx), "the handler must not open a transaction, the update calls the BMC and the server API")
					require.True(t, bmcConnectionTest, "an update through the API must test the BMC connection")

					return tc.serviceUpdateErr
				},
			}

			handler := &serverHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodPut, "/one", strings.NewReader(`{}`))
			req.SetPathValue("name", "one")
			req.Header.Set("If-Match", tc.ifMatch)

			rec := httptest.NewRecorder()

			// Run test
			err := handler.serverPut(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Len(t, service.UpdateCalls(), tc.wantUpdateCalls)
			require.Equal(t, tc.wantStatusCode, rec.Code)
		})
	}
}

func Test_serverHandler_serversGet(t *testing.T) {
	t.Parallel()

	slimKeys := []string{
		"bmc_config",
		"certificate",
		"channel",
		"cluster",
		"connection_url",
		"deployment_state",
		"description",
		"fingerprint",
		"last_seen",
		"last_updated",
		"machine_id",
		"name",
		"properties",
		"public_connection_url",
		"server_status",
		"server_status_detail",
		"server_type",
		"system_state_is_trusted",
		"system_uuid",
		"version_data",
	}

	fullKeys := append(slices.Clone(slimKeys), "bmc_data", "deployment", "hardware_data", "os_data")
	slices.Sort(fullKeys)

	assertURLs := func(t *testing.T, metadata json.RawMessage) {
		t.Helper()

		var urls []string
		err := json.Unmarshal(metadata, &urls)
		require.NoError(t, err, "metadata must be a list of URLs")
		require.Equal(t, []string{"/1.0/provisioning/servers/one"}, urls, "URLs in the response")
	}

	assertServers := func(wantKeys []string) func(t *testing.T, metadata json.RawMessage) {
		return func(t *testing.T, metadata json.RawMessage) {
			t.Helper()

			var servers []map[string]json.RawMessage
			err := json.Unmarshal(metadata, &servers)
			require.NoError(t, err, "metadata must be a list of server objects")
			require.Len(t, servers, 1, "number of servers in the response")
			require.Equal(t, wantKeys, slices.Sorted(maps.Keys(servers[0])), "keys of the server in the response")
			require.JSONEq(t, `"wait-install"`, string(servers[0]["deployment_state"]), "deployment state of the server")
		}
	}

	tests := []struct {
		name  string
		query string

		assertMetadata func(t *testing.T, metadata json.RawMessage)
	}{
		{
			name:  "missing recursion returns URLs",
			query: "",

			assertMetadata: assertURLs,
		},
		{
			name:  "invalid recursion returns URLs",
			query: "?recursion=abc",

			assertMetadata: assertURLs,
		},
		{
			name:  "recursion 1 returns servers without the large fields",
			query: "?recursion=1",

			assertMetadata: assertServers(slimKeys),
		},
		{
			name:  "recursion 2 returns full servers",
			query: "?recursion=2",

			assertMetadata: assertServers(fullKeys),
		},
		{
			name:  "recursion above 2 returns full servers",
			query: "?recursion=3",

			assertMetadata: assertServers(fullKeys),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			service := &provisioningMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{
						{
							Name: "one",
							StatusInternal: provisioning.ServerStatusInternal{
								Deployment: &provisioning.ServerDeployment{
									State: api.ServerDeploymentStateWaitInstall,
								},
							},
						},
					}, nil
				},
				GetAllNamesWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
					return []string{"one"}, nil
				},
			}

			handler := &serverHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodGet, "/1.0/provisioning/servers"+tc.query, http.NoBody)
			rec := httptest.NewRecorder()

			// Run test
			err := handler.serversGet(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Equal(t, http.StatusOK, rec.Code, "status code of the response")

			var resp struct {
				Metadata json.RawMessage `json:"metadata"`
			}

			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err, "response body must be valid JSON")

			tc.assertMetadata(t, resp.Metadata)
		})
	}
}

func Test_toAPIServer_deploymentState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server provisioning.Server

		wantDeploymentState api.ServerDeploymentState
	}{
		{
			name:   "no deployment",
			server: provisioning.Server{Name: "one"},

			wantDeploymentState: "",
		},
		{
			name: "deployment in progress",
			server: provisioning.Server{
				Name: "one",
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State: api.ServerDeploymentStateWaitInstall,
					},
				},
			},

			wantDeploymentState: api.ServerDeploymentStateWaitInstall,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			server := toAPIServer(tc.server)

			// Assert
			require.Equal(t, tc.wantDeploymentState, server.DeploymentState, "deployment state of the server")
		})
	}
}
