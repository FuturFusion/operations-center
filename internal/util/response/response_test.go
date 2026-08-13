package response_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/response"
)

func TestReadCloserResponse(t *testing.T) {
	const content = "file-content"

	tests := []struct {
		name           string
		method         string
		acceptEncoding string
		compress       bool
		fileSize       int

		wantBody              string
		wantContentType       string
		wantContentEncoding   string
		wantContentLength     string
		wantContentDisposiion string
	}{
		{
			name:     "uncompressed with known size",
			method:   http.MethodGet,
			compress: false,
			fileSize: len(content),

			wantBody:              content,
			wantContentType:       "application/octet-stream",
			wantContentLength:     "12",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:     "uncompressed with unknown size",
			method:   http.MethodGet,
			compress: false,
			fileSize: -1,

			wantBody:              content,
			wantContentType:       "application/octet-stream",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:           "compressed for a client accepting gzip",
			method:         http.MethodGet,
			acceptEncoding: "gzip",
			compress:       true,
			fileSize:       -1,

			wantContentType:       "application/octet-stream",
			wantContentEncoding:   "gzip",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:     "handed out as gzip file to a client not accepting gzip",
			method:   http.MethodGet,
			compress: true,
			fileSize: -1,

			wantContentType:       "application/gzip",
			wantContentDisposiion: `attachment; filename="image.iso.gz"`,
		},
		{
			name:     "head reports the metadata without a body",
			method:   http.MethodHead,
			compress: false,
			fileSize: len(content),

			wantContentType:       "application/octet-stream",
			wantContentLength:     "12",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/image.iso", nil)
			if tc.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}

			rc := io.NopCloser(strings.NewReader(content))
			resp := response.ReadCloserResponse(req, rc, tc.compress, "image.iso", tc.fileSize, nil)

			recorder := httptest.NewRecorder()

			err := resp.Render(recorder)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.Code())
			require.Equal(t, tc.wantContentType, recorder.Header().Get("Content-Type"))
			require.Equal(t, tc.wantContentEncoding, recorder.Header().Get("Content-Encoding"))
			require.Equal(t, tc.wantContentLength, recorder.Header().Get("Content-Length"))
			require.Equal(t, tc.wantContentDisposiion, recorder.Header().Get("Content-Disposition"))

			// The compressed variants are only checked for their headers, the
			// exact gzip encoding of the body is of no interest here.
			if !tc.compress {
				require.Equal(t, tc.wantBody, recorder.Body.String())
			}
		})
	}
}
