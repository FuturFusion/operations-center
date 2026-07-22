package redfish_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockRedfishServer struct {
	serviceRootStatusCode int
	systemsStatusCode     int
	systemsBody           string
	systemStatusCode      int
	systemBody            string
	resetStatusCode       int
	resetLocation         string

	taskMonitorStatusCodes []int
	taskMonitorRetryAfter  string

	biosStatusCode      int
	biosBody            string
	biosPatchStatusCode int

	secureBootStatusCode          int
	secureBootBody                string
	secureBootDatabasesStatusCode int
	secureBootDatabasesBody       string
	secureBootDatabases           map[string]mockSecureBootDatabase

	gotDeletedCertPaths *[]string
	gotPostedCerts      *map[string][]string
	gotBiosPatchBody    *[]byte
}

type mockSecureBootDatabase struct {
	statusCode int
	body       string

	postStatusCode int

	certificatesStatusCode int
	certificatesBody       string

	certificates map[string]mockCertificate
}

type mockCertificate struct {
	statusCode       int
	body             string
	deleteStatusCode int
}

const secureBootDatabasesPathPrefix = "/redfish/v1/Systems/1/SecureBoot/SecureBootDatabases/"

func newMockRedfishServer(t *testing.T, cfg mockRedfishServer, gotBody *[]byte) *httptest.Server {
	t.Helper()

	taskMonitorCallCount := 0

	svr := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/redfish/v1/":
				w.WriteHeader(cfg.serviceRootStatusCode)

				if cfg.serviceRootStatusCode == http.StatusOK {
					_, _ = w.Write([]byte(`{
  "Id": "RootService",
  "Name": "Root Service",
  "Systems": { "@odata.id": "/redfish/v1/Systems" }
}`))
				}

			case r.URL.Path == "/redfish/v1/Systems":
				w.WriteHeader(cfg.systemsStatusCode)
				_, _ = w.Write([]byte(cfg.systemsBody))

			case r.URL.Path == "/redfish/v1/Systems/1":
				w.WriteHeader(cfg.systemStatusCode)
				_, _ = w.Write([]byte(cfg.systemBody))

			case r.URL.Path == "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset":
				if gotBody != nil {
					*gotBody, _ = io.ReadAll(r.Body)
				}

				if cfg.resetLocation != "" {
					w.Header().Set("Location", cfg.resetLocation)
				}

				w.WriteHeader(cfg.resetStatusCode)

			case r.URL.Path == "/redfish/v1/TaskMonitor/1":
				statusCode := cfg.taskMonitorStatusCodes[min(taskMonitorCallCount, len(cfg.taskMonitorStatusCodes)-1)]
				taskMonitorCallCount++

				if cfg.taskMonitorRetryAfter != "" {
					w.Header().Set("Retry-After", cfg.taskMonitorRetryAfter)
				}

				w.WriteHeader(statusCode)

			case r.URL.Path == "/redfish/v1/Systems/1/Bios":
				handleBios(w, r, cfg)

			case r.URL.Path == "/redfish/v1/Systems/1/SecureBoot":
				w.WriteHeader(cfg.secureBootStatusCode)
				_, _ = w.Write([]byte(cfg.secureBootBody))

			case r.URL.Path == "/redfish/v1/Systems/1/SecureBoot/SecureBootDatabases":
				w.WriteHeader(cfg.secureBootDatabasesStatusCode)
				_, _ = w.Write([]byte(cfg.secureBootDatabasesBody))

			case strings.HasPrefix(r.URL.Path, secureBootDatabasesPathPrefix):
				handleSecureBootDatabasePath(w, r, cfg)

			default:
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			}
		}),
	)
	t.Cleanup(svr.Close)

	return svr
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

		w.WriteHeader(cfg.biosPatchStatusCode)

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleSecureBootDatabasePath(w http.ResponseWriter, r *http.Request, cfg mockRedfishServer) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, secureBootDatabasesPathPrefix), "/")

	db, ok := cfg.secureBootDatabases[parts[0]]
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	switch len(parts) {
	case 1:
		handleSecureBootDatabase(w, r, cfg, parts[0], db)

	case 2:
		w.WriteHeader(db.certificatesStatusCode)
		_, _ = w.Write([]byte(db.certificatesBody))

	case 3:
		handleSecureBootCertificate(w, r, cfg, db, parts[2])

	default:
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

func handleSecureBootDatabase(w http.ResponseWriter, r *http.Request, cfg mockRedfishServer, dbID string, db mockSecureBootDatabase) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(db.statusCode)
		_, _ = w.Write([]byte(db.body))

	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)

		if cfg.gotPostedCerts != nil {
			(*cfg.gotPostedCerts)[dbID] = append((*cfg.gotPostedCerts)[dbID], string(body))
		}

		w.WriteHeader(db.postStatusCode)

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleSecureBootCertificate(w http.ResponseWriter, r *http.Request, cfg mockRedfishServer, db mockSecureBootDatabase, certID string) {
	cert, ok := db.certificates[certID]
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(cert.statusCode)
		_, _ = w.Write([]byte(cert.body))

	case http.MethodDelete:
		if cfg.gotDeletedCertPaths != nil {
			*cfg.gotDeletedCertPaths = append(*cfg.gotDeletedCertPaths, r.URL.Path)
		}

		w.WriteHeader(cert.deleteStatusCode)

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}
