package redfish_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bmc/redfish"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestRedfish_GetServerDetails(t *testing.T) {
	tests := []struct {
		name string

		serviceRootStatusCode int
		systemsStatusCode     int
		systemsBody           string
		systemStatusCode      int
		systemBody            string

		assertErr require.ErrorAssertionFunc
		want      api.BMCServerDetails
	}{
		{
			name: "success",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" }
  ]
}`,
			systemStatusCode: http.StatusOK,
			systemBody: `{
  "@odata.id": "/redfish/v1/Systems/1",
  "Id": "1"
}`,

			assertErr: require.NoError,
			want: api.BMCServerDetails{
				SystemUUID: "1",
			},
		},
		{
			name: "error - failed to connect to BMC",

			serviceRootStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody: `{
  "Members@odata.count": 0,
  "Members": []
}`,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual BMC system",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" }
  ]
}`,
			systemStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode: tc.serviceRootStatusCode,
				systemsStatusCode:     tc.systemsStatusCode,
				systemsBody:           tc.systemsBody,
				systemStatusCode:      tc.systemStatusCode,
				systemBody:            tc.systemBody,
			}, nil)

			client := redfish.New()
			details, err := client.GetServerDetails(t.Context(), provisioning.Server{
				BMCEndpoint: svr.URL,
			})

			tc.assertErr(t, err)
			require.Equal(t, tc.want, details)
		})
	}
}

const (
	resetSystemsBody = `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" }
  ]
}`

	resetSystemBody = `{
  "@odata.id": "/redfish/v1/Systems/1",
  "Id": "1",
  "Actions": {
    "#ComputerSystem.Reset": {
      "Target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
    }
  }
}`

	resetEmptySystemsBody = `{
  "Members@odata.count": 0,
  "Members": []
}`
)

func TestRedfish_Start(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode int
		systemsStatusCode     int
		systemsBody           string
		systemStatusCode      int
		systemBody            string
		resetStatusCode       int
		resetLocation         string

		wantResetType   string
		wantTaskMonitor *provisioning.BMCTaskMonitor
		assertErr       require.ErrorAssertionFunc
	}{
		{
			name:  "success - not forced",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "On",
			assertErr:     require.NoError,
		},
		{
			name:  "success - forced",
			force: true,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "ForceOn",
			assertErr:     require.NoError,
		},
		{
			name:  "success - task monitor returned",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusAccepted,
			resetLocation:         "/redfish/v1/TaskMonitor/1",

			wantResetType: "On",
			wantTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},
			assertErr: require.NoError,
		},
		{
			name: "error - failed to connect to BMC",

			serviceRootStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetEmptySystemsBody,

			assertErr: require.Error,
		},
		{
			name: "error - reset action failed",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode: tc.serviceRootStatusCode,
				systemsStatusCode:     tc.systemsStatusCode,
				systemsBody:           tc.systemsBody,
				systemStatusCode:      tc.systemStatusCode,
				systemBody:            tc.systemBody,
				resetStatusCode:       tc.resetStatusCode,
				resetLocation:         tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.Start(t.Context(), provisioning.Server{BMCEndpoint: svr.URL}, tc.force)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantTaskMonitor, taskMonitor)

			if tc.wantResetType != "" {
				require.JSONEq(t, fmt.Sprintf(`{"ResetType":%q}`, tc.wantResetType), string(gotBody))
			}
		})
	}
}

func TestRedfish_Stop(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode int
		systemsStatusCode     int
		systemsBody           string
		systemStatusCode      int
		systemBody            string
		resetStatusCode       int
		resetLocation         string

		wantResetType   string
		wantTaskMonitor *provisioning.BMCTaskMonitor
		assertErr       require.ErrorAssertionFunc
	}{
		{
			name:  "success - not forced",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "GracefulShutdown",
			assertErr:     require.NoError,
		},
		{
			name:  "success - forced",
			force: true,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "ForceOff",
			assertErr:     require.NoError,
		},
		{
			name:  "success - task monitor returned",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusAccepted,
			resetLocation:         "/redfish/v1/TaskMonitor/1",

			wantResetType: "GracefulShutdown",
			wantTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},
			assertErr: require.NoError,
		},
		{
			name: "error - failed to connect to BMC",

			serviceRootStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetEmptySystemsBody,

			assertErr: require.Error,
		},
		{
			name: "error - reset action failed",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode: tc.serviceRootStatusCode,
				systemsStatusCode:     tc.systemsStatusCode,
				systemsBody:           tc.systemsBody,
				systemStatusCode:      tc.systemStatusCode,
				systemBody:            tc.systemBody,
				resetStatusCode:       tc.resetStatusCode,
				resetLocation:         tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.Stop(t.Context(), provisioning.Server{BMCEndpoint: svr.URL}, tc.force)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantTaskMonitor, taskMonitor)

			if tc.wantResetType != "" {
				require.JSONEq(t, fmt.Sprintf(`{"ResetType":%q}`, tc.wantResetType), string(gotBody))
			}
		})
	}
}

