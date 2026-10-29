package redfish

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const redactedValue = "<redacted>"

// redactedJSONKeys are the keys of a JSON body, whose string value is replaced
// in a recording. They are insurance, credentials are not expected in bodies.
var redactedJSONKeys = []string{"Password", "Token"}

// newExchangeObserver returns the observer, that hands the exchanges of one
// adapter call to the recorder of the scope.
func newExchangeObserver(ctx context.Context, scope provisioning.BMCRecordingScope) func(api.BMCRecordingExchange) {
	return func(exchange api.BMCRecordingExchange) {
		exchange.DeploymentState = scope.State
		exchange.StepRetries = scope.Retries

		scope.Recorder.Record(ctx, exchange)
	}
}

// recordedHeaders returns a copy of the headers with the credentials redacted.
func recordedHeaders(header http.Header) map[string][]string {
	if header == nil {
		return nil
	}

	recorded := make(map[string][]string, len(header))
	for name, values := range header {
		if slices.ContainsFunc(redactedHeaders, func(redacted string) bool {
			return strings.EqualFold(name, redacted)
		}) {
			recorded[name] = []string{redactedValue}
			continue
		}

		recorded[name] = slices.Clone(values)
	}

	return recorded
}

// recordedBody returns the body as text, or base64 encoded, if it is not
// valid UTF-8.
func recordedBody(data []byte, truncated bool) (body string, encoding string) {
	text := data
	if truncated {
		// The cut at the limit can split a multi byte character.
		text = trimIncompleteRune(data)
	}

	if utf8.Valid(text) {
		return string(text), ""
	}

	return base64.StdEncoding.EncodeToString(data), api.BMCRecordingBodyEncodingBase64
}

func trimIncompleteRune(data []byte) []byte {
	for i := len(data) - 1; i >= 0 && i >= len(data)-utf8.UTFMax; i-- {
		if utf8.RuneStart(data[i]) {
			if !utf8.FullRune(data[i:]) {
				return data[:i]
			}

			break
		}
	}

	return data
}

// redactJSONKeys replaces the string values of the keys at any depth of a
// JSON body. A body, that is not valid JSON, is returned unchanged. This
// includes a truncated body.
func redactJSONKeys(data []byte, keys ...string) []byte {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any

	err := decoder.Decode(&value)
	if err != nil || decoder.More() {
		return data
	}

	// Encoding changes the order of the keys, so it is only done if needed.
	if !redactJSONValue(value, keys) {
		return data
	}

	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(value)
	if err != nil {
		return data
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

func redactJSONValue(value any, keys []string) bool {
	redacted := false

	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			_, isString := child.(string)
			if isString && slices.ContainsFunc(keys, func(redactedKey string) bool {
				return strings.EqualFold(key, redactedKey)
			}) {
				value[key] = redactedValue
				redacted = true

				continue
			}

			redacted = redactJSONValue(child, keys) || redacted
		}

	case []any:
		for _, child := range value {
			redacted = redactJSONValue(child, keys) || redacted
		}
	}

	return redacted
}

// bodyCapture keeps the first bytes of a body up to the limit and counts all
// of them.
type bodyCapture struct {
	limit int
	data  []byte
	size  int64
}

func (c *bodyCapture) Write(p []byte) (int, error) {
	c.size += int64(len(p))

	room := c.limit - len(c.data)
	if room > 0 {
		c.data = append(c.data, p[:min(room, len(p))]...)
	}

	return len(p), nil
}

func (c *bodyCapture) truncated() bool {
	return c.size > int64(len(c.data))
}

// captureRequest captures the body of the request. The request, that is to
// be sent, carries the full body.
func captureRequest(req *http.Request, limit int) (*http.Request, *bodyCapture) {
	capture := &bodyCapture{limit: limit}

	if req.Body == nil || req.Body == http.NoBody {
		return req, capture
	}

	// GetBody returns an independent copy, so the request stays untouched.
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			_, _ = io.Copy(capture, body)
			_ = body.Close()
		}

		return req, capture
	}

	prefix, _ := io.ReadAll(io.LimitReader(req.Body, int64(limit)+1))
	_, _ = capture.Write(prefix)

	if capture.truncated() && req.ContentLength > capture.size {
		capture.size = req.ContentLength
	}

	sent := req.Clone(req.Context())
	sent.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: io.MultiReader(bytes.NewReader(prefix), req.Body),
		Closer: req.Body,
	}

	return sent, capture
}

// recordedBodyReader captures a response body, while the caller reads it. It
// emits the exchange once, on EOF or on Close, whichever comes first.
type recordedBodyReader struct {
	body io.ReadCloser
	emit func(capture *bodyCapture, complete bool)

	mu      sync.Mutex
	capture *bodyCapture
	done    bool
}

func (r *recordedBodyReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.done {
		_, _ = r.capture.Write(p[:n])
	}

	if errors.Is(err, io.EOF) {
		r.finish(true)
	}

	return n, err
}

func (r *recordedBodyReader) Close() error {
	err := r.body.Close()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.finish(false)

	return err
}

func (r *recordedBodyReader) finish(complete bool) {
	if r.done {
		return
	}

	r.done = true
	r.emit(r.capture, complete)
}
