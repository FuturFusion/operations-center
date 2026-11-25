package client_test

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/dump"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestNew_serverCertificateVerification(t *testing.T) {
	serverCert, serverX509Cert := generateServerCert(t)
	_, otherX509Cert := generateServerCert(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{"auth":"untrusted"}}`))
	}))

	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	srv.StartTLS()
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		opts []client.Option

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - certificate of the server is pinned",
			opts: []client.Option{client.WithTrustedServerCertificate(serverX509Cert)},

			assertErr: require.NoError,
		},
		{
			name: "error - no certificate pinned, self-signed certificate is not trusted",
			opts: nil,

			assertErr: require.Error,
		},
		{
			name: "error - pinned certificate does not match the one presented by the server",
			opts: []client.Option{client.WithTrustedServerCertificate(otherX509Cert)},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := client.New(srv.URL, tc.opts...)
			require.NoError(t, err)

			_, err = c.DoRequest(t.Context(), http.MethodGet, "", url.Values{}, nil)
			tc.assertErr(t, err)
		})
	}
}

func TestIsServerTrusted(t *testing.T) {
	serverCert, serverX509Cert := generateServerCert(t)
	_, otherX509Cert := generateServerCert(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{"auth":"untrusted"}}`))
	}))

	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	srv.StartTLS()
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		opts       []client.Option
		pinnedCert api.Certificate

		wantTrusted    bool
		wantActualCert *x509.Certificate
	}{
		{
			name:        "trusted - certificate of the server is pinned",
			opts:        []client.Option{client.WithTrustedServerCertificate(serverX509Cert)},
			pinnedCert:  api.Certificate{Certificate: serverX509Cert},
			wantTrusted: true,
		},
		{
			name: "untrusted - no certificate pinned, self-signed certificate is not trusted",
			opts: nil,

			wantTrusted:    false,
			wantActualCert: serverX509Cert,
		},
		{
			name:       "untrusted - pinned certificate does not match the one presented by the server",
			opts:       []client.Option{client.WithTrustedServerCertificate(otherX509Cert)},
			pinnedCert: api.Certificate{Certificate: otherX509Cert},

			wantTrusted:    false,
			wantActualCert: serverX509Cert,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := client.New(srv.URL, tc.opts...)
			require.NoError(t, err)

			actualCert, trusted, err := c.IsServerTrusted(t.Context(), tc.pinnedCert)
			require.NoError(t, err)
			require.Equal(t, tc.wantTrusted, trusted)

			if tc.wantActualCert != nil {
				require.NotNil(t, actualCert.Certificate)
				require.True(t, tc.wantActualCert.Equal(actualCert.Certificate))
			} else {
				require.Nil(t, actualCert.Certificate)
			}
		})
	}
}

func Test_GetAPIServerInfo(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name   string
		client client.OperationsCenterClient

		wantAuth string
	}{
		{
			name:   "success - authenticated through the unix socket",
			client: d.socketClient,

			wantAuth: api.AuthenticationMethodUnix,
		},
		{
			name:   "success - authenticated with TLS client certificate",
			client: d.authorizedHTTPClient,

			wantAuth: api.AuthenticationMethodTLS,
		},
		{
			name:   "success - not authenticated, signals available authentication methods, route is exempt from authentication",
			client: d.unauthorizedHTTPClient,

			wantAuth: api.AuthenticationUntrusted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serverInfo, err := tc.client.GetAPIServerInfo(t.Context())
			require.NoError(t, err)

			require.Equal(t, tc.wantAuth, serverInfo.Auth)
			require.Equal(t, api.APIStatus, serverInfo.APIStatus)
			require.Equal(t, api.APIVersion, serverInfo.APIVersion)
			require.Equal(t, []string{api.AuthenticationMethodOIDC, api.AuthenticationMethodTLS}, serverInfo.AuthMethods)
			require.Equal(t, version.Version, serverInfo.ServerVersion)
		})
	}
}

