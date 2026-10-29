package redfish

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func newTestRetryTransport(t *testing.T, next http.RoundTripper) (*retryTransport, *[]time.Duration) {
	t.Helper()

	transport := newRetryTransport(next, 0)

	var waits []time.Duration

	transport.sleep = func(ctx context.Context, d time.Duration) error {
		err := ctx.Err()
		if err != nil {
			return err
		}

		waits = append(waits, d)

		return nil
	}

	return transport, &waits
}

func retryTestServer(t *testing.T, header http.Header, statuses ...int) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var requests atomic.Int64

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := int(requests.Add(1)) - 1

		status := statuses[min(got, len(statuses)-1)]

		for name, values := range header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"code":"Base.1.12.GeneralError"}}`))
	}))

	t.Cleanup(svr.Close)

	return svr, &requests
}

func TestRetryTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	retryAfterNow := time.Date(2026, 9, 3, 21, 4, 30, 0, time.UTC)

	tests := []struct {
		name     string
		method   string
		body     string
		header   http.Header
		statuses []int
		deadline time.Duration

		// now is the time the transport sees. A fixed time gives an exact wait
		// for a Retry-After date, which has a precision of one second.
		now time.Time

		wantRequests int
		wantStatus   int
		wantWaits    []time.Duration
	}{
		{
			name:     "a read, the BMC turns down transiently, is asked again",
			method:   http.MethodGet,
			statuses: []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusOK},

			wantRequests: 3,
			wantStatus:   http.StatusOK,
			wantWaits:    []time.Duration{1 * time.Second, 2 * time.Second},
		},
		{
			name:     "a BMC, that keeps turning the read down, is asked no more than the attempts allow",
			method:   http.MethodGet,
			statuses: []int{http.StatusServiceUnavailable},

			wantRequests: 4,
			// The answer of the BMC is handed out as it is, so the caller sees
			// what it actually said rather than an error of our own.
			wantStatus: http.StatusServiceUnavailable,
			wantWaits:  []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
		},
		{
			name:     "a BMC asking to slow down is not asked again sooner than that",
			method:   http.MethodGet,
			header:   http.Header{"Retry-After": []string{"3"}},
			statuses: []int{http.StatusTooManyRequests, http.StatusOK},

			wantRequests: 2,
			wantStatus:   http.StatusOK,
			wantWaits:    []time.Duration{3 * time.Second},
		},
		{
			name:     "a BMC asking for longer than the cap is not followed to the letter",
			method:   http.MethodGet,
			header:   http.Header{"Retry-After": []string{"30"}},
			statuses: []int{http.StatusServiceUnavailable, http.StatusOK},

			wantRequests: 2,
			wantStatus:   http.StatusOK,
			wantWaits:    []time.Duration{5 * time.Second},
		},
		{
			name:     "a BMC asking through a date is understood as well",
			method:   http.MethodGet,
			header:   http.Header{"Retry-After": []string{retryAfterNow.Add(4 * time.Second).Format(http.TimeFormat)}},
			statuses: []int{http.StatusServiceUnavailable, http.StatusOK},
			now:      retryAfterNow,

			wantRequests: 2,
			wantStatus:   http.StatusOK,
			wantWaits:    []time.Duration{4 * time.Second},
		},
		{
			name:     "a delete, the BMC turns down transiently, is asked again",
			method:   http.MethodDelete,
			statuses: []int{http.StatusServiceUnavailable, http.StatusOK},

			wantRequests: 2,
			wantStatus:   http.StatusOK,
			wantWaits:    []time.Duration{1 * time.Second},
		},
		{
			name:     "a write is never replayed",
			method:   http.MethodPost,
			body:     `{"Image":"https://oc.example.com/one.iso"}`,
			statuses: []int{http.StatusServiceUnavailable, http.StatusOK},

			wantRequests: 1,
			wantStatus:   http.StatusServiceUnavailable,
		},
		{
			name:     "a read, the BMC failed at, is not asked again",
			method:   http.MethodGet,
			statuses: []int{http.StatusInternalServerError, http.StatusOK},

			wantRequests: 1,
			wantStatus:   http.StatusInternalServerError,
		},
		{
			name:     "a read, the BMC turned down, is not asked again",
			method:   http.MethodGet,
			statuses: []int{http.StatusNotFound, http.StatusOK},

			wantRequests: 1,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:     "the answer of the BMC is preferred over running into the deadline",
			method:   http.MethodGet,
			statuses: []int{http.StatusServiceUnavailable},
			deadline: 50 * time.Millisecond,

			wantRequests: 1,
			wantStatus:   http.StatusServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svr, requests := retryTestServer(t, tc.header, tc.statuses...)

			transport, waits := newTestRetryTransport(t, http.DefaultTransport)

			if !tc.now.IsZero() {
				transport.now = func() time.Time { return tc.now }
			}

			ctx := t.Context()

			if tc.deadline > 0 {
				var cancel context.CancelFunc

				ctx, cancel = context.WithTimeout(ctx, tc.deadline)
				defer cancel()
			}

			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}

			req, err := http.NewRequestWithContext(ctx, tc.method, svr.URL, body)
			require.NoError(t, err)

			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.wantStatus, resp.StatusCode)
			require.Equal(t, int64(tc.wantRequests), requests.Load())
			require.Equal(t, tc.wantWaits, *waits)

			// The body of the answer handed out is still readable, so the caller
			// gets the details the BMC reported with it.
			content, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotEmpty(t, content)
		})
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type timeoutError struct{}

func (timeoutError) Error() string { return "net/http: timeout awaiting response headers" }

func (timeoutError) Timeout() bool { return true }

func (timeoutError) Temporary() bool { return true }

func TestRetryTransport_RoundTripTransportErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		body   string
		err    error

		wantRequests int
		wantWaits    []time.Duration
		assertErr    require.ErrorAssertionFunc
	}{
		{
			name:   "a delete, the BMC did not answer in time, is asked again",
			method: http.MethodDelete,
			err:    timeoutError{},

			wantRequests: 2,
			wantWaits:    []time.Duration{1 * time.Second},
			assertErr:    require.NoError,
		},
		{
			name:   "a read, whose connection the BMC dropped, is asked again",
			method: http.MethodGet,
			err:    &net.OpError{Op: "read", Err: syscall.ECONNRESET},

			wantRequests: 2,
			wantWaits:    []time.Duration{1 * time.Second},
			assertErr:    require.NoError,
		},
		{
			name:   "a read, whose connection the BMC closed, is asked again",
			method: http.MethodGet,
			err:    io.EOF,

			wantRequests: 2,
			wantWaits:    []time.Duration{1 * time.Second},
			assertErr:    require.NoError,
		},
		{
			name:   "a BMC, that can not be reached in time, is not asked again",
			method: http.MethodGet,
			err:    &net.OpError{Op: "dial", Err: timeoutError{}},

			wantRequests: 1,
			assertErr:    require.Error,
		},
		{
			name:   "a write, the BMC did not answer in time, is never replayed",
			method: http.MethodPost,
			body:   `{"CertificateString":"one"}`,
			err:    timeoutError{},

			wantRequests: 1,
			assertErr:    require.Error,
		},
		{
			name:   "a BMC, that can not be reached, is not asked again",
			method: http.MethodGet,
			err:    &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},

			wantRequests: 1,
			assertErr:    require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			requests := 0

			transport, waits := newTestRetryTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					return nil, tc.err
				}

				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			}))

			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "https://bmc.example.com/redfish/v1", body)
			require.NoError(t, err)

			resp, err := transport.RoundTrip(req)
			tc.assertErr(t, err)

			if resp != nil {
				_ = resp.Body.Close()
			}

			require.Equal(t, tc.wantRequests, requests)
			require.Equal(t, tc.wantWaits, *waits)
		})
	}
}

func TestRetryTransport_RoundTripChargesUnansweredAttempts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 3, 21, 4, 30, 0, time.UTC)
	requests := 0

	transport, waits := newTestRetryTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		now = now.Add(20 * time.Second)

		return nil, timeoutError{}
	}))
	transport.now = func() time.Time { return now }

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://bmc.example.com/redfish/v1", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	require.ErrorIs(t, err, timeoutError{})

	require.Equal(t, 2, requests, "the second attempt, that got no answer, does not fit in the budget anymore")
	require.Equal(t, []time.Duration{1 * time.Second}, *waits)
}

func TestRetryTransport_RoundTripSharesItsBudget(t *testing.T) {
	t.Parallel()

	svr, requests := retryTestServer(t, nil, http.StatusServiceUnavailable)

	transport, waits := newTestRetryTransport(t, http.DefaultTransport)
	transport.budget.Store(int64(3 * time.Second))

	for range 2 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, svr.URL, nil)
		require.NoError(t, err)

		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)

		_ = resp.Body.Close()
	}

	require.Equal(t, []time.Duration{1 * time.Second, 2 * time.Second}, *waits,
		"the second request found the budget of the connection spent",
	)
	require.Equal(t, int64(3+1), requests.Load(),
		"the first request was issued three times, the second one only once",
	)
}

func Test_parseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 3, 21, 4, 30, 0, time.UTC)

	tests := []struct {
		name   string
		header string

		want   time.Duration
		wantOK bool
	}{
		{name: "no header", header: "", want: 0, wantOK: false},
		{name: "seconds", header: "30", want: 30 * time.Second, wantOK: true},
		{name: "zero seconds", header: "0", want: 0, wantOK: true},
		{name: "negative seconds", header: "-5", want: 0, wantOK: true},
		{
			name:   "an HTTP date",
			header: now.Add(45 * time.Second).Format(http.TimeFormat),
			want:   45 * time.Second,
			wantOK: true,
		},
		{
			name:   "an HTTP date in the past leaves nothing to wait for",
			header: now.Add(-time.Hour).Format(http.TimeFormat),
			want:   0,
			wantOK: true,
		},
		{name: "not a duration at all", header: "soon", want: 0, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseRetryAfter(tc.header, now)

			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func newObservedRequest(method string, body string) func(t *testing.T) *http.Request {
	return func(t *testing.T) *http.Request {
		t.Helper()

		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}

		req, err := http.NewRequestWithContext(t.Context(), method, "https://bmc.example.com/redfish/v1", reader)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Basic YWRtaW46c2VjcmV0")

		return req
	}
}

type observedResponse struct {
	status int
	body   string
	err    error
}

func readAllAndClose(t *testing.T, resp *http.Response) {
	t.Helper()

	_, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestRetryTransport_RoundTripObserve(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 8, 15, 0, 0, time.UTC)
	redactedAuthorization := map[string][]string{"Authorization": {"<redacted>"}}
	responseHeaders := map[string][]string{"Content-Type": {"application/json"}, "X-Auth-Token": {"<redacted>"}}

	exchange := func(attempt int, method string, status int, body string) api.BMCRecordingExchange {
		return api.BMCRecordingExchange{
			Timestamp:       now,
			Attempt:         attempt,
			Method:          method,
			URL:             "https://bmc.example.com/redfish/v1",
			RequestHeaders:  redactedAuthorization,
			Status:          status,
			ResponseHeaders: responseHeaders,
			ResponseBody:    body,
			ResponseSize:    int64(len(body)),
		}
	}

	tests := []struct {
		name       string
		newRequest func(t *testing.T) *http.Request
		bodyLimit  int
		responses  []observedResponse
		consume    func(t *testing.T, resp *http.Response)

		assertErr     require.ErrorAssertionFunc
		wantSent      []string
		wantExchanges []api.BMCRecordingExchange
	}{
		{
			name:       "a read produces one exchange",
			newRequest: newObservedRequest(http.MethodGet, ""),
			responses:  []observedResponse{{status: http.StatusOK, body: `{"Id":"1"}`}},
			consume:    readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{""},
			wantExchanges: []api.BMCRecordingExchange{
				exchange(0, http.MethodGet, http.StatusOK, `{"Id":"1"}`),
			},
		},
		{
			name:       "a retried read produces one exchange per attempt",
			newRequest: newObservedRequest(http.MethodGet, ""),
			responses: []observedResponse{
				{status: http.StatusServiceUnavailable, body: "busy"},
				{status: http.StatusOK, body: "ok"},
			},
			consume: readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{"", ""},
			wantExchanges: []api.BMCRecordingExchange{
				exchange(0, http.MethodGet, http.StatusServiceUnavailable, "busy"),
				exchange(1, http.MethodGet, http.StatusOK, "ok"),
			},
		},
		{
			name:       "a write has its password redacted in the recording only",
			newRequest: newObservedRequest(http.MethodPatch, `{"UserName":"admin","Password":"secret"}`),
			responses: []observedResponse{
				{status: http.StatusServiceUnavailable, body: "busy"},
				{status: http.StatusOK, body: "ok"},
			},
			consume: readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{`{"UserName":"admin","Password":"secret"}`},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodPatch, http.StatusServiceUnavailable, "busy")
					e.RequestBody = `{"Password":"<redacted>","UserName":"admin"}`
					e.RequestSize = 40

					return e
				}(),
			},
		},
		{
			name:       "a request body above the limit is sent in full and recorded truncated",
			newRequest: newObservedRequest(http.MethodPost, "0123456789abcdef"),
			bodyLimit:  8,
			responses:  []observedResponse{{status: http.StatusOK}},
			consume:    readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{"0123456789abcdef"},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodPost, http.StatusOK, "")
					e.RequestBody = "01234567"
					e.RequestSize = 16
					e.RequestTruncated = true

					return e
				}(),
			},
		},
		{
			name: "a request body without GetBody is sent in full",
			newRequest: func(t *testing.T) *http.Request {
				t.Helper()

				req := newObservedRequest(http.MethodPost, "0123456789abcdef")(t)
				req.GetBody = nil

				return req
			},
			bodyLimit: 8,
			responses: []observedResponse{{status: http.StatusOK}},
			consume:   readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{"0123456789abcdef"},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodPost, http.StatusOK, "")
					e.RequestBody = "01234567"
					e.RequestSize = 16
					e.RequestTruncated = true

					return e
				}(),
			},
		},
		{
			name:       "a response body above the limit is recorded truncated",
			newRequest: newObservedRequest(http.MethodGet, ""),
			bodyLimit:  8,
			responses:  []observedResponse{{status: http.StatusOK, body: "0123456789abcdef"}},
			consume:    readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{""},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodGet, http.StatusOK, "01234567")
					e.ResponseSize = 16
					e.ResponseTruncated = true

					return e
				}(),
			},
		},
		{
			name:       "a truncated text body cut inside a character stays text",
			newRequest: newObservedRequest(http.MethodGet, ""),
			bodyLimit:  4,
			responses:  []observedResponse{{status: http.StatusOK, body: "abcé"}},
			consume:    readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{""},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodGet, http.StatusOK, "abc")
					e.ResponseSize = 5
					e.ResponseTruncated = true

					return e
				}(),
			},
		},
		{
			name:       "a binary response body is recorded base64 encoded",
			newRequest: newObservedRequest(http.MethodGet, ""),
			responses:  []observedResponse{{status: http.StatusOK, body: "\xff\xfe\x00"}},
			consume:    readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{""},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodGet, http.StatusOK, "//4A")
					e.ResponseBodyEncoding = api.BMCRecordingBodyEncodingBase64
					e.ResponseSize = 3

					return e
				}(),
			},
		},
		{
			name:       "a transport error of a read is recorded for the attempt",
			newRequest: newObservedRequest(http.MethodGet, ""),
			responses: []observedResponse{
				{err: io.EOF},
				{status: http.StatusOK, body: "ok"},
			},
			consume: readAllAndClose,

			assertErr: require.NoError,
			wantSent:  []string{"", ""},
			wantExchanges: []api.BMCRecordingExchange{
				{
					Timestamp:      now,
					Attempt:        0,
					Method:         http.MethodGet,
					URL:            "https://bmc.example.com/redfish/v1",
					RequestHeaders: redactedAuthorization,
					TransportError: "EOF",
				},
				exchange(1, http.MethodGet, http.StatusOK, "ok"),
			},
		},
		{
			name:       "a transport error of a write is recorded",
			newRequest: newObservedRequest(http.MethodPost, "{}"),
			responses:  []observedResponse{{err: timeoutError{}}},
			consume:    func(*testing.T, *http.Response) {},

			assertErr: require.Error,
			wantSent:  []string{"{}"},
			wantExchanges: []api.BMCRecordingExchange{
				{
					Timestamp:      now,
					Attempt:        0,
					Method:         http.MethodPost,
					URL:            "https://bmc.example.com/redfish/v1",
					RequestHeaders: redactedAuthorization,
					RequestBody:    "{}",
					RequestSize:    2,
					TransportError: timeoutError{}.Error(),
				},
			},
		},
		{
			name:       "a body closed before EOF is recorded once and truncated",
			newRequest: newObservedRequest(http.MethodGet, ""),
			responses:  []observedResponse{{status: http.StatusOK, body: "0123456789"}},
			consume: func(t *testing.T, resp *http.Response) {
				t.Helper()

				_, err := io.ReadFull(resp.Body, make([]byte, 4))
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.NoError(t, resp.Body.Close(), "a second close is harmless")
			},

			assertErr: require.NoError,
			wantSent:  []string{""},
			wantExchanges: []api.BMCRecordingExchange{
				func() api.BMCRecordingExchange {
					e := exchange(0, http.MethodGet, http.StatusOK, "0123")
					e.ResponseSize = 4
					e.ResponseTruncated = true

					return e
				}(),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var sent []string

			transport, _ := newTestRetryTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body := ""
				if req.Body != nil {
					content, err := io.ReadAll(req.Body)
					require.NoError(t, err)

					body = string(content)
				}

				sent = append(sent, body)
				response := tc.responses[len(sent)-1]

				if response.err != nil {
					return nil, response.err
				}

				return &http.Response{
					StatusCode: response.status,
					Header:     http.Header{"Content-Type": {"application/json"}, "X-Auth-Token": {"token"}},
					Body:       io.NopCloser(strings.NewReader(response.body)),
				}, nil
			}))
			transport.now = func() time.Time { return now }

			if tc.bodyLimit > 0 {
				transport.bodyLimit = tc.bodyLimit
			}

			var exchanges []api.BMCRecordingExchange

			transport.observe = func(exchange api.BMCRecordingExchange) {
				exchanges = append(exchanges, exchange)
			}

			// Run test
			resp, err := transport.RoundTrip(tc.newRequest(t)) //nolint:bodyclose // Closed by consume.
			tc.assertErr(t, err)
			tc.consume(t, resp)

			// Assert
			require.Equal(t, tc.wantSent, sent, "the BMC receives the original request bodies")
			require.Equal(t, tc.wantExchanges, exchanges)
		})
	}
}
