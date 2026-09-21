package redfish

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	tests := []struct {
		name     string
		method   string
		body     string
		header   http.Header
		statuses []int
		deadline time.Duration

		wantRequests int
		wantStatus   int
		wantWaits    []time.Duration

		// wantWaitRange bounds a single wait, whose exact value depends on the
		// clock, instead of pinning it down.
		wantWaitRange [2]time.Duration
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
			header:   http.Header{"Retry-After": []string{time.Now().Add(4 * time.Second).UTC().Format(http.TimeFormat)}},
			statuses: []int{http.StatusServiceUnavailable, http.StatusOK},

			wantRequests:  2,
			wantStatus:    http.StatusOK,
			wantWaitRange: [2]time.Duration{3 * time.Second, 4 * time.Second},
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
			svr, requests := retryTestServer(t, tc.header, tc.statuses...)

			transport, waits := newTestRetryTransport(t, http.DefaultTransport)

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
			if tc.wantWaitRange != [2]time.Duration{} {
				require.Len(t, *waits, 1)
				require.Greater(t, (*waits)[0], tc.wantWaitRange[0])
				require.LessOrEqual(t, (*waits)[0], tc.wantWaitRange[1])
			} else {
				require.Equal(t, tc.wantWaits, *waits)
			}

			// The body of the answer handed out is still readable, so the caller
			// gets the details the BMC reported with it.
			content, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotEmpty(t, content)
		})
	}
}

func TestRetryTransport_RoundTripSharesItsBudget(t *testing.T) {
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
			got, ok := parseRetryAfter(tc.header, now)

			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
