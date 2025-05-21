package response

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
}

// ReadCloserResponse returns a new file taking the file content from a io.ReadCloser.
func ReadCloserResponse(r *http.Request, rc io.ReadCloser, filename string, fileSize int, headers map[string]string) Response {
	return &readCloserResponse{
		req:      r,
		rc:       rc,
		filename: filename,
		fileSize: fileSize,
		headers:  headers,
	}
}

func (r readCloserResponse) Render(w http.ResponseWriter) error {
	if r.headers != nil {
		for k, v := range r.headers {
			w.Header().Set(k, v)
		}
	}

	// Only set Content-Type header if it is still set to the default or not yet set at all.
	if w.Header().Get("Content-Type") == "application/json" || w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", r.filename))
	w.Header().Set("Content-Length", strconv.Itoa(r.fileSize))

	_, err := io.Copy(w, r.rc)
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
