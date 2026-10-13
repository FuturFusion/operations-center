package openfga

import (
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/stretchr/testify/require"
)

// Run starts an OpenFGA server inside the test process and returns the URL of
// its HTTP API. The server keeps its data in memory. The server stops when the
// test ends.
// The HTTP API does not map errors to HTTP statuses like a standalone OpenFGA
// server. Do not assert on error responses.
func Run(t *testing.T) string {
	t.Helper()

	fgaServer, err := server.NewServerWithOpts(server.WithDatastore(memory.New()))
	require.NoError(t, err, "Failed to create the OpenFGA server")

	// Close also closes the datastore.
	t.Cleanup(fgaServer.Close)

	mux := runtime.NewServeMux()

	err = openfgav1.RegisterOpenFGAServiceHandlerServer(t.Context(), mux, fgaServer)
	require.NoError(t, err, "Failed to register the HTTP API of the OpenFGA server")

	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	return httpServer.URL
}
