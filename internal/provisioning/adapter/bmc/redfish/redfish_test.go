package redfish_test

import (
	"net/http"
	"net/http/httptest"
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
			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/redfish/v1/":
						w.WriteHeader(tc.serviceRootStatusCode)

						if tc.serviceRootStatusCode == http.StatusOK {
							_, _ = w.Write([]byte(`{
  "Id": "RootService",
  "Name": "Root Service",
  "RedfishVersion": "1.16.0",
  "Vendor": "Dell",
  "Systems": { "@odata.id": "/redfish/v1/Systems" },
  "Managers": { "@odata.id": "/redfish/v1/Managers" }
}`))
						}

					case "/redfish/v1/Systems":
						w.WriteHeader(tc.systemsStatusCode)
						_, _ = w.Write([]byte(tc.systemsBody))

					case "/redfish/v1/Systems/1":
						w.WriteHeader(tc.systemStatusCode)
						_, _ = w.Write([]byte(tc.systemBody))

					case "/redfish/v1/Managers":
						w.WriteHeader(tc.managersStatusCode)
						_, _ = w.Write([]byte(tc.managersBody))

					case "/redfish/v1/Managers/1":
						w.WriteHeader(tc.managerStatusCode)
						_, _ = w.Write([]byte(tc.managerBody))

					case "/redfish/v1/Systems/1/Processors":
						w.WriteHeader(tc.processorsStatusCode)
						_, _ = w.Write([]byte(tc.processorsBody))

					case "/redfish/v1/Systems/1/Processors/1":
						w.WriteHeader(tc.processorStatusCode)
						_, _ = w.Write([]byte(tc.processorBody))

					default:
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					}
				}),
			)
			defer svr.Close()

			client := redfish.New()

			before := time.Now()
			details, err := client.GetServerDetails(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{
					BMCEndpoint: svr.URL,
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
