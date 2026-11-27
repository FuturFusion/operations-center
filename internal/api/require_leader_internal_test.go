package api

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequireLeader(t *testing.T) {
	t.Parallel()

	clientTLS := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{generateCert(t)}}
	clustered := func() bool { return true }
	standalone := func() bool { return false }

	const handledLocally = "handled by this member"

	tests := []struct {
		name      string
		clustered func() bool
		method    string
		path      string
		tlsState  *tls.ConnectionState

		wantStatus int
		wantBody   string
	}{
		{
			name:      "GET is handled by this member",
			clustered: clustered,
			method:    http.MethodGet,
			path:      "/1.0/provisioning/clusters",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "HEAD is handled by this member",
			clustered: clustered,
			method:    http.MethodHead,
			path:      "/1.0/provisioning/clusters",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "POST is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodPost,
			path:      "/1.0/provisioning/clusters",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "PUT is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/system/security",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "PATCH is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodPatch,
			path:      "/1.0/provisioning/servers/server01",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "DELETE is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodDelete,
			path:      "/1.0/provisioning/servers/server01",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "local network configuration is handled by this member",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/system/network",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "local settings are handled by this member",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/system/settings",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "clean cache is handled by this member",
			clustered: clustered,
			method:    http.MethodPost,
			path:      "/1.0/system/:clean-cache",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "enable clustering is handled by this member",
			clustered: clustered,
			method:    http.MethodPost,
			path:      "/1.0/membership/:enable",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "join is handled by this member",
			clustered: clustered,
			method:    http.MethodPost,
			path:      "/1.0/membership/:join",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "self registration is handled by this member",
			clustered: clustered,
			method:    http.MethodPost,
			path:      "/1.0/provisioning/servers/:self_register",
			tlsState:  nil,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "a path below a member local path is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/system/network/other",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "self update through the unix socket is handled by this member",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/provisioning/servers/:self",
			tlsState:  nil,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
		{
			name:      "self update through TLS is forwarded to the leader",
			clustered: clustered,
			method:    http.MethodPut,
			path:      "/1.0/provisioning/servers/:self",
			tlsState:  clientTLS,

			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "The cluster leader is not known",
		},
		{
			name:      "standalone server handles changes itself",
			clustered: standalone,
			method:    http.MethodPost,
			path:      "/1.0/provisioning/clusters",
			tlsState:  clientTLS,

			wantStatus: http.StatusOK,
			wantBody:   handledLocally,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			// An unknown leader makes a forwarded request fail with a known error, without contacting another member.
			forward := forwarder{
				clustered:  tc.clustered,
				membership: leaderStub{leaderErr: errors.New("No leader")},
			}

			next := func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(handledLocally))
			}

			r := httptest.NewRequest(tc.method, tc.path, http.NoBody)
			r.TLS = tc.tlsState
			rec := httptest.NewRecorder()

			// Run test
			requireLeader(forward)(next)(rec, r)

			// Assert
			require.Equal(t, tc.wantStatus, rec.Code, "the response status shows whether the request was forwarded")
			require.Contains(t, rec.Body.String(), tc.wantBody, "the response body shows who handled the request")
		})
	}
}
