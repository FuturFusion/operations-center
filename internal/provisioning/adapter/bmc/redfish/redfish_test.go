package redfish_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/redfish/v1/":
						w.WriteHeader(tc.serviceRootStatusCode)

						if tc.serviceRootStatusCode == http.StatusOK {
							_, _ = w.Write([]byte(`{
  "Id": "RootService",
  "Name": "Root Service",
  "Systems": { "@odata.id": "/redfish/v1/Systems" }
}`))
						}

					case "/redfish/v1/Systems":
						w.WriteHeader(tc.systemsStatusCode)
						_, _ = w.Write([]byte(tc.systemsBody))

					case "/redfish/v1/Systems/1":
						w.WriteHeader(tc.systemStatusCode)
						_, _ = w.Write([]byte(tc.systemBody))

					default:
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					}
				}),
			)
			defer svr.Close()

			client := redfish.New()
			details, err := client.GetServerDetails(t.Context(), provisioning.Server{
				BMCEndpoint: svr.URL,
			})

			tc.assertErr(t, err)
			require.Equal(t, tc.want, details)
		})
	}
}
