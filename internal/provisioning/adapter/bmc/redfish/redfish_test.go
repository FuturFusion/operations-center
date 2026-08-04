package redfish_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" },
  "Bios": { "@odata.id": "/redfish/v1/Systems/1/Bios" },
  "VirtualMedia": { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia" }
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
				biosStatusCode: http.StatusOK,
				biosBody: `{
  "@odata.id": "/redfish/v1/Systems/1/Bios",
  "Id": "Bios",
  "Attributes": {
    "BootMode": "Uefi",
    "NumLock": true
  }
}`,
				systemVirtualMediaStatusCode: http.StatusOK,
				systemVirtualMediaBody: `{
  "Members@odata.count": 2,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/1" },
    { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/2" }
  ]
}`,
				systemVirtualMediaMemberStatusCode: http.StatusOK,
				systemVirtualMediaMemberBody: `{
  "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/1",
  "Id": "1",
  "Inserted": true,
  "Image": "http://example.com/image.iso",
  "ImageName": "image.iso",
  "ConnectedVia": "URI",
  "Status": { "Health": "OK" },
  "MediaTypes": ["CD", "DVD"],
  "TransferMethod": "Stream",
  "TransferProtocolType": "HTTPS",
  "WriteProtected": true
}`,
				systemVirtualMediaMember2StatusCode: http.StatusOK,
				systemVirtualMediaMember2Body: `{
  "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/2",
  "Id": "2",
  "Inserted": false,
  "ConnectedVia": "NotConnected",
  "Status": { "Health": "OK" },
  "MediaTypes": ["USBStick"]
}`,
			},

			assertErr: require.NoError,
			want: api.BMCData{
				BMCProtocol:              "Redfish",
				BMCProtocolVersion:       "1.16.0",
				BMCVendor:                "Dell",
				BMCModel:                 "iDRAC9",
				BMCFirmwareVersion:       "1.30.20.10",
				BMCServiceIdentification: "ServiceID1",
				ServerManufacturer:       "Dell Inc.",
				ServerModel:              "PowerEdge R770",
				ServerSubModel:           "SubModel",
				ServerUUID:               "e9de436e-b94e-4aef-8563-883aec84096e",
				ServerAssetTag:           "AssetTag1",
				ServerHostName:           "host1",
				ServerSKU:                "SKU123",
				ServerSerialNumber:       "Serial123",
				ServerBIOSVersion:        "1.7.5",
				ServerBIOSAttributes: map[string]any{
					"BootMode": "Uefi",
					"NumLock":  true,
				},
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerPowerState:              "On",
				ServerLocationIndicatorActive: true,
				ServerHealthStatus:            "OK",
				VirtualMedia: []api.BMCVirtualMedia{
					{
						ID:                   "system:1",
						Inserted:             true,
						Image:                "http://example.com/image.iso",
						ImageName:            "image.iso",
						ConnectedVia:         "URI",
						Status:               "OK",
						MediaTypes:           []string{"CD", "DVD"},
						TransferMethod:       "Stream",
						TransferProtocolType: "HTTPS",
						WriteProtected:       true,
					},
					{
						ID:           "system:2",
						Inserted:     false,
						ConnectedVia: "NotConnected",
						Status:       "OK",
						MediaTypes:   []string{"USBStick"},
					},
				},
			},
		},
		{
			name: "success - virtual media only on manager",

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
  "Id": "1",
  "VirtualMedia": { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia" }
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
				managerVirtualMediaStatusCode: http.StatusOK,
				managerVirtualMediaBody: `{
  "Members@odata.count": 2,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/1" },
    { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/2" }
  ]
}`,
				managerVirtualMediaMemberStatusCode: http.StatusOK,
				managerVirtualMediaMemberBody: `{
  "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/1",
  "Id": "1",
  "Inserted": false,
  "ConnectedVia": "NotConnected",
  "Status": { "Health": "OK" }
}`,
				managerVirtualMediaMember2StatusCode: http.StatusOK,
				managerVirtualMediaMember2Body: `{
  "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/2",
  "Id": "2",
  "Inserted": true,
  "Image": "http://example.com/image2.iso",
  "ImageName": "image2.iso",
  "ConnectedVia": "URI",
  "Status": { "Health": "OK" },
  "MediaTypes": ["CD"]
}`,
			},

			assertErr: require.NoError,
			want: api.BMCData{
				BMCProtocol:        "Redfish",
				BMCProtocolVersion: "1.16.0",
				BMCVendor:          "Dell",
				VirtualMedia: []api.BMCVirtualMedia{
					{
						ID:           "manager:1",
						Inserted:     false,
						ConnectedVia: "NotConnected",
						Status:       "OK",
						MediaTypes:   []string{},
					},
					{
						ID:           "manager:2",
						Inserted:     true,
						Image:        "http://example.com/image2.iso",
						ImageName:    "image2.iso",
						ConnectedVia: "URI",
						Status:       "OK",
						MediaTypes:   []string{"CD"},
					},
				},
			},
		},
		{
			name: "success - virtual media on both system and manager returns a combined collection",

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
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" },
  "VirtualMedia": { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia" }
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
  "VirtualMedia": { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia" }
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
				systemVirtualMediaStatusCode: http.StatusOK,
				systemVirtualMediaBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/1" }
  ]
}`,
				systemVirtualMediaMemberStatusCode: http.StatusOK,
				systemVirtualMediaMemberBody: `{
  "@odata.id": "/redfish/v1/Systems/1/VirtualMedia/1",
  "Id": "1",
  "Inserted": true,
  "ConnectedVia": "URI",
  "Status": { "Health": "OK" },
  "MediaTypes": ["CD"]
}`,
				managerVirtualMediaStatusCode: http.StatusOK,
				managerVirtualMediaBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/1" }
  ]
}`,
				managerVirtualMediaMemberStatusCode: http.StatusOK,
				managerVirtualMediaMemberBody: `{
  "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/1",
  "Id": "1",
  "Inserted": false,
  "ConnectedVia": "NotConnected",
  "Status": { "Health": "OK" }
}`,
			},

			assertErr: require.NoError,
			want: api.BMCData{
				BMCProtocol:        "Redfish",
				BMCProtocolVersion: "1.16.0",
				BMCVendor:          "Dell",
				VirtualMedia: []api.BMCVirtualMedia{
					{
						ID:           "manager:1",
						Inserted:     false,
						ConnectedVia: "NotConnected",
						Status:       "OK",
						MediaTypes:   []string{},
					},
					{
						ID:           "system:1",
						Inserted:     true,
						ConnectedVia: "URI",
						Status:       "OK",
						MediaTypes:   []string{"CD"},
					},
				},
			},
		},
		{
			name: "success - no BIOS or virtual media links present",

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
				processorStatusCode: http.StatusOK,
				processorBody: `{
  "@odata.id": "/redfish/v1/Systems/1/Processors/1",
  "Id": "1"
}`,
			},

			assertErr: require.NoError,
			want: api.BMCData{
				BMCProtocol:        "Redfish",
				BMCProtocolVersion: "1.16.0",
				BMCVendor:          "Dell",
			},
		},
		{
			name: "error - failed to get BIOS of BMC system",

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
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" },
  "Bios": { "@odata.id": "/redfish/v1/Systems/1/Bios" }
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
				biosStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get virtual media of BMC system",

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
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" },
  "VirtualMedia": { "@odata.id": "/redfish/v1/Systems/1/VirtualMedia" }
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
				systemVirtualMediaStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get virtual media of BMC manager",

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
  "Id": "1",
  "VirtualMedia": { "@odata.id": "/redfish/v1/Managers/1/VirtualMedia" }
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
				managerVirtualMediaStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
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

const (
	logEmptyCollectionBody = `{
  "Members@odata.count": 0,
  "Members": []
}`

	logChassisCollectionBody = `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Chassis/1" }
  ]
}`

	logChassisMemberBody = `{
  "@odata.id": "/redfish/v1/Chassis/1",
  "Id": "1",
  "LogServices": { "@odata.id": "/redfish/v1/Chassis/1/LogServices" }
}`

	logSystemsCollectionBody = `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" }
  ]
}`

	logSystemMemberBody = `{
  "@odata.id": "/redfish/v1/Systems/1",
  "Id": "1",
  "LogServices": { "@odata.id": "/redfish/v1/Systems/1/LogServices" }
}`

	logManagersCollectionBody = `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Managers/1" }
  ]
}`

	logManagerMemberBody = `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1",
  "LogServices": { "@odata.id": "/redfish/v1/Managers/1/LogServices" }
}`

	logServicesBody = `{
  "Members@odata.count": 2,
  "Members": [
    {
      "@odata.id": "/redfish/v1/LogServices/Other",
      "Id": "Other",
      "Entries": { "@odata.id": "/redfish/v1/OtherEntries" }
    },
    {
      "@odata.id": "/redfish/v1/LogServices/Logs",
      "Id": "Logs",
      "Entries": { "@odata.id": "/redfish/v1/LogEntries" }
    }
  ]
}`

	logEntriesBody = `{
  "Members@odata.count": 4,
  "Members": [
    {
      "@odata.id": "/redfish/v1/LogEntries/4",
      "Id": "4",
      "EntryType": "Event",
      "Message": "Fourth log message, no timestamp at all",
      "Severity": "OK"
    },
    {
      "@odata.id": "/redfish/v1/LogEntries/1",
      "Id": "1",
      "EntryCode": "Assert",
      "EntryType": "SEL",
      "Message": "First log message",
      "Severity": "OK",
      "EventTimestamp": "2026-07-30T08:04:00Z"
    },
    {
      "@odata.id": "/redfish/v1/LogEntries/3",
      "Id": "3",
      "EntryType": "Event",
      "Message": "Third log message, EventTimestamp missing, fallback to Created",
      "Severity": "Warning",
      "Created": "2026-07-30T07:00:00Z"
    },
    {
      "@odata.id": "/redfish/v1/LogEntries/2",
      "Id": "2",
      "EntryType": "Event",
      "Message": "Second log message, with both timestamps, Created is respected",
      "Severity": "Critical",
      "Created": "2026-07-30T09:00:00Z",
      "EventTimestamp": "2026-07-30T06:00:00Z"
    }
  ]
}`
)

func TestRedfish_LogEntriesBySource(t *testing.T) {
	wantLogEntries := []api.BMCLogEvent{
		{
			EntryType: "Event",
			Message:   "Second log message, with both timestamps, Created is respected",
			Severity:  "Critical",
			Timestamp: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
		},
		{
			EntryCode: "Assert",
			EntryType: "SEL",
			Message:   "First log message",
			Severity:  "OK",
			Timestamp: time.Date(2026, 7, 30, 8, 4, 0, 0, time.UTC),
		},
		{
			EntryType: "Event",
			Message:   "Third log message, EventTimestamp missing, fallback to Created",
			Severity:  "Warning",
			Timestamp: time.Date(2026, 7, 30, 7, 0, 0, 0, time.UTC),
		},
		{
			EntryType: "Event",
			Message:   "Fourth log message, no timestamp at all",
			Severity:  "OK",
			Timestamp: time.Time{},
		},
	}

	tests := []struct {
		name      string
		logSource string
		responses mockRedfishServer

		assertErr require.ErrorAssertionFunc
		want      []api.BMCLogEvent
	}{
		{
			name:      "success - chassis log source",
			logSource: "chassis/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				logServicesStatusCode:   http.StatusOK,
				logServicesBody:         logServicesBody,
				logEntriesStatusCode:    http.StatusOK,
				logEntriesBody:          logEntriesBody,
			},

			assertErr: require.NoError,
			want:      wantLogEntries,
		},
		{
			name:      "success - system log source",
			logSource: "system/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusOK,
				systemsBody:           logSystemsCollectionBody,
				systemStatusCode:      http.StatusOK,
				systemBody:            logSystemMemberBody,
				logServicesStatusCode: http.StatusOK,
				logServicesBody:       logServicesBody,
				logEntriesStatusCode:  http.StatusOK,
				logEntriesBody:        logEntriesBody,
			},

			assertErr: require.NoError,
			want:      wantLogEntries,
		},
		{
			name:      "success - manager log source",
			logSource: "manager/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				managersStatusCode:    http.StatusOK,
				managersBody:          logManagersCollectionBody,
				managerStatusCode:     http.StatusOK,
				managerBody:           logManagerMemberBody,
				logServicesStatusCode: http.StatusOK,
				logServicesBody:       logServicesBody,
				logEntriesStatusCode:  http.StatusOK,
				logEntriesBody:        logEntriesBody,
			},

			assertErr: require.NoError,
			want:      wantLogEntries,
		},
		{
			name:      "error - invalid log source format",
			logSource: "invalid",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
			},

			assertErr: errassert.Contains("Invalid log source"),
		},
		{
			name:      "error - unknown log source service",
			logSource: "foobar/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
			},

			assertErr: errassert.Contains("Invalid log source service"),
		},
		{
			name:      "error - failed to connect to BMC",
			logSource: "chassis/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to connect to BMC"),
		},
		{
			name:      "error - failed to get BMC chassis",
			logSource: "chassis/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				chassisStatusCode:     http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get BMC chassis"),
		},
		{
			name:      "error - failed to get BMC system",
			logSource: "system/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get BMC system"),
		},
		{
			name:      "error - failed to get BMC manager",
			logSource: "manager/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				managersStatusCode:    http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get BMC manager"),
		},
		{
			name:      "error - failed to get log services",
			logSource: "chassis/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				logServicesStatusCode:   http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get log services"),
		},
		{
			name:      "error - log type not found",
			logSource: "chassis/DoesNotExist",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				logServicesStatusCode:   http.StatusOK,
				logServicesBody:         logServicesBody,
			},

			assertErr: errassert.Contains("Failed to find log type"),
		},
		{
			name:      "error - failed to get log entries",
			logSource: "chassis/Logs",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				logServicesStatusCode:   http.StatusOK,
				logServicesBody:         logServicesBody,
				logEntriesStatusCode:    http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get log entries"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := newMockRedfishServer(t, tc.responses, nil)

			client := redfish.New()

			events, err := client.LogEntriesBySource(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{Endpoint: svr.URL},
			}, tc.logSource)

			tc.assertErr(t, err)

			require.Equal(t, tc.want, events)
		})
	}
}

