package redfish_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"
)

type mockRedfishServer struct {
	serviceRootStatusCode     int
	systemsStatusCode         int
	systemsBody               string
	systemStatusCode          int
	systemBody                string
	managersStatusCode        int
	managersBody              string
	managerStatusCode         int
	managerBody               string
	processorsStatusCode      int
	processorsBody            string
	processorStatusCode       int
	processorBody             string
	resetActionInfoStatusCode int
	resetActionInfoBody       string
	resetStatusCode           int
	resetLocation             string

	systemVirtualMediaStatusCode        int
	systemVirtualMediaBody              string
	systemVirtualMediaMemberStatusCode  int
	systemVirtualMediaMemberBody        string
	systemVirtualMediaMember2StatusCode int
	systemVirtualMediaMember2Body       string

	managerVirtualMediaStatusCode        int
	managerVirtualMediaBody              string
	managerVirtualMediaMemberStatusCode  int
	managerVirtualMediaMemberBody        string
	managerVirtualMediaMember2StatusCode int
	managerVirtualMediaMember2Body       string

	insertMediaStatusCode int
	insertMediaLocation   string
	ejectMediaStatusCode  int
	ejectMediaLocation    string

	chassisStatusCode       int
	chassisBody             string
	chassisMemberStatusCode int
	chassisMemberBody       string
	logServicesStatusCode   int
	logServicesBody         string
	logEntriesStatusCode    int
	logEntriesBody          string

	taskMonitorStatusCodes []int
	taskMonitorRetryAfter  string

	biosStatusCode               int
	biosBody                     string
	biosPatchStatusCode          int
	biosPatchBody                string
	biosPatchTaskMonitorLocation string

	gotBiosPatchBody *[]byte

	systemPatchStatusCode int

	gotSystemPatchBody *[]byte

	// extraRoutes allows tests to serve additional canned responses for paths
	// not covered by the dedicated fields above, keyed by the exact request
	// path.
	extraRoutes map[string]mockRedfishRoute
}

type mockRedfishRoute struct {
	statusCode int
	body       string
	header     map[string]string
}

const defaultResetActionInfoBody = `{
  "@odata.id": "/redfish/v1/Systems/1/ResetActionInfo",
  "Parameters": [
    {
      "Name": "ResetType",
      "DataType": "String",
      "AllowableValues": ["On", "ForceOn", "ForceOff", "GracefulShutdown", "GracefulRestart", "ForceRestart"]
    }
  ]
}`

func newMockRedfishServer(t *testing.T, cfg mockRedfishServer, gotBody *[]byte) *httptest.Server {
	t.Helper()

	svr := httptest.NewServer(newMockRedfishHandler(cfg, gotBody))

	t.Cleanup(svr.Close)

	return svr
}

