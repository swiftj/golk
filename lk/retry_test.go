package lk

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

// scriptTransport fails the first len(fail) attempts that have a non-nil
// entry with that error, and passes the rest to the real transport.
type scriptTransport struct {
	mu    sync.Mutex
	fail  []error
	calls int
}

func (s *scriptTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	i := s.calls
	s.calls++
	var err error
	if i < len(s.fail) {
		err = s.fail[i]
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(r)
}

var errConnRefused = errors.New("dial tcp: connection refused")

type outcome int

const (
	status429 outcome = iota
	status500
	status502
	status503
	status504
	status400
	status401
	status403
	status404
	status409InProgress
	status409Other
	status412
	status422
	timedOut
	connFailure
)

func (o outcome) String() string {
	return [...]string{"429", "500", "502", "503", "504", "400", "401", "403", "404", "409 IN_PROGRESS", "409 HAS_CHILDREN", "412", "422", "timeout", "connection failure"}[o]
}

func (o outcome) reply(fc *testutil.FakeClock) testutil.Reply {
	switch o {
	case timedOut:
		return timeoutReply(fc)
	case status409InProgress:
		return testutil.APIError(409, "IN_PROGRESS", "r")
	case status409Other:
		return testutil.APIError(409, "HAS_CHILDREN", "r")
	}
	s := map[outcome]int{status429: 429, status500: 500, status502: 502, status503: 503, status504: 504,
		status400: 400, status401: 401, status403: 403, status404: 404, status412: 412, status422: 422}[o]
	return testutil.APIError(s, "INTERNAL", "r")
}

// attempts sends one call whose first attempt ends in o and whose second
// would succeed, and returns how many attempts were made.
func attempts(t *testing.T, method string, keyed bool, o outcome) int {
	t.Helper()
	srv := testutil.NewFakeServer(t)
	st := &scriptTransport{}
	c, fc := newTestClient(t, srv, WithMaxRetries(1), WithHTTPClient(&http.Client{Transport: st}))
	if o == connFailure {
		st.fail = []error{errConnRefused}
	} else {
		srv.Enqueue(o.reply(fc))
	}
	srv.Enqueue(testutil.JSON(200, `{}`))
	h := http.Header{}
	if keyed {
		h.Set("Idempotency-Key", "test-key-1")
	}
	var body any
	if method != http.MethodGet && method != http.MethodDelete {
		body = map[string]string{"name": "x"}
	}
	_ = c.Do(context.Background(), Request{Method: method, Path: "/api/v3/x", Body: body, Header: h}, nil)
	return st.calls
}

func TestRetry_C_RETRY_02_IdempotentMethods(t *testing.T) {
	retried := []outcome{status429, status502, status503, status504, timedOut, connFailure}
	notRetried := []outcome{status500, status400, status401, status403, status404, status409InProgress, status409Other, status412, status422}
	for _, m := range []string{"GET", "PUT", "PATCH", "DELETE"} {
		for _, o := range retried {
			if n := attempts(t, m, false, o); n != 2 {
				t.Errorf("%s on %s: %d attempts, want retry", m, o, n)
			}
		}
		for _, o := range notRetried {
			if n := attempts(t, m, false, o); n != 1 {
				t.Errorf("%s on %s: %d attempts, want no retry", m, o, n)
			}
		}
	}
}

func TestRetry_C_RETRY_03_KeyedPost(t *testing.T) {
	for _, o := range []outcome{status429, status502, status503, status504, timedOut, connFailure, status409InProgress} {
		if n := attempts(t, "POST", true, o); n != 2 {
			t.Errorf("keyed POST on %s: %d attempts, want retry", o, n)
		}
	}
}

func TestRetry_C_RETRY_04_UnkeyedPost(t *testing.T) {
	for o, want := range map[outcome]int{
		status429: 2, connFailure: 2,
		timedOut: 1, status502: 1, status503: 1, status504: 1,
	} {
		if n := attempts(t, "POST", false, o); n != want {
			t.Errorf("unkeyed POST on %s: %d attempts, want %d", o, n, want)
		}
	}
}

func TestRetry_C_RETRY_05_InProgressNeedsKey(t *testing.T) {
	if n := attempts(t, "POST", false, status409InProgress); n != 1 {
		t.Errorf("unkeyed POST retried IN_PROGRESS")
	}
	if n := attempts(t, "PATCH", false, status409InProgress); n != 1 {
		t.Errorf("PATCH retried IN_PROGRESS")
	}
}

func TestRetry_C_RETRY_06_NeverRetried(t *testing.T) {
	for _, m := range []string{"GET", "POST"} {
		for _, o := range []outcome{status500, status400, status401, status403, status404, status409Other, status412, status422} {
			if n := attempts(t, m, true, o); n != 1 {
				t.Errorf("%s on %s retried", m, o)
			}
		}
	}
}

func TestRetry_C_RETRY_01_C_RETRY_07_ExhaustedReturnsLastError(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.APIError(409, "IN_PROGRESS", "a"), testutil.APIError(409, "IN_PROGRESS", "b"), testutil.APIError(409, "IN_PROGRESS", "c"))
	c, _ := newTestClient(t, srv)
	_, err := c.Resources.Create(context.Background(), ResourceCreate{Name: "x"})
	var e *APIError
	if !errors.Is(err, ErrInProgress) || !errors.As(err, &e) || e.RequestID != "c" {
		t.Fatalf("err = %v, want the last IN_PROGRESS", err)
	}
	if n := len(srv.Requests()); n != DefaultMaxRetries+1 {
		t.Fatalf("attempts = %d, want maxRetries+1 = %d", n, DefaultMaxRetries+1)
	}
}

