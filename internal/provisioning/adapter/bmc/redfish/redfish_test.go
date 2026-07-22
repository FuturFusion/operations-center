package redfish_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bmc/redfish"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestRedfish_ConnectionTest(t *testing.T) {
	responses := mockRedfishServer{
		serviceRootStatusCode: http.StatusOK,
	}

	noSANServer, noSANCertPEM := newTLSServerWithoutSAN(t, responses)

	tests := []struct {
		name string

		svr                *httptest.Server
		autoPinCertificate bool
		certificate        func(t *testing.T, serverCert *x509.Certificate) string

		assertErr       require.ErrorAssertionFunc
		wantCertificate func(t *testing.T, serverCert *x509.Certificate) string
	}{
		{
			name: "success - SAN certificate: no existing certificate fetches and trusts remote certificate",
			// The default httptest TLS certificate carries SAN DNS/IP entries.
			svr:                httptest.NewTLSServer(newMockRedfishHandler(responses, nil)),
			autoPinCertificate: true,
			certificate: func(_ *testing.T, _ *x509.Certificate) string {
				return ""
			},

			assertErr: require.NoError,
			wantCertificate: func(_ *testing.T, serverCert *x509.Certificate) string {
				return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}))
			},
		},
		{
			name:               "success - SAN certificate: existing trusted certificate skips fetching new certificate",
			svr:                httptest.NewTLSServer(newMockRedfishHandler(responses, nil)),
			autoPinCertificate: true,
			certificate: func(_ *testing.T, serverCert *x509.Certificate) string {
				return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}))
			},

			assertErr: require.NoError,
		},
		{
			name:               "success - certificate without SAN pinned: fingerprint matches presented certificate",
			svr:                noSANServer,
			autoPinCertificate: false,
			certificate: func(_ *testing.T, _ *x509.Certificate) string {
				return noSANCertPEM
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - certificate without SAN pinned: fingerprint mismatch with presented certificate",
			svr:                httptest.NewTLSServer(newMockRedfishHandler(responses, nil)),
			autoPinCertificate: false,
			certificate: func(t *testing.T, _ *x509.Certificate) string {
				t.Helper()
				// Generate separate certificate without any SAN.
				certPEMByte, _, err := incustls.GenerateMemCert(true, false)
				require.NoError(t, err)

				return string(certPEMByte)
			},

			assertErr: errassert.Contains("Certificate fingerprint mismatch"),
		},
		{
			name:               "error - failed to get remote certificate during connection test",
			svr:                httptest.NewServer(newMockRedfishHandler(responses, nil)), // not https
			autoPinCertificate: true,
			certificate: func(_ *testing.T, _ *x509.Certificate) string {
				return ""
			},

			assertErr: errassert.Contains("Failed to get remote certificate from BMC during connection test"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.svr.Close()

			client := redfish.New()

			cert, err := client.ConnectionTest(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{
					Endpoint:           tc.svr.URL,
					Certificate:        tc.certificate(t, tc.svr.Certificate()),
					AutoPinCertificate: tc.autoPinCertificate,
				},
			})

			tc.assertErr(t, err)

			wantCert := ""
			if tc.wantCertificate != nil {
				wantCert = tc.wantCertificate(t, tc.svr.Certificate())
			}

			require.Equal(t, wantCert, cert)
		})
	}
}

