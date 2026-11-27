package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompressionMiddleware(t *testing.T) {
	t.Parallel()

	const (
		apiPath = "/1.0/provisioning/servers"
		etag    = `"etag-value"`
	)

	largeJSON := jsonOfSize(2 * compressionMinSize)
	smallJSON := `{"data":"a"}`
	precompressedJSON := gzipNoCompression(t, largeJSON)
	require.Greater(t, len(precompressedJSON), compressionMinSize, "precompressed body is large enough to be compressed again")

	jsonHeader := http.Header{"Content-Type": {"application/json"}}

	tests := []struct {
		name           string
		path           string
		acceptEncoding string
		responseHeader http.Header
		status         int
		body           string
		etag           string

		wantStatus          int
		wantContentEncoding string
		wantVary            string
		wantETag            string
		decode              func(t *testing.T, r io.Reader) string
		wantBody            string
	}{
		{
			name:           "large JSON with gzip accepted is compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            largeJSON,
		},
		{
			name:           "large JSON with gzip and zstd accepted is compressed with gzip",
			path:           apiPath,
			acceptEncoding: "gzip, deflate, br, zstd",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            largeJSON,
		},
		{
			name:           "large JSON without gzip accepted is not compressed",
			path:           apiPath,
			acceptEncoding: "",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
		{
			name:           "large JSON with gzip rejected by quality value is not compressed",
			path:           apiPath,
			acceptEncoding: "gzip;q=0",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
		{
			name:           "large JSON with charset parameter is compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: http.Header{"Content-Type": {"application/json; charset=utf-8"}},
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            largeJSON,
		},
		{
			name:           "small JSON is not compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           smallJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            smallJSON,
		},
		{
			name:           "JSON of exactly the minimum size is compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           jsonOfSize(compressionMinSize),
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            jsonOfSize(compressionMinSize),
		},
		{
			name:           "JSON one byte below the minimum size is not compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           jsonOfSize(compressionMinSize - 1),
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            jsonOfSize(compressionMinSize - 1),
		},
		{
			name:           "large JSON error response keeps its status and is compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusNotFound,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusNotFound,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            largeJSON,
		},
		{
			name:           "small JSON created response keeps its status",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusCreated,
			body:           smallJSON,
			etag:           etag,

			wantStatus:          http.StatusCreated,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            smallJSON,
		},
		{
			name:           "large binary content is not compressed",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: http.Header{"Content-Type": {"application/octet-stream"}},
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
		{
			name:           "content compressed by the handler is not compressed again",
			path:           apiPath,
			acceptEncoding: "gzip",
			responseHeader: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}},
			status:         http.StatusOK,
			body:           precompressedJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "gzip",
			wantVary:            "Accept-Encoding",
			wantETag:            etag,
			decode:              gunzipBody,
			wantBody:            largeJSON,
		},
		{
			name:           "IncusOS proxy is not compressed",
			path:           "/os/1.0/system/resources",
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
		{
			name:           "server OS proxy is not compressed",
			path:           "/1.0/provisioning/servers/server01/os/1.0/system/resources",
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
		{
			name:           "server OS proxy without trailing slash is not compressed",
			path:           "/1.0/provisioning/servers/server01/os",
			acceptEncoding: "gzip",
			responseHeader: jsonHeader,
			status:         http.StatusOK,
			body:           largeJSON,
			etag:           etag,

			wantStatus:          http.StatusOK,
			wantContentEncoding: "",
			wantVary:            "",
			wantETag:            etag,
			decode:              plainBody,
			wantBody:            largeJSON,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			handler, err := compressionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				maps.Copy(w.Header(), tc.responseHeader)
				w.Header().Set("ETag", tc.etag)
				w.WriteHeader(tc.status)

				_, err := io.WriteString(w, tc.body)
				assert.NoError(t, err, "write response body")
			}))
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			rec := httptest.NewRecorder()

			// Run test
			handler.ServeHTTP(rec, req)

			// Assert
			resp := rec.Result()
			defer resp.Body.Close()

			require.Equal(t, tc.wantStatus, resp.StatusCode, "status code of the response")
			require.Equal(t, tc.wantContentEncoding, resp.Header.Get("Content-Encoding"), "content encoding of the response")
			require.Equal(t, tc.wantVary, resp.Header.Get("Vary"), "vary header of the response")
			require.Equal(t, tc.wantETag, resp.Header.Get("ETag"), "etag header of the response")
			require.Equal(t, tc.wantBody, tc.decode(t, resp.Body), "body after decoding")
		})
	}
}

func jsonOfSize(size int) string {
	const prefix, suffix = `{"data":"`, `"}`

	return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
}

// gzipNoCompression wraps the data in gzip without compressing it. This keeps
// the result larger than the minimum size, so a second compression is visible.
func gzipNoCompression(t *testing.T, data string) string {
	t.Helper()

	var buf bytes.Buffer

	gzipWriter, err := gzip.NewWriterLevel(&buf, gzip.NoCompression)
	require.NoError(t, err, "create gzip writer")

	_, err = io.WriteString(gzipWriter, data)
	require.NoError(t, err, "write gzip data")
	require.NoError(t, gzipWriter.Close(), "close gzip writer")

	return buf.String()
}

func gunzipBody(t *testing.T, r io.Reader) string {
	t.Helper()

	gzipReader, err := gzip.NewReader(r)
	require.NoError(t, err, "gzip reader for the response body")

	data, err := io.ReadAll(gzipReader)
	require.NoError(t, err, "read response body")
	require.NoError(t, gzipReader.Close(), "close gzip reader")

	return string(data)
}

func plainBody(t *testing.T, r io.Reader) string {
	t.Helper()

	data, err := io.ReadAll(r)
	require.NoError(t, err, "read response body")

	return string(data)
}

func TestIsProxyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string

		want bool
	}{
		{name: "IncusOS proxy", path: "/os/1.0", want: true},
		{name: "IncusOS proxy root", path: "/os/", want: true},
		{name: "IncusOS path without trailing slash", path: "/os", want: false},
		{name: "server OS proxy", path: "/1.0/provisioning/servers/server01/os/1.0", want: true},
		{name: "server OS proxy root", path: "/1.0/provisioning/servers/server01/os/", want: true},
		{name: "server OS proxy without trailing slash", path: "/1.0/provisioning/servers/server01/os", want: true},
		{name: "server list", path: "/1.0/provisioning/servers", want: false},
		{name: "server", path: "/1.0/provisioning/servers/server01", want: false},
		{name: "server named os", path: "/1.0/provisioning/servers/os", want: false},
		{name: "server sub resource", path: "/1.0/provisioning/servers/server01/system/network", want: false},
		{name: "server sub resource starting with os", path: "/1.0/provisioning/servers/server01/osx", want: false},
		{name: "path starting with os", path: "/osx", want: false},
		{name: "API root", path: "/1.0", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isProxyPath(tc.path)

			require.Equal(t, tc.want, got, "path %q", tc.path)
		})
	}
}