func TestRedfish_Restart(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode int
		systemsStatusCode     int
		systemsBody           string
		systemStatusCode      int
		systemBody            string
		resetStatusCode       int
		resetLocation         string

		wantResetType   string
		wantTaskMonitor *provisioning.BMCTaskMonitor
		assertErr       require.ErrorAssertionFunc
	}{
		{
			name:  "success - not forced",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "GracefulRestart",
			assertErr:     require.NoError,
		},
		{
			name:  "success - forced",
			force: true,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusNoContent,

			wantResetType: "ForceRestart",
			assertErr:     require.NoError,
		},
		{
			name:  "success - task monitor returned",
			force: false,

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusAccepted,
			resetLocation:         "/redfish/v1/TaskMonitor/1",

			wantResetType: "GracefulRestart",
			wantTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},
			assertErr: require.NoError,
		},
		{
			name: "error - failed to connect to BMC",

			serviceRootStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetEmptySystemsBody,

			assertErr: require.Error,
		},
		{
			name: "error - reset action failed",

			serviceRootStatusCode: http.StatusOK,
			systemsStatusCode:     http.StatusOK,
			systemsBody:           resetSystemsBody,
			systemStatusCode:      http.StatusOK,
			systemBody:            resetSystemBody,
			resetStatusCode:       http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode: tc.serviceRootStatusCode,
				systemsStatusCode:     tc.systemsStatusCode,
				systemsBody:           tc.systemsBody,
				systemStatusCode:      tc.systemStatusCode,
				systemBody:            tc.systemBody,
				resetStatusCode:       tc.resetStatusCode,
				resetLocation:         tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.Restart(t.Context(), provisioning.Server{BMCEndpoint: svr.URL}, tc.force)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantTaskMonitor, taskMonitor)

			if tc.wantResetType != "" {
				require.JSONEq(t, fmt.Sprintf(`{"ResetType":%q}`, tc.wantResetType), string(gotBody))
			}
		})
	}
}

func TestRedfish_WaitForTask(t *testing.T) {
	tests := []struct {
		name           string
		argCtx         func(t *testing.T) context.Context
		argTaskMonitor *provisioning.BMCTaskMonitor

		serviceRootStatusCode  int
		taskMonitorStatusCodes []int
		taskMonitorRetryAfter  string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - nil TaskMonitor",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: nil,

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusOK},

			assertErr: require.NoError,
		},
		{
			name: "success - already finished",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusOK},

			assertErr: require.NoError,
		},
		{
			name: "success - finished with created status",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusCreated},

			assertErr: require.NoError,
		},
		{
			name: "success - polls until finished",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusAccepted, http.StatusAccepted, http.StatusOK},
			taskMonitorRetryAfter:  "0",

			assertErr: require.NoError,
		},
		{
			name: "error - failed to connect to BMC",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - unexpected status code polling task",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusNotFound},

			assertErr: require.Error,
		},
		{
			name: "error - context already canceled",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()

				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusOK},

			assertErr: require.Error,
		},
		{
			name: "error - context canceled while waiting",
			argCtx: func(t *testing.T) context.Context {
				t.Helper()

				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				t.Cleanup(cancel)

				return ctx
			},
			argTaskMonitor: &provisioning.BMCTaskMonitor{
				URI: "/redfish/v1/TaskMonitor/1",
			},

			serviceRootStatusCode:  http.StatusOK,
			taskMonitorStatusCodes: []int{http.StatusAccepted},
			taskMonitorRetryAfter:  "5",

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode:  tc.serviceRootStatusCode,
				taskMonitorStatusCodes: tc.taskMonitorStatusCodes,
				taskMonitorRetryAfter:  tc.taskMonitorRetryAfter,
			}, nil)

			client := redfish.New()
			err := client.WaitForTask(tc.argCtx(t), provisioning.Server{BMCEndpoint: svr.URL}, tc.argTaskMonitor)

			tc.assertErr(t, err)
		})
	}
}
