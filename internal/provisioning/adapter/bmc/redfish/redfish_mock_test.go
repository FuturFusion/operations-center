package redfish_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockRedfishServer struct {
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
	resetStatusCode       int
	resetLocation         string

	taskMonitorStatusCodes []int
	taskMonitorRetryAfter  string
}

func newMockRedfishServer(t *testing.T, cfg mockRedfishServer, gotBody *[]byte) *httptest.Server {
	t.Helper()

	taskMonitorCallCount := 0

	svr := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
  "Managers": { "@odata.id": "/redfish/v1/Managers" }
}`))
				}

			case "/redfish/v1/Systems":
				w.WriteHeader(cfg.systemsStatusCode)
				_, _ = w.Write([]byte(cfg.systemsBody))

			case "/redfish/v1/Systems/1":
				w.WriteHeader(cfg.systemStatusCode)
				_, _ = w.Write([]byte(cfg.systemBody))

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

			case "/redfish/v1/Systems/1/Processors":
				w.WriteHeader(cfg.processorsStatusCode)
				_, _ = w.Write([]byte(cfg.processorsBody))

			case "/redfish/v1/Systems/1/Processors/1":
				w.WriteHeader(cfg.processorStatusCode)
				_, _ = w.Write([]byte(cfg.processorBody))

			case "/redfish/v1/TaskMonitor/1":
				statusCode := cfg.taskMonitorStatusCodes[min(taskMonitorCallCount, len(cfg.taskMonitorStatusCodes)-1)]
				taskMonitorCallCount++

				if cfg.taskMonitorRetryAfter != "" {
					w.Header().Set("Retry-After", cfg.taskMonitorRetryAfter)
				}

				w.WriteHeader(statusCode)

			default:
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			}
		}),
	)
	t.Cleanup(svr.Close)

	return svr
}
