package incus_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
)

func TestClientSupportBundle(t *testing.T) {
	t.Parallel()

	readStream := func(rc io.ReadCloser, err error) (any, error) {
		if err != nil {
			return nil, err
		}

		defer rc.Close()

		content, err := io.ReadAll(rc)
		return string(content), err
	}

	errNotFound := []queue.Item[response]{
		{
			Value: response{
				statusCode:   http.StatusNotFound,
				responseBody: []byte(`{"type": "error", "error": "Not found", "error_code": 404}`),
			},
		},
	}

	assertNotFound := func(tt require.TestingT, err error, a ...any) {
		require.ErrorContains(tt, err, "Not found")
		require.True(tt, incusapi.StatusErrorCheck(err, http.StatusNotFound))
	}

	methods := []methodTestSet{
		{
			name: "GetOS",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOS(ctx, server)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": {"environment": {"os_name": "IncusOS"}}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.JSONEq(t, `{"environment": {"os_name": "IncusOS"}}`, string(res.(json.RawMessage)))
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
		{
			name: "GetOSApplicationNames",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOSApplicationNames(ctx, server)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": ["/os/1.0/applications/incus", "/os/1.0/applications/ceph"]}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, []string{"incus", "ceph"}, res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
				{
					name: "error - invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": {}}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
		{
			name: "GetOSApplication",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOSApplication(ctx, server, "incus")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": {"state": {"initialized": true}}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications/incus"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.JSONEq(t, `{"state": {"initialized": true}}`, string(res.(json.RawMessage)))
					},
				},
			},
		},
		{
			name: "GetOSDebugLog",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOSDebugLog(ctx, server, 10)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": [{"MESSAGE": "System is ready"}]}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/debug/log?entries=10"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, []map[string]any{{"MESSAGE": "System is ready"}}, res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/log?entries=10"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
				{
					name: "error - invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": {}}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/log?entries=10"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
		{
			name: "GetOSDebugKernel",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOSDebugKernel(ctx, server)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": {}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/debug/kernel"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, incusosapi.DebugKernel{}, res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/kernel"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, incusosapi.DebugKernel{}, res)
					},
				},
				{
					name: "error - invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": []}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/kernel"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, incusosapi.DebugKernel{}, res)
					},
				},
			},
		},
		{
			name: "GetOSDebugProcesses",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return c.GetOSDebugProcesses(ctx, server)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": "PID COMMAND\n"}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/debug/processes"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "PID COMMAND\n", res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/processes"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Empty(t, res)
					},
				},
				{
					name: "error - invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata": []}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /os/1.0/debug/processes"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Empty(t, res)
					},
				},
			},
		},
		{
			name: "GetOSApplicationDebugLogs",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return readStream(c.GetOSApplicationDebugLogs(ctx, server, "incus"))
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`logs`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"POST /os/1.0/applications/incus/:debug"},
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.JSONEq(t, `{"action": "get-logs"}`, gotBodies[0])
					},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "logs", res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: assertNotFound,
					wantPaths: []string{"POST /os/1.0/applications/incus/:debug"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
				{
					name: "error - unexpected http status code without error response",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: func(tt require.TestingT, err error, a ...any) {
						require.ErrorContains(tt, err, "500 Internal Server Error")
					},
					wantPaths: []string{"POST /os/1.0/applications/incus/:debug"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
		{
			name: "GetInstanceLogfile",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return readStream(c.GetInstanceLogfile(ctx, server, "project one", "c1", "qemu.log"))
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`log`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/instances/c1/logs/qemu.log?project=project+one"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "log", res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: assertNotFound,
					wantPaths: []string{"GET /1.0/instances/c1/logs/qemu.log?project=project+one"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
		{
			name: "GetInstanceConsoleLog",
			clientCall: func(ctx context.Context, c incus.Client, server provisioning.Server) (any, error) {
				return readStream(c.GetInstanceConsoleLog(ctx, server, "default", "c1"))
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`console`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/instances/c1/console?project=default"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "console", res)
					},
				},
				{
					name:      "error - unexpected http status code",
					response:  errNotFound,
					assertErr: assertNotFound,
					wantPaths: []string{"GET /1.0/instances/c1/console?project=default"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Nil(t, res)
					},
				},
			},
		},
	}

	runMethodTestSets(t, methods)
}
