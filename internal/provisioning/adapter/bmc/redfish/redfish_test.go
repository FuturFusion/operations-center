package redfish_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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

type redfishServerResponses struct {
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
}

func newRedfishTestHandler(tc redfishServerResponses) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

// newTLSServerWithoutSAN starts a TLS test server presenting a self-signed
// certificate without any Subject Alternative Names. Pinning such a
// certificate as the BMC's trusted certificate makes getClient fall back to
// fingerprint based verification instead of the SAN/RootCA based one.
func newTLSServerWithoutSAN(t *testing.T, responses redfishServerResponses) (svr *httptest.Server, certPEM string) {
	t.Helper()

	// Request a client-type certificate to get one without any SAN.
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tlsCert, err := tls.X509KeyPair(certPEMByte, keyPEMByte)
	require.NoError(t, err)

	svr = httptest.NewUnstartedServer(newRedfishTestHandler(responses))
	svr.TLS = &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	svr.StartTLS()

	require.Empty(t, svr.Certificate().DNSNames)
	require.Empty(t, svr.Certificate().IPAddresses)

	return svr, string(certPEMByte)
}

func TestRedfish_ConnectionTest(t *testing.T) {
	responses := redfishServerResponses{
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
			svr:                httptest.NewTLSServer(newRedfishTestHandler(responses)),
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
			svr:                httptest.NewTLSServer(newRedfishTestHandler(responses)),
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
			svr:                httptest.NewTLSServer(newRedfishTestHandler(responses)),
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
			svr:                httptest.NewServer(newRedfishTestHandler(responses)), // not https
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
		responses redfishServerResponses

		assertErr require.ErrorAssertionFunc
		want      api.BMCData
	}{
		{
			name: "success",

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
				serviceRootStatusCode: http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - failed to get BMC systems",

			responses: redfishServerResponses{
				serviceRootStatusCode: http.StatusOK,
				systemsStatusCode:     http.StatusInternalServerError,
			},

			assertErr: require.Error,
		},
		{
			name: "error - no BMC systems found",

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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

			responses: redfishServerResponses{
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
			svr := httptest.NewServer(newRedfishTestHandler(tc.responses))
			defer svr.Close()

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
	responses := redfishServerResponses{
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
			svr := httptest.NewTLSServer(newRedfishTestHandler(responses))
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