func newMockRedfishHandler(cfg mockRedfishServer, gotBody *[]byte) http.HandlerFunc {
	// Apply some defaults, if not explicitly set.
	if cfg.resetActionInfoStatusCode == 0 {
		cfg.resetActionInfoStatusCode = http.StatusOK
		cfg.resetActionInfoBody = defaultResetActionInfoBody
	}

	taskMonitorCallCount := 0

	return func(w http.ResponseWriter, r *http.Request) {
		route, ok := cfg.extraRoutes[r.URL.Path]
		if ok {
			for name, value := range route.header {
				w.Header().Set(name, value)
			}

			w.WriteHeader(route.statusCode)
			_, _ = w.Write([]byte(route.body))

			return
		}

		switch r.URL.Path {
		case "/redfish/v1/":
			w.WriteHeader(cfg.serviceRootStatusCode)

			if cfg.serviceRootStatusCode == http.StatusOK {
				_, _ = w.Write([]byte(`{
  "Id": "RootService",
  "Name": "Root Service",
  "RedfishVersion": "1.16.0",
  "Vendor": "Dell",
  "Systems": { "@odata.id": "/redfish/v1/Systems" },
  "Managers": { "@odata.id": "/redfish/v1/Managers" },
  "Chassis": { "@odata.id": "/redfish/v1/Chassis" },
  "Registries": { "@odata.id": "/redfish/v1/Registries" }
}`))
			}

		case "/redfish/v1/Systems":
			w.WriteHeader(cfg.systemsStatusCode)
			_, _ = w.Write([]byte(cfg.systemsBody))

		case "/redfish/v1/Systems/1":
			handleSystem(w, r, cfg)

		case "/redfish/v1/Systems/1/ResetActionInfo":
			statusCode := cfg.resetActionInfoStatusCode
			body := cfg.resetActionInfoBody

			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(body))

		case "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset":
			if gotBody != nil {
				*gotBody, _ = io.ReadAll(r.Body)
			}

			if cfg.resetLocation != "" {
				w.Header().Set("Location", cfg.resetLocation)
			}

			w.WriteHeader(cfg.resetStatusCode)

		case "/redfish/v1/Managers":
			w.WriteHeader(cfg.managersStatusCode)
			_, _ = w.Write([]byte(cfg.managersBody))

		case "/redfish/v1/Managers/1":
			w.WriteHeader(cfg.managerStatusCode)
			_, _ = w.Write([]byte(cfg.managerBody))

		case "/redfish/v1/Systems/1/VirtualMedia":
			w.WriteHeader(cfg.systemVirtualMediaStatusCode)
			_, _ = w.Write([]byte(cfg.systemVirtualMediaBody))

		case "/redfish/v1/Systems/1/VirtualMedia/1":
			w.WriteHeader(cfg.systemVirtualMediaMemberStatusCode)
			_, _ = w.Write([]byte(cfg.systemVirtualMediaMemberBody))

		case "/redfish/v1/Systems/1/VirtualMedia/2":
			w.WriteHeader(cfg.systemVirtualMediaMember2StatusCode)
			_, _ = w.Write([]byte(cfg.systemVirtualMediaMember2Body))

		case "/redfish/v1/Managers/1/VirtualMedia":
			w.WriteHeader(cfg.managerVirtualMediaStatusCode)
			_, _ = w.Write([]byte(cfg.managerVirtualMediaBody))

		case "/redfish/v1/Managers/1/VirtualMedia/1":
			w.WriteHeader(cfg.managerVirtualMediaMemberStatusCode)
			_, _ = w.Write([]byte(cfg.managerVirtualMediaMemberBody))

		case "/redfish/v1/Managers/1/VirtualMedia/2":
			w.WriteHeader(cfg.managerVirtualMediaMember2StatusCode)
			_, _ = w.Write([]byte(cfg.managerVirtualMediaMember2Body))

		case "/redfish/v1/Systems/1/VirtualMedia/1/Actions/VirtualMedia.InsertMedia",
			"/redfish/v1/Systems/1/VirtualMedia/2/Actions/VirtualMedia.InsertMedia",
			"/redfish/v1/Managers/1/VirtualMedia/1/Actions/VirtualMedia.InsertMedia",
			"/redfish/v1/Managers/1/VirtualMedia/2/Actions/VirtualMedia.InsertMedia":
			if gotBody != nil {
				*gotBody, _ = io.ReadAll(r.Body)
			}

			if cfg.insertMediaLocation != "" {
				w.Header().Set("Location", cfg.insertMediaLocation)
			}

			w.WriteHeader(cfg.insertMediaStatusCode)

		case "/redfish/v1/Systems/1/VirtualMedia/1/Actions/VirtualMedia.EjectMedia",
			"/redfish/v1/Systems/1/VirtualMedia/2/Actions/VirtualMedia.EjectMedia",
			"/redfish/v1/Managers/1/VirtualMedia/1/Actions/VirtualMedia.EjectMedia",
			"/redfish/v1/Managers/1/VirtualMedia/2/Actions/VirtualMedia.EjectMedia":
			if gotBody != nil {
				*gotBody, _ = io.ReadAll(r.Body)
			}

			if cfg.ejectMediaLocation != "" {
				w.Header().Set("Location", cfg.ejectMediaLocation)
			}

			w.WriteHeader(cfg.ejectMediaStatusCode)

		case "/redfish/v1/Systems/1/Processors":
			w.WriteHeader(cfg.processorsStatusCode)
			_, _ = w.Write([]byte(cfg.processorsBody))

		case "/redfish/v1/Systems/1/Processors/1":
			w.WriteHeader(cfg.processorStatusCode)
			_, _ = w.Write([]byte(cfg.processorBody))

		case "/redfish/v1/Chassis":
			w.WriteHeader(cfg.chassisStatusCode)
			_, _ = w.Write([]byte(cfg.chassisBody))

		case "/redfish/v1/Chassis/1":
			w.WriteHeader(cfg.chassisMemberStatusCode)
			_, _ = w.Write([]byte(cfg.chassisMemberBody))

		case "/redfish/v1/Chassis/1/LogServices",
			"/redfish/v1/Systems/1/LogServices",
			"/redfish/v1/Managers/1/LogServices":
			w.WriteHeader(cfg.logServicesStatusCode)
			_, _ = w.Write([]byte(cfg.logServicesBody))

		case "/redfish/v1/LogEntries":
			w.WriteHeader(cfg.logEntriesStatusCode)
			_, _ = w.Write([]byte(cfg.logEntriesBody))

		case "/redfish/v1/TaskMonitor/1":
			statusCode := cfg.taskMonitorStatusCodes[min(taskMonitorCallCount, len(cfg.taskMonitorStatusCodes)-1)]
			taskMonitorCallCount++

			if cfg.taskMonitorRetryAfter != "" {
				w.Header().Set("Retry-After", cfg.taskMonitorRetryAfter)
			}

			w.WriteHeader(statusCode)

		case "/redfish/v1/Systems/1/Bios":
			handleBios(w, r, cfg)

		default:
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		}
	}
}