func TestRedfish_LogSources(t *testing.T) {
	tests := []struct {
		name      string
		responses mockRedfishServer

		assertErr require.ErrorAssertionFunc
		want      []string
	}{
		{
			name: "success - log sources from chassis, system and manager",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				systemsStatusCode:       http.StatusOK,
				systemsBody:             logSystemsCollectionBody,
				systemStatusCode:        http.StatusOK,
				systemBody:              logSystemMemberBody,
				managersStatusCode:      http.StatusOK,
				managersBody:            logManagersCollectionBody,
				managerStatusCode:       http.StatusOK,
				managerBody:             logManagerMemberBody,
				logServicesStatusCode:   http.StatusOK,
				logServicesBody:         logServicesBody,
			},

			assertErr: require.NoError,
			want: []string{
				"chassis/Logs",
				"chassis/Other",
				"manager/Logs",
				"manager/Other",
				"system/Logs",
				"system/Other",
			},
		},
		{
			name: "error - failed to connect to BMC",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to connect to BMC"),
		},
		{
			name: "success - not found entity is skipped",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				chassisStatusCode:     http.StatusOK,
				chassisBody:           logEmptyCollectionBody,
				systemsStatusCode:     http.StatusOK,
				systemsBody:           logSystemsCollectionBody,
				systemStatusCode:      http.StatusOK,
				systemBody:            logSystemMemberBody,
				managersStatusCode:    http.StatusOK,
				managersBody:          logManagersCollectionBody,
				managerStatusCode:     http.StatusOK,
				managerBody:           logManagerMemberBody,
				logServicesStatusCode: http.StatusOK,
				logServicesBody:       logServicesBody,
			},

			assertErr: require.NoError,
			want: []string{
				"manager/Logs",
				"manager/Other",
				"system/Logs",
				"system/Other",
			},
		},
		{
			name: "success - no entities",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				chassisStatusCode:     http.StatusOK,
				chassisBody:           logEmptyCollectionBody,
				systemsStatusCode:     http.StatusOK,
				systemsBody:           logEmptyCollectionBody,
				managersStatusCode:    http.StatusOK,
				managersBody:          logEmptyCollectionBody,
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name: "error - failed to get BMC chassis",

			responses: mockRedfishServer{
				serviceRootStatusCode: http.StatusOK,
				chassisStatusCode:     http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get BMC chassis"),
		},
		{
			name: "error - failed to get log services",

			responses: mockRedfishServer{
				serviceRootStatusCode:   http.StatusOK,
				chassisStatusCode:       http.StatusOK,
				chassisBody:             logChassisCollectionBody,
				chassisMemberStatusCode: http.StatusOK,
				chassisMemberBody:       logChassisMemberBody,
				systemsStatusCode:       http.StatusOK,
				systemsBody:             logSystemsCollectionBody,
				systemStatusCode:        http.StatusOK,
				systemBody:              logSystemMemberBody,
				managersStatusCode:      http.StatusOK,
				managersBody:            logManagersCollectionBody,
				managerStatusCode:       http.StatusOK,
				managerBody:             logManagerMemberBody,
				logServicesStatusCode:   http.StatusInternalServerError,
			},

			assertErr: errassert.Contains("Failed to get log services"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := newMockRedfishServer(t, tc.responses, nil)

			client := redfish.New()

			logSources, err := client.LogSources(t.Context(), provisioning.Server{
				BMCConfig: api.BMCConfig{Endpoint: svr.URL},
			})

			tc.assertErr(t, err)

			require.Equal(t, tc.want, logSources)
		})
	}
}

