package response_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/response"
)

func TestReadCloserResponse(t *testing.T) {
	const content = "file-content"

	tests := []struct {
		name           string
		method         string
		accept         string
		acceptEncoding string
		compress       bool
		fileSize       int

		wantBody              string
		wantGzipBody          bool
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

			wantGzipBody:          true,
			wantContentType:       "application/octet-stream",
			wantContentEncoding:   "gzip",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:           "compressed for a client accepting gzip, size known",
			method:         http.MethodGet,
			acceptEncoding: "gzip",
			compress:       true,
			fileSize:       len(content),

			wantGzipBody:          true,
			wantContentType:       "application/octet-stream",
			wantContentEncoding:   "gzip",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:     "uncompressed for a client asking for neither",
			method:   http.MethodGet,
			compress: true,
			fileSize: len(content),

			wantBody:              content,
			wantContentType:       "application/octet-stream",
			wantContentLength:     "12",
			wantContentDisposiion: `attachment; filename="image.iso"`,
		},
		{
			name:     "handed out as gzip file to a client asking for it",
			method:   http.MethodGet,
			accept:   "application/gzip",
			compress: true,
			fileSize: len(content),

			wantGzipBody:          true,
			wantContentType:       "application/gzip",
			wantContentDisposiion: `attachment; filename="image.iso.gz"`,
		},
		{
			name:           "gzip file wins over gzip transfer encoding",
			method:         http.MethodGet,
			accept:         "application/gzip",
			acceptEncoding: "gzip",
			compress:       true,
			fileSize:       len(content),

			wantGzipBody:          true,
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
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}

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

			if tc.wantGzipBody {
				gzr, err := gzip.NewReader(recorder.Body)
				require.NoError(t, err)

				decompressed, err := io.ReadAll(gzr)
				require.NoError(t, err)
				require.Equal(t, content, string(decompressed))

				return
			}

			require.Equal(t, tc.wantBody, recorder.Body.String())
		})
	}
}

// countingReadSeeker records how far its underlying reader was actually
// repositioned, so a test can tell whether a seek reached it at all.
type countingReadSeeker struct {
	io.ReadSeeker

	seeks int
}

func (c *countingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	c.seeks++

	return c.ReadSeeker.Seek(offset, whence)
}

func (c *countingReadSeeker) Close() error { return nil }

func TestServeContentResponse_sizeProbeDoesNotSeek(t *testing.T) {
	const content = "0123456789"

	rs := &countingReadSeeker{ReadSeeker: strings.NewReader(content)}

	req := httptest.NewRequest(http.MethodHead, "/image", http.NoBody)
	w := httptest.NewRecorder()

	resp := response.ServeContentResponse(req, rs, "image.iso", time.Now(), int64(len(content)), nil)
	require.NoError(t, resp.Render(w))

	result := w.Result()
	defer result.Body.Close()

	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, strconv.Itoa(len(content)), result.Header.Get("Content-Length"))
	require.Equal(t, "bytes", result.Header.Get("Accept-Ranges"))

	// http.ServeContent determines the size by seeking to the end and back.
	// Answering that from the known size matters, because for a reader over a
	// compressed file every seek costs decompression.
	require.Zero(t, rs.seeks, "size probe must not reach the underlying reader")
}

func TestServeContentResponse_closesContent(t *testing.T) {
	closed := false

	rs := &closeRecordingReadSeeker{ReadSeeker: strings.NewReader("data"), closed: &closed}

	req := httptest.NewRequest(http.MethodGet, "/image", http.NoBody)
	w := httptest.NewRecorder()

	resp := response.ServeContentResponse(req, rs, "image.iso", time.Now(), 4, nil)
	require.NoError(t, resp.Render(w))

	require.True(t, closed)
}

type closeRecordingReadSeeker struct {
	io.ReadSeeker

	closed *bool
}

func (c closeRecordingReadSeeker) Close() error {
	*c.closed = true
	return nil
}
