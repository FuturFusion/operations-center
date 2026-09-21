package redfish

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

const retryDrainLimit = 64 * 1024

// retryTransport issues a request again, when the BMC turned it down with a
// server side error or asked to slow down. Only requests, that are safe to
// replay, are retried, see isReplaySafe.
//
// The retry sits below the Redfish client on purpose: a BMC, whose data sources
// are briefly unavailable, answers a handful of reads with 503 and is fine
// again seconds later.
//
// An outage, that outlasts the budget, still reaches the caller as the answer
// the BMC actually gave, so nothing is hidden.
type retryTransport struct {
	next     http.RoundTripper
	attempts int
	delay    time.Duration
	delayMax time.Duration

	// budget is the time left for waiting between attempts, in nanoseconds,
	// shared by every request of the connection.
	budget atomic.Int64

	sleep func(ctx context.Context, d time.Duration) error
}

// newRetryTransport returns the retry for one BMC connection. The delay before
// the first retry is passed rather than taken from the configuration, so a test
// does not have to wait for it in real time; everything else is scaled from it,
// so a shorter delay shortens the whole budget with it.
func newRetryTransport(next http.RoundTripper, delay time.Duration) *retryTransport {
	if delay <= 0 {
		delay = config.BMCRequestRetryDelay
	}

	scale := float64(delay) / float64(config.BMCRequestRetryDelay)

	t := &retryTransport{
		next:     next,
		attempts: config.BMCRequestRetries,
		delay:    delay,
		delayMax: time.Duration(float64(config.BMCRequestRetryDelayMax) * scale),
		sleep:    sleepUntilDone,
	}

	t.budget.Store(int64(float64(config.BMCRequestRetryBudget) * scale))

	return t
}

func sleepUntilDone(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isReplaySafe(req) {
		return t.next.RoundTrip(req)
	}

	delay := t.delay

	var resp *http.Response

	var err error

	for attempt := 0; ; attempt++ {
		resp, err = t.next.RoundTrip(req)

		if attempt >= t.attempts || !t.isWorthRetrying(resp, err) {
			return resp, err
		}

		// The deadline is checked before the budget is spent, so a wait, that is
		// not going to happen, does not cost the connection anything.
		wait := min(t.retryDelay(resp, delay), t.delayMax)
		if !t.fitsInDeadline(req.Context(), wait) || !t.spend(wait) {
			return resp, err
		}

		// The response is thrown away, so its body is drained and closed rather
		// than being left to the caller, which never sees it.
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, retryDrainLimit))
			_ = resp.Body.Close()
		}

		sleepErr := t.sleep(req.Context(), wait)
		if sleepErr != nil {
			return nil, sleepErr
		}

		delay *= 2
	}
}

// isReplaySafe reports, whether a request can be issued again.
func isReplaySafe(req *http.Request) bool {
	if req.Body != nil {
		return false
	}

	return req.Method == http.MethodGet || req.Method == http.MethodHead
}

// isWorthRetrying reports, whether the outcome of an attempt is one, that asking
// again can resolve.
func (t *retryTransport) isWorthRetrying(resp *http.Response, err error) bool {
	if err != nil {
		return false
	}

	return isTransientStatus(resp.StatusCode)
}

// isTransientStatus reports, whether a status means the BMC has not acted on the
// request yet, so asking again can still get it done.
func isTransientStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusServiceUnavailable, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusGatewayTimeout:
		return true
	}

	return false
}

// retryDelay returns, how long to wait before the next attempt, taking what the
// BMC asked for as a lower bound, so a BMC asking to slow down is not asked
// again sooner than that.
func (t *retryTransport) retryDelay(resp *http.Response, backoff time.Duration) time.Duration {
	if resp == nil {
		return backoff
	}

	retryAfter, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if !ok {
		return backoff
	}

	return max(backoff, retryAfter)
}

// spend takes the wait out of the budget of the connection, reporting, whether
// there was enough left for it.
func (t *retryTransport) spend(wait time.Duration) bool {
	for {
		left := t.budget.Load()
		if left < int64(wait) {
			return false
		}

		if t.budget.CompareAndSwap(left, left-int64(wait)) {
			return true
		}
	}
}

// fitsInDeadline reports, whether waiting leaves the caller time to do anything
// with the answer.
func (t *retryTransport) fitsInDeadline(ctx context.Context, wait time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx.Err() == nil
	}

	return time.Until(deadline) > wait
}

// parseRetryAfter parses a Retry-After header in either of the forms RFC 9110
// allows: a number of seconds or an HTTP date. A date in the past means, there
// is nothing left to wait for.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	seconds, err := strconv.Atoi(header)
	if err == nil {
		if seconds < 0 {
			return 0, true
		}

		return time.Duration(seconds) * time.Second, true
	}

	at, err := http.ParseTime(header)
	if err != nil {
		return 0, false
	}

	return max(at.Sub(now), 0), true
}