func TestRedfish_GetData(t *testing.T) {
	tests := []struct {
		name      string
		responses mockRedfishServer

		assertErr require.ErrorAssertionFunc
		want      api.BMCData
	}{
		{
			name: "success",

			responses: mockRedfishServer{
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
			},

			assertErr: require.NoError,
			want: api.BMCData{
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

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusOK,
				systemsBody: `{
  "Members@odata.count": 0,
  "Members": []
}`,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual BMC system",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusOK,
				systemsBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" }
  ]
}`,
				systemStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC managers",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
		{
			name: "error - no BMC managers found",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual BMC manager",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get processors of BMC system",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
		{
			name: "error - no processors found for the BMC system",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get individual processor of BMC system",

			responses: mockRedfishServer{
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
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := newMockRedfishServer(t, tc.responses, nil)

			client := redfish.New()

			before := time.Now()
			details, err := client.GetData(t.Context(), provisioning.Server{
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

func TestRedfish_GetData_WithTrustedCertificate(t *testing.T) {
	responses := mockRedfishServer{
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
		processorStatusCode: http.StatusOK,
		processorBody: `{
  "@odata.id": "/redfish/v1/Systems/1/Processors/1",
  "Id": "1"
}`,
	}

	tests := []struct {
		name string

		certificate func(t *testing.T, serverCert *x509.Certificate) string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - SAN certificate pinned and matches presented certificate",
			certificate: func(_ *testing.T, serverCert *x509.Certificate) string {
				return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}))
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid PEM certificate",
			certificate: func(_ *testing.T, _ *x509.Certificate) string {
				return "not a valid certificate"
			},

			assertErr: errassert.Contains("Invalid remote certificate"),
		},
		{
			name: "error - certificate PEM decodes but is not a valid X.509 certificate",
			certificate: func(_ *testing.T, _ *x509.Certificate) string {
				return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-a-real-der-certificate")}))
			},

			assertErr: errassert.Contains("x509: malformed certificate"),
		},
		{
			name: "error - certificate without SAN pinned: fingerprint mismatch with presented certificate",
			certificate: func(t *testing.T, _ *x509.Certificate) string {
				t.Helper()

				// Generate separate certificate without any SAN.
				certPEMByte, _, err := incustls.GenerateMemCert(true, false)
				require.NoError(t, err)

				return string(certPEMByte)
			},

			assertErr: errassert.Contains("Certificate fingerprint mismatch"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := httptest.NewTLSServer(newMockRedfishHandler(responses, nil))
			defer svr.Close()

			client := redfish.New()

			_, err := client.GetData(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{
					Endpoint:    svr.URL,
					Certificate: tt.certificate(t, svr.Certificate()),
				},
			})

			tt.assertErr(t, err)
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
      "Target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset",
      "@Redfish.ActionInfo": "/redfish/v1/Systems/1/ResetActionInfo"
    }
  }
}`

	// resetActionInfoUnsupportedBody advertises an unused reset type, so the reset type check fails.
	resetActionInfoUnsupportedBody = `{
  "@odata.id": "/redfish/v1/Systems/1/ResetActionInfo",
  "Parameters": [
    {
      "Name": "ResetType",
      "DataType": "String",
      "AllowableValues": ["foobar"]
    }
  ]
}`

	resetEmptySystemsBody = `{
  "Members@odata.count": 0,
  "Members": []
}`
)

func TestRedfish_ServerPowerOn(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode     int
		systemsStatusCode         int
		systemsBody               string
		systemStatusCode          int
		systemBody                string
		resetActionInfoStatusCode int
		resetActionInfoBody       string
		resetStatusCode           int
		resetLocation             string

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
		{
			name: "error - failed to get reset action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusInternalServerError,

			assertErr: errassert.Contains("Failed to get BMC reset action info"),
		},
		{
			name: "error - reset type parameter missing from action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody: `{
  "@odata.id": "/redfish/v1/Systems/1/ResetActionInfo",
  "Parameters": []
}`,

			assertErr: errassert.Contains("Failed to get supported reset types from BMC"),
		},
		{
			name: "error - reset type not supported by BMC",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody:       resetActionInfoUnsupportedBody,

			assertErr: errassert.Contains("is not supported by the BMC"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode:     tc.serviceRootStatusCode,
				systemsStatusCode:         tc.systemsStatusCode,
				systemsBody:               tc.systemsBody,
				systemStatusCode:          tc.systemStatusCode,
				systemBody:                tc.systemBody,
				resetActionInfoStatusCode: tc.resetActionInfoStatusCode,
				resetActionInfoBody:       tc.resetActionInfoBody,
				resetStatusCode:           tc.resetStatusCode,
				resetLocation:             tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.ServerPowerOn(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantTaskMonitor, taskMonitor)

			if tc.wantResetType != "" {
				require.JSONEq(t, fmt.Sprintf(`{"ResetType":%q}`, tc.wantResetType), string(gotBody))
			}
		})
	}
}

func TestRedfish_ServerPowerOff(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode     int
		systemsStatusCode         int
		systemsBody               string
		systemStatusCode          int
		systemBody                string
		resetActionInfoStatusCode int
		resetActionInfoBody       string
		resetStatusCode           int
		resetLocation             string

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
		{
			name: "error - failed to get reset action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusInternalServerError,

			assertErr: errassert.Contains("Failed to get BMC reset action info"),
		},
		{
			name: "error - reset type parameter missing from action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody: `{
  "@odata.id": "/redfish/v1/Systems/1/ResetActionInfo",
  "Parameters": []
}`,

			assertErr: errassert.Contains("Failed to get supported reset types from BMC"),
		},
		{
			name: "error - reset type not supported by BMC",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody:       resetActionInfoUnsupportedBody,

			assertErr: errassert.Contains("is not supported by the BMC"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode:     tc.serviceRootStatusCode,
				systemsStatusCode:         tc.systemsStatusCode,
				systemsBody:               tc.systemsBody,
				systemStatusCode:          tc.systemStatusCode,
				systemBody:                tc.systemBody,
				resetActionInfoStatusCode: tc.resetActionInfoStatusCode,
				resetActionInfoBody:       tc.resetActionInfoBody,
				resetStatusCode:           tc.resetStatusCode,
				resetLocation:             tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.ServerPowerOff(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantTaskMonitor, taskMonitor)

			if tc.wantResetType != "" {
				require.JSONEq(t, fmt.Sprintf(`{"ResetType":%q}`, tc.wantResetType), string(gotBody))
			}
		})
	}
}

func TestRedfish_ServerRestart(t *testing.T) {
	tests := []struct {
		name  string
		force bool

		serviceRootStatusCode     int
		systemsStatusCode         int
		systemsBody               string
		systemStatusCode          int
		systemBody                string
		resetActionInfoStatusCode int
		resetActionInfoBody       string
		resetStatusCode           int
		resetLocation             string

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
		{
			name: "error - failed to get reset action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusInternalServerError,

			assertErr: errassert.Contains("Failed to get BMC reset action info"),
		},
		{
			name: "error - reset type parameter missing from action info",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody: `{
  "@odata.id": "/redfish/v1/Systems/1/ResetActionInfo",
  "Parameters": []
}`,

			assertErr: errassert.Contains("Failed to get supported reset types from BMC"),
		},
		{
			name: "error - reset type not supported by BMC",

			serviceRootStatusCode:     http.StatusOK,
			systemsStatusCode:         http.StatusOK,
			systemsBody:               resetSystemsBody,
			systemStatusCode:          http.StatusOK,
			systemBody:                resetSystemBody,
			resetActionInfoStatusCode: http.StatusOK,
			resetActionInfoBody:       resetActionInfoUnsupportedBody,

			assertErr: errassert.Contains("is not supported by the BMC"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte

			svr := newMockRedfishServer(t, mockRedfishServer{
				serviceRootStatusCode:     tc.serviceRootStatusCode,
				systemsStatusCode:         tc.systemsStatusCode,
				systemsBody:               tc.systemsBody,
				systemStatusCode:          tc.systemStatusCode,
				systemBody:                tc.systemBody,
				resetActionInfoStatusCode: tc.resetActionInfoStatusCode,
				resetActionInfoBody:       tc.resetActionInfoBody,
				resetStatusCode:           tc.resetStatusCode,
				resetLocation:             tc.resetLocation,
			}, &gotBody)

			client := redfish.New()
			taskMonitor, err := client.ServerRestart(t.Context(), provisioning.Server{BMCConfig: api.BMCConfig{Endpoint: svr.URL}}, tc.force)

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