func TestRedfish_Dump(t *testing.T) {
	responses := mockRedfishServer{
		serviceRootStatusCode: http.StatusOK,

		systemsStatusCode: http.StatusOK,
		systemsBody: `{
  "Members@odata.count": 2,
  "Members": [
    { "@odata.id": "/redfish/v1/Systems/1" },
    { "@odata.id": "/redfish/v1/Systems/2" }
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
		managerStatusCode: http.StatusOK,
		managerBody: `{
  "@odata.id": "/redfish/v1/Managers/1",
  "Id": "1"
}`,

		chassisStatusCode: http.StatusOK,
		chassisBody: `{
  "Members@odata.count": 1,
  "Members": [
    { "@odata.id": "/redfish/v1/Chassis/1" }
  ]
}`,
		chassisMemberStatusCode: http.StatusOK,
		chassisMemberBody: `{
  "@odata.id": "/redfish/v1/Chassis/1",
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

		logServicesStatusCode: http.StatusOK,
		logServicesBody: `{
  "Members@odata.count": 0,
  "Members": []
}`,

		extraRoutes: map[string]mockRedfishRoute{
			"/redfish/v1/Systems/1/Bios": {
				statusCode: http.StatusNotFound,
				body: `{
		  "error": {
		    "code": "Base.1.0.GeneralError",
		    "message": "Resource not found"
		  }
		}`,
			},
			"/redfish/v1/Systems/1/BootOptions": {
				statusCode: http.StatusOK,
				body: `{
		  "Members@odata.count": 0,
		  "Members": []
		}`,
			},
			"/redfish/v1/Systems/1/Oem/Vendor": {
				statusCode: http.StatusOK,
				body: `{
		  "@odata.id": "/redfish/v1/Systems/1/Oem/Vendor"
		}`,
			},
		},
	}

	svr := newMockRedfishServer(t, responses, nil)

	client := redfish.New()

	server := provisioning.Server{
		BMCConfig: api.BMCConfig{
			Endpoint: svr.URL,
			Username: "admin",
			Password: "admin",
		},
	}

	t.Run("collections only fetch their first member", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, nil, false, false)
		require.NoError(t, err)

		require.Contains(t, dump, "/redfish/v1/Systems/1")
		require.NotContains(t, dump, "/redfish/v1/Systems/2")
	})

	t.Run("failing endpoints are recorded as errors without stopping the dump", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, nil, false, false)
		require.NoError(t, err)

		// Unmapped endpoint falls through to the mock's default 404 handler.
		odata, ok := dump["/redfish/v1/odata"]
		require.True(t, ok)
		require.Nil(t, odata.Response)
		require.NotNil(t, odata.Error)
		require.Equal(t, http.StatusNotFound, odata.Error.StatusCode)

		// Explicit Redfish error body.
		bios, ok := dump["/redfish/v1/Systems/1/Bios"]
		require.True(t, ok)
		require.Nil(t, bios.Response)
		require.NotNil(t, bios.Error)
		require.Equal(t, http.StatusNotFound, bios.Error.StatusCode)
		require.Equal(t, "Base.1.0.GeneralError", bios.Error.Code)

		// Sibling endpoints still succeed.
		serviceRoot, ok := dump["/redfish/v1/"]
		require.True(t, ok)
		require.NotNil(t, serviceRoot.Response)
		require.Nil(t, serviceRoot.Error)
	})

	t.Run("empty collections do not yield a member entry", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, nil, false, false)
		require.NoError(t, err)

		require.Contains(t, dump, "/redfish/v1/Systems/1/BootOptions")

		for uri := range dump {
			require.False(t, strings.HasPrefix(uri, "/redfish/v1/Systems/1/BootOptions/"), "unexpected member fetched for empty collection: %s", uri)
		}
	})

	t.Run("trace is empty unless requested", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, nil, false, false)
		require.NoError(t, err)

		require.Empty(t, dump["/redfish/v1/"].Trace)
	})

	t.Run("trace contains only redacted headers", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, nil, false, true)
		require.NoError(t, err)

		trace := dump["/redfish/v1/"].Trace
		require.NotEmpty(t, trace)
		require.Contains(t, trace, "GET /redfish/v1/ HTTP/1.1")
		require.Contains(t, trace, "HTTP/1.1 200 OK")
		require.Contains(t, trace, "Authorization: <redacted>")
		require.NotContains(t, trace, "RedfishVersion")
		require.NotContains(t, trace, base64.StdEncoding.EncodeToString([]byte("admin:admin")))
	})

	t.Run("additional endpoints are dumped alongside the predefined set", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, []string{"/redfish/v1/Systems/1/Oem/Vendor", "/redfish/v1/Systems/1/Bios"}, false, false)
		require.NoError(t, err)

		vendor, ok := dump["/redfish/v1/Systems/1/Oem/Vendor"]
		require.True(t, ok)
		require.NotNil(t, vendor.Response)
		require.Nil(t, vendor.Error)

		// An additional endpoint that duplicates a predefined one still
		// yields exactly one dump entry.
		require.Contains(t, dump, "/redfish/v1/Systems/1/Bios")
	})

	t.Run("skip predefined dumps only additional endpoints", func(t *testing.T) {
		dump, err := client.Dump(t.Context(), server, []string{"/redfish/v1/Systems/1/Oem/Vendor"}, true, false)
		require.NoError(t, err)

		require.Len(t, dump, 1)

		vendor, ok := dump["/redfish/v1/Systems/1/Oem/Vendor"]
		require.True(t, ok)
		require.NotNil(t, vendor.Response)
		require.Nil(t, vendor.Error)
	})
}