func Test_DoInternalRequest(t *testing.T) {
	d := daemonSetup(t)

	query := dump.SQLQuery{Query: "SELECT 1"}

	tests := []struct {
		name string
		do   func(ctx context.Context) (*api.Response, error)

		assertErr  require.ErrorAssertionFunc
		assertResp func(t *testing.T, resp *api.Response)
	}{
		{
			name: "success - internal API",
			do: func(ctx context.Context) (*api.Response, error) {
				return d.authorizedHTTPClient.DoInternalRequest(ctx, http.MethodPost, "/sql", nil, query)
			},

			assertErr: require.NoError,
			assertResp: func(t *testing.T, resp *api.Response) {
				t.Helper()

				require.JSONEq(t, `{"Results":[{"type":"select","columns":["1"],"rows":[[1]],"rows_affected":0}]}`, string(resp.Metadata), "result of the SQL query")
			},
		},
		{
			name: "error - not authenticated",
			do: func(ctx context.Context) (*api.Response, error) {
				return d.unauthorizedHTTPClient.DoInternalRequest(ctx, http.MethodPost, "/sql", nil, query)
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated, a...)
			},
			assertResp: func(t *testing.T, resp *api.Response) {
				t.Helper()

				require.Equal(t, http.StatusUnauthorized, resp.Code, "error code of the response")
			},
		},
		{
			name: "error - internal API is not below the versioned API",
			do: func(ctx context.Context) (*api.Response, error) {
				return d.authorizedHTTPClient.DoRequest(ctx, http.MethodPost, "/internal/sql", nil, query)
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound, a...)
			},
			assertResp: func(t *testing.T, resp *api.Response) {
				t.Helper()

				require.Nil(t, resp, "the plain text response of the router is not decoded")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.do(t.Context())

			tc.assertErr(t, err)
			tc.assertResp(t, resp)
		})
	}
}

func TestWithTrustedCertificate_doesNotModifyProvidedCertificate(t *testing.T) {
	_, serverX509Cert := generateServerCert(t)

	require.False(t, serverX509Cert.IsCA)

	_, err := client.New("https://localhost:8443", client.WithTrustedServerCertificate(serverX509Cert))
	require.NoError(t, err)

	// The certificate provided by the caller is not turned into a CA, only the
	// copy held by the TLS config is.
	require.False(t, serverX509Cert.IsCA)
}

func generateServerCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	return cert, x509Cert
}

func TestOperationsCenterClient_DoHTTP(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string

		wantStatusCode int
	}{
		{
			name:        "success - POST without body and without Content-Type gets the default Content-Type",
			method:      http.MethodPost,
			path:        "/1.0/provisioning/servers/unknown/system/:reboot",
			contentType: "",
			body:        "",

			wantStatusCode: http.StatusNotFound,
		},
		{
			name:        "success - POST with body and without Content-Type gets the default Content-Type",
			method:      http.MethodPost,
			path:        "/internal/sql",
			contentType: "",
			body:        `{"query": "SELECT 1"}`,

			wantStatusCode: http.StatusOK,
		},
		{
			name:        "error - POST with Content-Type keeps the Content-Type of the caller",
			method:      http.MethodPost,
			path:        "/internal/sql",
			contentType: "text/plain",
			body:        `{"query": "SELECT 1"}`,

			wantStatusCode: http.StatusUnsupportedMediaType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			req, err := http.NewRequestWithContext(t.Context(), tc.method, d.authorizedHTTPClient.GetBaseAddr()+tc.path, strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tc.contentType)

			// Run test
			resp, err := d.authorizedHTTPClient.DoHTTP(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Assert
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatusCode, resp.StatusCode, "status code of the response, body: %s", string(body))
		})
	}
}