func TestRetry_C_RETRY_08_ConnectionDroppedAfterSend(t *testing.T) {
	drop := testutil.Reply{Handler: func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}}
	for _, keyed := range []bool{true, false} {
		srv := testutil.NewFakeServer(t, drop, testutil.JSON(201, `{}`))
		// No keep-alive, so net/http never retries a pooled connection itself.
		c, _ := newTestClient(t, srv, WithHTTPClient(&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}))
		h := http.Header{}
		if keyed {
			h.Set("Idempotency-Key", "k1")
		}
		err := c.Do(context.Background(), Request{Method: "POST", Path: "/api/v3/x", Body: map[string]int{"a": 1}, Header: h}, nil)
		if err != nil {
			t.Errorf("keyed=%v: dropped connection not retried: %v", keyed, err)
		}
		if n := len(srv.Requests()); n != 2 {
			t.Errorf("keyed=%v: %d requests, want 2", keyed, n)
		}
	}
}

func TestRetry_C_RETRY_09_BodyReplayedExactly(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(503, "INTERNAL", "a"), testutil.APIError(502, "INTERNAL", "b"), testutil.JSON(201, okResource))
	c, _ := newTestClient(t, srv)
	if _, err := c.Resources.Create(context.Background(), ResourceCreate{Name: "Gunter", Tags: Some([]string{"Penguin"})}); err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()
	for i := range r {
		assertJSON(t, r[i].Body, `{"name":"Gunter","tags":["Penguin"]}`)
	}
}

func TestBackoff_C_BACKOFF_01_HonorsRetryAfter(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.Reply{Status: 503, Header: map[string]string{"Retry-After": "2"}, Body: `{"message":"m","code":"INTERNAL","requestId":"r"}`},
		testutil.Reply{Status: 429, Header: map[string]string{"Retry-After": "0"}, Body: `{"message":"m","code":"RATE_LIMITED","requestId":"r"}`},
		testutil.JSON(200, okResource))
	c, fc := newTestClient(t, srv)
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if got := fc.Waits(); !slices.Equal(got, []time.Duration{2 * time.Second, 0}) {
		t.Fatalf("waits = %v", got)
	}
}