// newTLSServerWithoutSAN starts a TLS test server presenting a self-signed
// certificate without any Subject Alternative Names. Pinning such a
// certificate as the BMC's trusted certificate makes getClient fall back to
// fingerprint based verification instead of the SAN/RootCA based one.
func newTLSServerWithoutSAN(t *testing.T, responses mockRedfishServer) (svr *httptest.Server, certPEM string) {
	t.Helper()

	// Request a client-type certificate to get one without any SAN.
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tlsCert, err := tls.X509KeyPair(certPEMByte, keyPEMByte)
	require.NoError(t, err)

	svr = httptest.NewUnstartedServer(newMockRedfishHandler(responses, nil))
	svr.TLS = &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	svr.StartTLS()

	require.Empty(t, svr.Certificate().DNSNames)
	require.Empty(t, svr.Certificate().IPAddresses)

	return svr, string(certPEMByte)
}

func handleSystem(w http.ResponseWriter, r *http.Request, cfg mockRedfishServer) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(cfg.systemStatusCode)
		_, _ = w.Write([]byte(cfg.systemBody))

	case http.MethodPatch:
		body, _ := io.ReadAll(r.Body)

		if cfg.gotSystemPatchBody != nil {
			*cfg.gotSystemPatchBody = body
		}

		w.WriteHeader(cfg.systemPatchStatusCode)

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleBios(w http.ResponseWriter, r *http.Request, cfg mockRedfishServer) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(cfg.biosStatusCode)
		_, _ = w.Write([]byte(cfg.biosBody))

	case http.MethodPatch:
		body, _ := io.ReadAll(r.Body)

		if cfg.gotBiosPatchBody != nil {
			*cfg.gotBiosPatchBody = body
		}

		if cfg.biosPatchTaskMonitorLocation != "" {
			w.Header().Set("Location", cfg.biosPatchTaskMonitorLocation)
		}

		w.WriteHeader(cfg.biosPatchStatusCode)

		if cfg.biosPatchBody != "" {
			_, _ = w.Write([]byte(cfg.biosPatchBody))
		}

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}
