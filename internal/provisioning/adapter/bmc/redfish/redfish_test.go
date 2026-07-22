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
		managersStatusCode    int
		managersBody          string
		managerStatusCode     int
		managerBody           string
		processorsStatusCode  int
		processorsBody        string
		processorStatusCode   int
		processorBody         string

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
  "Id": "1",
  "Manufacturer": "Dell Inc.",
  "Model": "PowerEdge R770",
  "SubModel": "SubModel",
  "UUID": "e9de436e-b94e-4aef-8563-883aec84096e",
  "AssetTag": "AssetTag1",
  "HostName": "host1",
  "SKU": "SKU123",
  "SerialNumber": "Serial123",
  "BiosVersion": "1.7.5",
  "PowerState": "On",
  "LocationIndicatorActive": true,
  "Status": { "Health": "OK" },
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" }
}`,
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`,
			managerStatusCode: http.StatusOK,
			managerBody: `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1",
  "Model": "iDRAC9",
  "FirmwareVersion": "1.30.20.10",
  "ServiceIdentification": "ServiceID1"
}`,
			processorsStatusCode: http.StatusOK,
			processorsBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1/Processors/1" }
  ]
}`,
			processorStatusCode: http.StatusOK,
			processorBody: `{
  "@odata.id": "/redfish/v1/Systems/1/Processors/1",
  "Id": "1",
  "ProcessorArchitecture": "x86",
  "InstructionSet": "x86-64"
}`,

			assertErr: require.NoError,
			want: api.BMCServerDetails{
				BMCProtocol:                   "Redfish",
				BMCProtocolVersion:            "1.16.0",
				BMCVendor:                     "Dell",
				BMCModel:                      "iDRAC9",
				BMCFirmwareVersion:            "1.30.20.10",
				BMCServiceIdentification:      "ServiceID1",
				ServerManufacturer:            "Dell Inc.",
				ServerModel:                   "PowerEdge R770",
				ServerSubModel:                "SubModel",
				ServerUUID:                    "e9de436e-b94e-4aef-8563-883aec84096e",
				ServerAssetTag:                "AssetTag1",
				ServerHostName:                "host1",
				ServerSKU:                     "SKU123",
				ServerSerialNumber:            "Serial123",
				ServerBIOSVersion:             "1.7.5",
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerPowerState:              "On",
				ServerLocationIndicatorActive: true,
				ServerHealthStatus:            "OK",
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
		{
			name: "error - failed to get BMC managers",

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
			managersStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no BMC managers found",

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
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 0,
  "Members": []
}`,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual BMC manager",

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
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`,
			managerStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get processors of BMC system",

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
  "Id": "1",
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" }
}`,
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`,
			managerStatusCode: http.StatusOK,
			managerBody: `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1"
}`,
			processorsStatusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
		{
			name: "error - no processors found for the BMC system",

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
  "Id": "1",
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" }
}`,
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`,
			managerStatusCode: http.StatusOK,
			managerBody: `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1"
}`,
			processorsStatusCode: http.StatusOK,
			processorsBody: `{
  "Members@odata.count": 0,
  "Members": []
}`,

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual processor of BMC system",

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
  "Id": "1",
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" }
}`,
			managersStatusCode: http.StatusOK,
			managersBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`,
			managerStatusCode: http.StatusOK,
			managerBody: `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1"
}`,
			processorsStatusCode: http.StatusOK,
			processorsBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1/Processors/1" }
  ]
}`,
			processorStatusCode: http.StatusInternalServerError,

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
				managersStatusCode:    tc.managersStatusCode,
				managersBody:          tc.managersBody,
				managerStatusCode:     tc.managerStatusCode,
				managerBody:           tc.managerBody,
				processorsStatusCode:  tc.processorsStatusCode,
				processorsBody:        tc.processorsBody,
				processorStatusCode:   tc.processorStatusCode,
				processorBody:         tc.processorBody,
			}, nil)

			client := redfish.New()

			before := time.Now()
			details, err := client.GetServerDetails(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{
					Endpoint: svr.URL,
				},
			})
			after := time.Now()

			tc.assertErr(t, err)

			if err == nil {
				require.WithinRange(t, details.LastUpdated, before, after)
			}

			details.LastUpdated = time.Time{}
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
			taskMonitor, err := client.Start(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

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
			taskMonitor, err := client.Stop(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

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
			taskMonitor, err := client.Restart(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

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
			err := client.WaitForTask(tc.argCtx(t), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.argTaskMonitor)

			tc.assertErr(t, err)
		})
	}
}