func TestOperationsCenterClient_compressedResponse(t *testing.T) {
	// The size of the response in bytes, from which on the server compresses it.
	const compressionMinSize = 1024

	d := daemonSetup(t)

	serverNames := []string{"compressedOne", "compressedTwo", "compressedThree"}
	for _, name := range serverNames {
		seedServer(t, d, name, nil)
	}

	tests := []struct {
		name         string
		setupRequest func(req *http.Request)
		readBody     func(t *testing.T, r io.Reader) []byte

		wantContentEncoding string
		wantUncompressed    bool
	}{
		{
			name: "explicit gzip leaves the body compressed for the caller",
			setupRequest: func(req *http.Request) {
				req.Header.Set("Accept-Encoding", "gzip")
			},
			readBody: readGzipBody,

			wantContentEncoding: "gzip",
			wantUncompressed:    false,
		},
		{
			name:         "transport decompresses the body",
			setupRequest: func(*http.Request) {},
			readBody:     readPlainBody,

			wantContentEncoding: "",
			wantUncompressed:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.authorizedHTTPClient.GetBaseAddr()+"/1.0/provisioning/servers?recursion=1", http.NoBody)
			require.NoError(t, err)

			tc.setupRequest(req)

			// Run test
			resp, err := d.authorizedHTTPClient.DoHTTP(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body := tc.readBody(t, resp.Body)

			// Assert
			require.Equal(t, http.StatusOK, resp.StatusCode, "status code of the response")
			require.Equal(t, tc.wantContentEncoding, resp.Header.Get("Content-Encoding"), "content encoding seen by the caller")
			require.Equal(t, tc.wantUncompressed, resp.Uncompressed, "transport decompressed the response")
			require.GreaterOrEqual(t, len(body), compressionMinSize, "response is large enough to be compressed")

			var apiResponse api.Response
			require.NoError(t, json.Unmarshal(body, &apiResponse), "decode response")

			var servers []api.Server
			require.NoError(t, json.Unmarshal(apiResponse.Metadata, &servers), "decode servers")
			require.ElementsMatch(t, serverNames, serverNamesOf(servers), "names of the seeded servers")
		})
	}

	t.Run("client method decompresses the body", func(t *testing.T) {
		// Run test
		servers, err := d.authorizedHTTPClient.GetServers(t.Context())

		// Assert
		require.NoError(t, err)
		require.ElementsMatch(t, serverNames, serverNamesOf(servers), "names of the seeded servers")
	})
}

func TestOperationsCenterClient_proxyResponseNotCompressed(t *testing.T) {
	d := daemonSetup(t)
	seedServer(t, d, "proxyServer", nil)

	tests := []struct {
		name string
		path string

		wantContentEncoding string
		wantVary            string
	}{
		{
			name: "IncusOS proxy",
			path: "/os/1.0",

			wantContentEncoding: "",
			wantVary:            "",
		},
		{
			name: "server OS proxy",
			path: "/1.0/provisioning/servers/proxyServer/os/1.0",

			wantContentEncoding: "",
			wantVary:            "",
		},
		{
			name: "API outside of the proxies",
			path: "/1.0/provisioning/servers",

			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.authorizedHTTPClient.GetBaseAddr()+tc.path, http.NoBody)
			require.NoError(t, err)
			req.Header.Set("Accept-Encoding", "gzip")

			// Run test
			resp, err := d.authorizedHTTPClient.DoHTTP(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Assert
			require.Equal(t, tc.wantContentEncoding, resp.Header.Get("Content-Encoding"), "content encoding of the response")
			require.Equal(t, tc.wantVary, resp.Header.Get("Vary"), "vary header of the response")
		})
	}
}

func readGzipBody(t *testing.T, r io.Reader) []byte {
	t.Helper()

	gzipReader, err := gzip.NewReader(r)
	require.NoError(t, err, "gzip reader for the response body")

	body, err := io.ReadAll(gzipReader)
	require.NoError(t, err, "read response body")
	require.NoError(t, gzipReader.Close(), "close gzip reader")

	return body
}

func readPlainBody(t *testing.T, r io.Reader) []byte {
	t.Helper()

	body, err := io.ReadAll(r)
	require.NoError(t, err, "read response body")

	return body
}

func serverNamesOf(servers []api.Server) []string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names
}
