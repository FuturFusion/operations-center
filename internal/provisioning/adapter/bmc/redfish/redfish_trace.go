package redfish

import (
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish"
)

// traceWriter collects the HTTP dumps produced by gofish.
type traceWriter struct {
	sections []string
}

func (w *traceWriter) Write(p []byte) (int, error) {
	w.sections = append(w.sections, string(p))

	return len(p), nil
}

// String returns the collected dump sections with the Authorization header
// redacted.
func (w *traceWriter) String() string {
	sections := make([]string, 0, len(w.sections))

	for _, section := range w.sections {
		sections = append(sections, redactAuthorization(section))
	}

	return strings.Join(sections, "\n---\n")
}

// headersOnly reduces the collected dump sections to their headers, dropping
// request and response bodies, and redacts the Authorization header so BMC
// credentials never leave the daemon.
func (w *traceWriter) headersOnly() string {
	sections := make([]string, 0, len(w.sections))

	for _, section := range w.sections {
		head, _, found := strings.Cut(section, "\r\n\r\n")
		if !found {
			head = section
		}

		sections = append(sections, redactAuthorization(head))
	}

	return strings.Join(sections, "\n---\n")
}

// redactedHeaders are the headers whose value is replaced in an HTTP dump, so
// BMC credentials never leave the daemon.
var redactedHeaders = []string{
	"authorization", // auth credentials
	"x-auth-token",  // Redfish session token
	"cookie",        // cookie based Redfish session
	"set-cookie",    // cookie based Redfish session
}

// redactAuthorization replaces the value of every credential carrying header
// found in an HTTP dump.
func redactAuthorization(dump string) string {
	lines := strings.Split(dump, "\r\n")

	for i, line := range lines {
		name, _, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		name = strings.TrimSpace(name)

		for _, redacted := range redactedHeaders {
			if strings.EqualFold(name, redacted) {
				lines[i] = http.CanonicalHeaderKey(name) + ": <redacted>"

				break
			}
		}
	}

	return strings.Join(lines, "\n")
}

// traceRequests makes client record the HTTP requests and responses it
// exchanges with the BMC, until stop is called.
func traceRequests(client *gofish.APIClient) (trace *traceWriter, stop func()) {
	trace = &traceWriter{}
	client.SetDumpWriter(trace)

	return trace, func() {
		client.SetDumpWriter(nil)
	}
}
