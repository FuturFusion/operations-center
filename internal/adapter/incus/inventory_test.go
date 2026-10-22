package incus_test

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

// TestClientInventory runs the generated inventory method test sets.
func TestClientInventory(t *testing.T) {
	t.Parallel()

	runMethodTestSets(t, appendTestCases(t, nil))
}

func TestClient_HasExtension(t *testing.T) {
	t.Parallel()

	caPool, certPEM, keyPEM := setupCerts(t)

	tests := []struct {
		name         string
		argExtension string
		response     response
		want         bool
	}{
		{
			name:         "success",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusOK,
				responseBody: []byte(`{
  "metadata": {
    "api_extensions": [
      "foobar"
    ]
  }
}`),
			},
			want: true,
		},
		{
			name:         "extension not found",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusOK,
				responseBody: []byte(`{
  "metadata": {
    "api_extensions": []
  }
}`),
			},
			want: false,
		},
		{
			name:         "error",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusInternalServerError,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.response.statusCode)
				_, _ = w.Write(tc.response.responseBody)
			}))
			server.TLS = &tls.Config{
				NextProtos: []string{"h2", "http/1.1"},
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  caPool,
			}

			server.StartTLS()
			defer server.Close()

			client := incus.New(certPEM, keyPEM)

			serverCert := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: server.Certificate().Raw,
			})

			target := newTestServer(server.URL, string(serverCert))

			// Run test
			got := client.HasExtension(t.Context(), target, tc.argExtension)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}

func TestClient_in_transaction(t *testing.T) {
	t.Parallel()

	// Setup
	caPool, certPEM, keyPEM := setupCerts(t)

	logBuf := &log.SyncBuffer{}
	ctx := logger.ContextWithTestLogger(t, logBuf, false, false, false)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(
			[]byte(`{
  "metadata": {
    "api_extensions": [
      "foobar"
    ]
  }
}`),
		)
	}))
	server.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}

	server.StartTLS()
	defer server.Close()

	client := incus.New(certPEM, keyPEM)

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	target := newTestServer(server.URL, string(serverCert))

	// Run test
	err := transaction.Do(ctx, func(ctx context.Context) error {
		_ = client.HasExtension(ctx, target, "foobar")
		return nil
	})
	require.NoError(t, err)

	// Assert
	log.Contains("Incus API call inside of a transaction")(t, logBuf)
}
