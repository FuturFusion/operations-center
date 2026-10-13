package incus_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func Test(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			t.Cleanup(server.Close)

			c := incus.New("", "", incus.WithSkipGetServer(true))
			cert, err := c.GetRemoteCertificate(t.Context(), provisioning.Server{
				ConnectionURL: server.URL,
			})
			require.NoError(t, err)

			require.NotEmpty(t, cert.Raw)
		})
	}
}
