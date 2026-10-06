package response_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/response"
)

func TestWith_abortsStartedResponse(t *testing.T) {
	// The content is large enough, for the response to be on the wire before the failure.
	content := io.MultiReader(strings.NewReader(strings.Repeat("x", 1<<20)), iotest.ErrReader(errors.New("boom")))

	server := httptest.NewServer(response.With(func(r *http.Request) response.Response {
		return response.ReadCloserResponse(r, io.NopCloser(content), false, "file", -1, nil)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The client must not take the truncated content for a complete response.
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
}
