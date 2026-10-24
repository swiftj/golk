package lk

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	backoffBase = 250 * time.Millisecond
	// maxWait caps every wait between attempts. A 429 or 503 asking for
	// longer is not retried (DECISIONS.md D-004, A-01).
	maxWait = 8 * time.Second
)

// attempt is the outcome of one HTTP attempt that did not succeed.
type attempt struct {
	apiErr *APIError
	// connErr is set when no response arrived.
	connErr  error
	timedOut bool
}

// retryable reports whether a failed attempt may be retried, per the SDK
// guide's Retries section. keyed reports whether the request carries an
// Idempotency-Key. Caller cancellation never reaches here.
func retryable(method string, keyed bool, a attempt) bool {
	unkeyedPost := method == http.MethodPost && !keyed
	if a.connErr != nil {
		// An unkeyed POST that timed out may have been applied.
		return !unkeyedPost || !a.timedOut
	}
	switch a.apiErr.Status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return !unkeyedPost
	case http.StatusConflict:
		return method == http.MethodPost && keyed && a.apiErr.Code == CodeInProgress
	}
	return false
}

// retryWait returns how long to wait before retry number n (0-based), or
// false when the server's Retry-After rules the retry out.
func retryWait(n int, a attempt, rnd randFloat) (time.Duration, bool) {
	if e := a.apiErr; e != nil && e.hasRetryAfter {
		if e.RetryAfter > maxWait && (e.Status == http.StatusTooManyRequests || e.Status == http.StatusServiceUnavailable) {
			return 0, false
		}
		return min(e.RetryAfter, maxWait), true
	}
	return backoff(n, rnd), true
}

// backoff is exponential with jitter: retry 0 waits 250–500ms, and each later
// wait doubles, capped at 8s.
func backoff(n int, rnd randFloat) time.Duration {
	base := backoffBase
	for i := 0; i < n && base < maxWait; i++ {
		base *= 2
	}
	d := base + time.Duration(rnd()*float64(base))
	return min(d, maxWait)
}

// parseRetryAfter reads a Retry-After header: delta-seconds (the documented
// form) or an HTTP-date. Anything else is treated as absent.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		if secs > int64(24*time.Hour/time.Second) {
			secs = int64(24 * time.Hour / time.Second)
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}
