package response

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

type HandlerFunc func(r *http.Request) Response

// Response represents an API response.
type Response interface {
	Render(w http.ResponseWriter) error
	String() string
	Code() int
}

// Sync response.
type syncResponse struct {
	success   bool
	etag      any
	metadata  any
	location  string
	code      int
	headers   map[string]string
	plaintext bool
	compress  bool
}

// EmptySyncResponse represents an empty syncResponse.
var EmptySyncResponse = &syncResponse{success: true, metadata: make(map[string]any)}

// SyncResponse returns a new syncResponse with the success and metadata fields
// set to the provided values.
func SyncResponse(success bool, metadata any) Response {
	return &syncResponse{success: success, metadata: metadata}
}

func SyncResponseETag(success bool, metadata any, etag any) Response {
	return &syncResponse{success: success, metadata: metadata, etag: etag}
}

// SyncResponseLocation returns a new syncResponse with a location.
func SyncResponseLocation(success bool, metadata any, location string) Response {
	return &syncResponse{success: success, metadata: metadata, location: location}
}

// SyncResponsePlain return a new syncResponse with plaintext.
func SyncResponsePlain(success bool, compress bool, metadata string) Response {
	return &syncResponse{success: success, metadata: metadata, plaintext: true, compress: compress}
}

func (r *syncResponse) Render(w http.ResponseWriter) error {
	// Set an appropriate ETag header
	if r.etag != nil {
		etag, err := EtagHash(r.etag)
		if err == nil {
			w.Header().Set("ETag", fmt.Sprintf("\"%s\"", etag))
		}
	}

	if r.headers != nil {
		for h, v := range r.headers {
			w.Header().Set(h, v)
		}
	}

	if r.location != "" {
		w.Header().Set("Location", r.location)
		if r.code == 0 {
			r.code = http.StatusCreated
		}
	}

	w.Header().Set("Content-Type", "application/json")
	// Handle plain text headers.
	if r.plaintext {
		w.Header().Set("Content-Type", "text/plain")
	}

	// Handle compression.
	if r.compress {
		w.Header().Set("Content-Encoding", "gzip")
	}

	// Write header and status code.
	if r.code == 0 {
		r.code = http.StatusOK
	}

	if w.Header().Get("Connection") != "keep-alive" {
		w.WriteHeader(r.code)
	}

	// Prepare the JSON response
	status := api.Success
	if !r.success {
		status = api.Failure

		// If the metadata is an error, consider the response a SmartError
		// to propagate the data and preserve the status code.
		err, ok := r.metadata.(error)
		if ok {
			return SmartError(err).Render(w)
		}
	}

	// Handle plain text responses.
	if r.plaintext {
		if r.metadata != nil {
			if r.compress {
				comp := gzip.NewWriter(w)
				defer comp.Close()

				_, err := comp.Write([]byte(r.metadata.(string)))
				if err != nil {
					return err
				}
			} else {
				_, err := w.Write([]byte(r.metadata.(string)))
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	// Handle JSON responses.
	resp := api.ResponseRaw{
		Type:       api.SyncResponse,
		Status:     status.String(),
		StatusCode: int(status),
		Metadata:   r.metadata,
	}

	return writeJSON(w, resp)
}

func (r *syncResponse) String() string {
	if r.success {
		return "success"
	}

	return "failure"
}

// Code returns the HTTP code.
func (r *syncResponse) Code() int {
	return r.code
}

type manualResponse struct {
	hook func(w http.ResponseWriter) error
}

// ManualResponse creates a new manual response responder.
func ManualResponse(hook func(w http.ResponseWriter) error) Response {
	return &manualResponse{hook: hook}
}

func (r *manualResponse) Render(w http.ResponseWriter) error {
	return r.hook(w)
}

func (r *manualResponse) String() string {
	return "unknown (manual response)"
}

// Code returns the HTTP code.
func (r *manualResponse) Code() int {
	return -1
}

// Error response.
type errorResponse struct {
	code int    // Code to return in both the HTTP header and Code field of the response body.
	msg  string // Message to return in the Error field of the response body.
}

// BadRequest returns a bad request response (400) with the given error.
func BadRequest(err error) Response {
	return errorResponseFromError(http.StatusBadRequest, err)
}

// Forbidden returns a forbidden response (403) with the given error.
func Forbidden(err error) Response {
	return errorResponseFromError(http.StatusForbidden, err)
}

// NotFound returns a not found response (404) with the given error.
func NotFound(err error) Response {
	return errorResponseFromError(http.StatusNotFound, err)
}

// PreconditionFailed returns a precondition failed response (412) with the
// given error.
func PreconditionFailed(err error) Response {
	return errorResponseFromError(http.StatusPreconditionFailed, err)
}

// InternalError returns an internal error response (500) with the given error.
func InternalError(err error) Response {
	return errorResponseFromError(http.StatusInternalServerError, err)
}

// NotImplemented returns a not implemented response (501) with the given error.
func NotImplemented(err error) Response {
	return errorResponseFromError(http.StatusNotImplemented, err)
}

// Unavailable return an unavailable response (503) with the given error.
func Unavailable(err error) Response {
	return errorResponseFromError(http.StatusServiceUnavailable, err)
}

func errorResponseFromError(status int, err error) Response {
	message := http.StatusText(status)
	if err != nil {
		message += ": " + err.Error()
	}

	return &errorResponse{status, message}
}

func (r *errorResponse) String() string {
	return r.msg
}

// Code returns the HTTP code.
func (r *errorResponse) Code() int {
	return r.code
}

func (r *errorResponse) Render(w http.ResponseWriter) error {
	var output io.Writer

	buf := &bytes.Buffer{}
	output = buf

	resp := api.ResponseRaw{
		Type:  api.ErrorResponse,
		Error: r.msg,
		Code:  r.code, // Set the error code in the Code field of the response body.
	}

	err := json.NewEncoder(output).Encode(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if w.Header().Get("Connection") != "keep-alive" {
		w.WriteHeader(r.code) // Set the error code in the HTTP header response.
	}

	_, err = fmt.Fprint(w, buf.String())

	return err
}

// writeJSON encodes the body as JSON and sends it back to the client.
func writeJSON(w http.ResponseWriter, body any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	err := enc.Encode(body)

	return err
}

// Unauthorized return an unauthorized response (401) with the given error.
func Unauthorized(err error) Response {
	message := "unauthorized"
	if err != nil {
		message = err.Error()
	}

	return &errorResponse{http.StatusUnauthorized, message}
}

type readCloserResponse struct {
	req      *http.Request
	rc       io.ReadCloser
	filename string
	fileSize int
	headers  map[string]string
	compress bool
}

// ReadCloserResponse returns a new file taking the file content from a io.ReadCloser.
// If the fileSize is unknown, -1 should be passed in order to omit the
// Content-Length HTTP header.
//
// If compress is set to true, the content may be gzip compressed, depending on
// what the client asks for:
//
//   - "Accept: application/gzip" requests the compressed file itself, which is
//     returned with the content type "application/gzip" and a ".gz" suffix
//     appended to the filename.
//   - "Accept-Encoding: gzip" only asks for a compressed transfer, which is
//     returned with "Content-Encoding: gzip" and the unmodified filename.
//   - Otherwise the content is returned uncompressed.
//
// Since the size of the compressed content is not known upfront, the
// Content-Length HTTP header is omitted whenever the content is compressed.
func ReadCloserResponse(r *http.Request, rc io.ReadCloser, compress bool, filename string, fileSize int, headers map[string]string) Response {
	return &readCloserResponse{
		req:      r,
		rc:       rc,
		filename: filename,
		fileSize: fileSize,
		headers:  headers,
		compress: compress,
	}
}

// AcceptsGzip reports whether the client offered to take the response gzip
// compressed.
func AcceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// RequestsGzipFile reports whether the client asked for the gzip compressed
// file itself, rather than only offering to take a compressed transfer.
func RequestsGzipFile(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/gzip")
}

func (r readCloserResponse) Render(w http.ResponseWriter) error {
	defer func() {
		_ = r.rc.Close()
	}()

	if r.headers != nil {
		for k, v := range r.headers {
			w.Header().Set(k, v)
		}
	}

	wantsGzipFile := r.compress && RequestsGzipFile(r.req)
	wantsGzipEncoding := r.compress && !wantsGzipFile && AcceptsGzip(r.req)

	fileName := r.filename
	contentType := "application/octet-stream"
	var writer io.Writer = w
	if wantsGzipFile || wantsGzipEncoding {
		gzWriter := gzip.NewWriter(w)
		defer gzWriter.Close()

		writer = gzWriter

		if wantsGzipFile {
			contentType = "application/gzip"
			if !strings.HasSuffix(fileName, ".gz") {
				fileName += ".gz"
			}
		} else {
			w.Header().Set("Content-Encoding", "gzip")
		}
	}

	// Only set Content-Type header if it is still set to the default or not yet set at all.
	if w.Header().Get("Content-Type") == "application/json" || w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", contentType)
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	if r.fileSize >= 0 && writer == w {
		w.Header().Set("Content-Length", strconv.Itoa(r.fileSize))
	}

	// A HEAD request asks for the metadata of the file only. Producing the body
	// is wasted effort, the server discards it.
	if r.req.Method == http.MethodHead {
		return nil
	}

	_, err := file.SafeCopy(writer, r.rc)
	if err != nil {
		return err
	}

	return nil
}

func (r readCloserResponse) String() string {
	return fmt.Sprintf("readCloser response for %q", r.filename)
}

func (r readCloserResponse) Code() int {
	return http.StatusOK
}

type serveContentResponse struct {
	req      *http.Request
	content  io.ReadSeekCloser
	filename string
	modTime  time.Time
	size     int64
	headers  map[string]string
}

// ServeContentResponse returns a response serving the given seekable content.
func ServeContentResponse(r *http.Request, content io.ReadSeekCloser, filename string, modTime time.Time, size int64, headers map[string]string) Response {
	return &serveContentResponse{
		req:      r,
		content:  content,
		filename: filename,
		modTime:  modTime,
		size:     size,
		headers:  headers,
	}
}

func (r serveContentResponse) Render(w http.ResponseWriter) error {
	defer func() {
		_ = r.content.Close()
	}()

	for k, v := range r.headers {
		w.Header().Set(k, v)
	}

	if w.Header().Get("Content-Type") == "application/json" || w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", r.filename))

	http.ServeContent(w, r.req, r.filename, r.modTime, newLazySeeker(r.content, r.size))

	return nil
}

func (r serveContentResponse) String() string {
	return fmt.Sprintf("serve content response for %q", r.filename)
}

// Code returns -1, since the status code is determined by http.ServeContent
// while the response is rendered and is not known before.
func (r serveContentResponse) Code() int {
	return -1
}

// lazySeeker adapts an io.ReadSeeker of known size for use with
// http.ServeContent.
//
// It answers seeks arithmetically and only repositions the underlying reader
// once the next read actually needs the data.
type lazySeeker struct {
	rs   io.ReadSeeker
	size int64

	// pos is the position reads are expected to continue at, readerPos the
	// position the underlying reader is actually at.
	pos       int64
	readerPos int64
}

func newLazySeeker(rs io.ReadSeeker, size int64) *lazySeeker {
	return &lazySeeker{
		rs:   rs,
		size: size,
	}
}

func (l *lazySeeker) Read(p []byte) (int, error) {
	if l.pos >= l.size {
		return 0, io.EOF
	}

	if l.pos != l.readerPos {
		pos, err := l.rs.Seek(l.pos, io.SeekStart)
		l.readerPos = pos
		if err != nil {
			return 0, err
		}
	}

	if int64(len(p)) > l.size-l.pos {
		p = p[:l.size-l.pos]
	}

	n, err := l.rs.Read(p)
	l.pos += int64(n)
	l.readerPos += int64(n)

	return n, err
}

func (l *lazySeeker) Seek(offset int64, whence int) (int64, error) {
	var abs int64

	switch whence {
	case io.SeekStart:
		abs = offset

	case io.SeekCurrent:
		abs = l.pos + offset

	case io.SeekEnd:
		abs = l.size + offset

	default:
		return 0, errors.New("invalid whence")
	}

	if abs < 0 {
		return 0, errors.New("negative position")
	}

	l.pos = abs

	return abs, nil
}