func TestBackoff_C_BACKOFF_02_LongRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		status    int
		code      string
		attempts  int
		waits     []time.Duration
		errStatus int
	}{
		// 429/503 asking for more than 8s fail at once, with RetryAfter set.
		{429, "RATE_LIMITED", 1, nil, 429},
		{503, "INTERNAL", 1, nil, 503},
		// Other retryable statuses wait the 8s cap and retry (D-004, A-01).
		{502, "INTERNAL", 2, []time.Duration{8 * time.Second}, 0},
		{504, "INTERNAL", 2, []time.Duration{8 * time.Second}, 0},
	} {
		srv := testutil.NewFakeServer(t,
			testutil.Reply{Status: tc.status, Header: map[string]string{"Retry-After": "9"}, Body: `{"message":"m","code":"` + tc.code + `","requestId":"r"}`},
			testutil.JSON(200, okResource))
		c, fc := newTestClient(t, srv)
		_, err := c.Resources.Get(context.Background(), "r1")
		if n := len(srv.Requests()); n != tc.attempts {
			t.Errorf("%d: %d attempts, want %d", tc.status, n, tc.attempts)
		}
		if got := fc.Waits(); !slices.Equal(got, tc.waits) {
			t.Errorf("%d: waits = %v, want %v", tc.status, got, tc.waits)
		}
		if tc.errStatus != 0 {
			var e *APIError
			if !errors.As(err, &e) || e.Status != tc.errStatus || e.RetryAfter != 9*time.Second {
				t.Errorf("%d: err = %v, want RetryAfter 9s", tc.status, err)
			}
		} else if err != nil {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
}

func TestBackoff_C_BACKOFF_03_ExponentialWithJitter(t *testing.T) {
	low := func() float64 { return 0 }
	high := func() float64 { return 0.999999 }
	if d := backoff(0, low); d != 250*time.Millisecond {
		t.Errorf("first wait low = %s", d)
	}
	if d := backoff(0, high); d < 499*time.Millisecond || d >= 500*time.Millisecond {
		t.Errorf("first wait high = %s", d)
	}
	srv := testutil.NewFakeServer(t)
	for range 7 {
		srv.Enqueue(testutil.APIError(503, "INTERNAL", "r"))
	}
	c, fc := newTestClient(t, srv, WithMaxRetries(6))
	_, _ = c.Resources.Get(context.Background(), "r1")
	want := []time.Duration{375 * time.Millisecond, 750 * time.Millisecond, 1500 * time.Millisecond,
		3 * time.Second, 6 * time.Second, 8 * time.Second}
	if got := fc.Waits(); !slices.Equal(got, want) {
		t.Fatalf("waits = %v, want %v", got, want)
	}
}

func TestBackoff_C_BACKOFF_04_RetryAfterHTTPDate(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.Reply{Status: 503, Header: map[string]string{"Retry-After": epoch.Add(3 * time.Second).Format(http.TimeFormat)},
			Body: `{"message":"m","code":"INTERNAL","requestId":"r"}`},
		testutil.Reply{Status: 503, Header: map[string]string{"Retry-After": "whenever"},
			Body: `{"message":"m","code":"INTERNAL","requestId":"r"}`},
		testutil.JSON(200, okResource))
	c, fc := newTestClient(t, srv)
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	// An HTTP-date is honored; an unparseable value falls back to backoff.
	if got := fc.Waits(); !slices.Equal(got, []time.Duration{3 * time.Second, 750 * time.Millisecond}) {
		t.Fatalf("waits = %v", got)
	}
}

// stuckClock never lets a wait finish; it cancels the call instead.
type stuckClock struct {
	*testutil.FakeClock
	cancel context.CancelFunc
}

func (s stuckClock) After(time.Duration) <-chan time.Time {
	s.cancel()
	return make(chan time.Time)
}

func TestBackoff_C_BACKOFF_05_WaitRespectsCancel(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(503, "INTERNAL", "r"))
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := newTestClient(t, srv)
	c.clock = stuckClock{testutil.NewFakeClock(epoch), cancel}
	_, err := c.Resources.Get(ctx, "r1")
	var ce *ConnectionError
	if !errors.As(err, &ce) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("%d attempts", n)
	}
}

func TestBackoff_C_BACKOFF_06_NoRealSleeps(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	for range 5 {
		srv.Enqueue(testutil.APIError(503, "INTERNAL", "r"))
	}
	c, fc := newTestClient(t, srv, WithMaxRetries(4))
	start := time.Now()
	_, _ = c.Resources.Get(context.Background(), "r1")
	var waited time.Duration
	for _, w := range fc.Waits() {
		waited += w
	}
	if waited < 5*time.Second {
		t.Fatalf("fake waits total %s; expected backoff to be recorded", waited)
	}
	if real := time.Since(start); real > 2*time.Second {
		t.Fatalf("retries took %s of real time", real)
	}
}
