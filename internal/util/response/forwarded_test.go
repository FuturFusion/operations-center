package response_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/response"
)

// recordingClient stands in for the client to another cluster member.
type recordingClient struct {
	ctxErr  error
	forward string
}

func (c *recordingClient) GetBaseAddr() string {
	return "https://192.0.2.1:8443"
}

func (c *recordingClient) DoHTTP(req *http.Request) (*http.Response, error) {
	c.ctxErr = req.Context().Err()
	c.forward = req.Header.Get("X-Test")

	if c.ctxErr != nil {
		return nil, c.ctxErr
	}

	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("forwarded"))}, nil
}

func TestForwardedResponse_context(t *testing.T) {
	t.Parallel()

	t.Run("the forwarded request is sent while the client waits", func(t *testing.T) {
		t.Parallel()

		c := &recordingClient{}
		r := httptest.NewRequest(http.MethodGet, "/1.0", http.NoBody)
		r.Header.Set("X-Test", "kept")

		rec := httptest.NewRecorder()
		require.NoError(t, response.ForwardedResponse(c, r).Render(rec))
		require.NoError(t, c.ctxErr)
		require.Equal(t, "kept", c.forward)
		require.Equal(t, "forwarded", rec.Body.String())
	})

	t.Run("the forwarded request ends with the request of the client", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		c := &recordingClient{}
		r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/1.0", http.NoBody)

		err := response.ForwardedResponse(c, r).Render(httptest.NewRecorder())
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, c.ctxErr, context.Canceled)
	})
}
