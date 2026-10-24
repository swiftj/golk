package lk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

func TestAuth_C_AUTH_01_BearerHeader(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if got := srv.Requests()[0].Header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization = %q", got)
	}
}

func TestAuth_C_AUTH_02_MissingKey(t *testing.T) {
	if _, err := New(""); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("New(\"\") err = %v, want ErrMissingAPIKey", err)
	}
	t.Setenv(EnvAPIKey, "")
	if _, err := NewFromEnv(); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("NewFromEnv err = %v, want ErrMissingAPIKey", err)
	}
}

func TestAuth_C_AUTH_03_KeyNeverPrintedOrLogged(t *testing.T) {
	var logs bytes.Buffer
	srv := testutil.NewFakeServer(t, testutil.APIError(404, "NOT_FOUND", "req-1"))
	c, _ := newTestClient(t, srv, WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	_, err := c.Resources.Get(context.Background(), "r1")
	for _, s := range []string{
		fmt.Sprint(c), fmt.Sprintf("%v %+v %#v %s", c, c, c, c),
		fmt.Sprintf("%v %+v %#v %s", c.apiKey, c.apiKey, c.apiKey, c.apiKey), err.Error(), logs.String(),
	} {
		if strings.Contains(s, testKey) {
			t.Fatalf("API key leaked: %q", s)
		}
	}
	if logs.Len() == 0 {
		t.Fatal("expected debug logs")
	}
}

func TestClient_C_CLIENT_01_ConstructionMakesNoRequest(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	_, _ = newTestClient(t, srv, WithProjectID(""))
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("New sent %d requests", n)
	}
}

func TestClient_C_CLIENT_02_BaseURL(t *testing.T) {
	c, err := New(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.baseURL.String(); got != "https://www.legendkeeper.com" {
		t.Errorf("default base URL = %q", got)
	}
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	c, _ = newTestClient(t, srv)
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if len(srv.Requests()) != 1 {
		t.Fatal("WithBaseURL not used")
	}
	for _, bad := range []string{"ftp://x", "not a url", "https://"} {
		if _, err := New(testKey, WithBaseURL(bad)); err == nil {
			t.Errorf("WithBaseURL(%q) accepted", bad)
		}
	}
}

func TestClient_C_CLIENT_03_C_CLIENT_04_Environment(t *testing.T) {
	t.Setenv(EnvAPIKey, testKey)
	t.Setenv(EnvBaseURL, "http://env.example")

	// New never reads the environment.
	c, err := New("other-key")
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL.String() != DefaultBaseURL {
		t.Errorf("New read %s: base URL = %s", EnvBaseURL, c.baseURL)
	}

	c, err = NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL.String() != "http://env.example" || string(c.apiKey) != testKey {
		t.Errorf("NewFromEnv base URL = %s", c.baseURL)
	}

	// An explicit option wins over the environment.
	c, _ = NewFromEnv(WithBaseURL("http://opt.example"))
	if c.baseURL.String() != "http://opt.example" {
		t.Errorf("option did not override env: %s", c.baseURL)
	}

	// An empty LEGENDKEEPER_BASE_URL counts as unset.
	t.Setenv(EnvBaseURL, "")
	c, _ = NewFromEnv()
	if c.baseURL.String() != DefaultBaseURL {
		t.Errorf("empty env base URL: got %s", c.baseURL)
	}
}

type countingTransport struct {
	mu   sync.Mutex
	n    int
	next http.RoundTripper
	err  error
}

func (ct *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ct.mu.Lock()
	ct.n++
	ct.mu.Unlock()
	if ct.err != nil {
		return nil, ct.err
	}
	return ct.next.RoundTrip(r)
}

func TestClient_C_CLIENT_05_CustomTransportSeesEveryAttempt(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(503, "INTERNAL", "a"), testutil.APIError(503, "INTERNAL", "b"), testutil.JSON(200, okResource))
	ct := &countingTransport{next: http.DefaultTransport}
	c, _ := newTestClient(t, srv, WithHTTPClient(&http.Client{Transport: ct}))
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if ct.n != 3 {
		t.Fatalf("transport saw %d attempts, want 3", ct.n)
	}
}

func TestClient_C_CLIENT_06_TransportErrorIsConnectionError(t *testing.T) {
	boom := errors.New("boom")
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv, WithHTTPClient(&http.Client{Transport: &countingTransport{err: boom}}), WithMaxRetries(0))
	_, err := c.Resources.Get(context.Background(), "r1")
	var ce *ConnectionError
	if !errors.As(err, &ce) || !errors.Is(err, boom) {
		t.Fatalf("err = %v, want ConnectionError wrapping boom", err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Fatal("ConnectionError must not be an APIError")
	}
}

func TestClient_C_CLIENT_07_CancelledContextNotRetried(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	ct := &countingTransport{next: http.DefaultTransport}
	c, _ := newTestClient(t, srv, WithHTTPClient(&http.Client{Transport: ct}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Resources.Get(ctx, "r1")
	var ce *ConnectionError
	if !errors.As(err, &ce) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want ConnectionError wrapping context.Canceled", err)
	}
	if ct.n > 1 {
		t.Fatalf("cancelled call made %d attempts", ct.n)
	}

	// Cancelled while the server is handling the request: one attempt, no retry.
	ctx, cancel = context.WithCancel(context.Background())
	srv.Enqueue(testutil.Reply{Handler: func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}})
	_, err = c.Resources.Get(ctx, "r1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("server saw %d requests, want 1", n)
	}
}

func TestClient_C_CLIENT_08_MaxRetries(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
		want int
	}{
		{"default 2", nil, 3},
		{"zero turns retries off", []Option{WithMaxRetries(0)}, 1},
		{"four", []Option{WithMaxRetries(4)}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t)
			for range tc.want {
				srv.Enqueue(testutil.APIError(503, "INTERNAL", "x"))
			}
			c, _ := newTestClient(t, srv, tc.opts...)
			_, err := c.Resources.Get(context.Background(), "r1")
			if !IsServer(err) {
				t.Fatalf("err = %v", err)
			}
			if n := len(srv.Requests()); n != tc.want {
				t.Fatalf("attempts = %d, want %d", n, tc.want)
			}
		})
	}
	if _, err := New(testKey, WithMaxRetries(-1)); err == nil {
		t.Fatal("negative retries accepted")
	}
}

func TestClient_C_CLIENT_09_IdempotencyKeysOnByDefault(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(201, okResource), testutil.JSON(201, okResource))
	c, _ := newTestClient(t, srv)
	_, _ = c.Resources.Create(context.Background(), ResourceCreate{Name: "a"})
	off, _ := newTestClient(t, srv, WithoutIdempotencyKeys())
	_, _ = off.Resources.Create(context.Background(), ResourceCreate{Name: "a"})
	r := srv.Requests()
	if r[0].Header.Get("Idempotency-Key") == "" {
		t.Error("default client sent no key")
	}
	if r[1].Header.Get("Idempotency-Key") != "" {
		t.Error("WithoutIdempotencyKeys still sent a key")
	}
}

func TestClient_C_CLIENT_10_ConcurrentUse(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	const n = 20
	for range n {
		srv.Enqueue(testutil.JSON(200, okResource))
	}
	c, _ := newTestClient(t, srv)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			_, err := c.Resources.Get(context.Background(), "r1")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestClient_C_CLIENT_11_ProjectFilledIn(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource), testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	_, _ = c.Resources.Get(context.Background(), "r1")
	_, _ = c.Resources.Get(context.Background(), "r1", ForProject("p2"))
	r := srv.Requests()
	if r[0].Path != "/api/v3/projects/p1/resources/r1" || r[1].Path != "/api/v3/projects/p2/resources/r1" {
		t.Fatalf("paths = %s, %s", r[0].Path, r[1].Path)
	}
}

func TestClient_C_CLIENT_12_EscapeHatch(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(200, `{"data":[]}`),
		testutil.APIError(503, "INTERNAL", "x"), testutil.JSON(201, `{"id":"r9"}`),
		testutil.APIError(503, "INTERNAL", "y"),
	)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()

	var out struct {
		Data []any `json:"data"`
	}
	err := c.Do(ctx, Request{Method: "GET", Path: "/api/v3/projects/px/calendars",
		Query: map[string][]string{"limit": {"5"}}, Header: http.Header{"X-Request-Id": {"guide-escape-hatch"}}}, &out)
	if err != nil {
		t.Fatal(err)
	}
	// A keyed POST through the hatch retries on 503 with the same key.
	var created struct{ ID string }
	err = c.Do(ctx, Request{Method: "POST", Path: "/api/v3/projects/px/resources", Body: map[string]string{"name": "x"},
		Header: http.Header{"Idempotency-Key": {"hatch-1"}}}, &created)
	if err != nil || created.ID != "r9" {
		t.Fatalf("keyed POST: %v %+v", err, created)
	}
	// An unkeyed POST gets no generated key and is not retried on 503.
	err = c.Do(ctx, Request{Method: "POST", Path: "/api/v3/projects/px/resources", Body: map[string]string{"name": "x"}}, nil)
	if !IsServer(err) {
		t.Fatalf("unkeyed POST err = %v", err)
	}
	r := srv.Requests()
	if len(r) != 4 {
		t.Fatalf("requests = %d, want 4", len(r))
	}
	if r[0].Path != "/api/v3/projects/px/calendars" || r[0].RawQuery != "limit=5" || r[0].Header.Get("X-Request-Id") != "guide-escape-hatch" {
		t.Errorf("GET sent %s?%s headers %v", r[0].Path, r[0].RawQuery, r[0].Header)
	}
	if r[1].Header.Get("Idempotency-Key") != "hatch-1" || r[2].Header.Get("Idempotency-Key") != "hatch-1" {
		t.Error("caller key not reused across retries")
	}
	if r[3].Header.Get("Idempotency-Key") != "" {
		t.Error("escape hatch generated a key")
	}
	if err := c.Do(ctx, Request{Method: "GET", Path: "no-slash"}, nil); err == nil {
		t.Error("relative path accepted")
	}
}

func TestTimeout_C_TIMEOUT_01_Default30s(t *testing.T) {
	c, _ := New(testKey)
	if c.timeout != 30*time.Second {
		t.Fatalf("default timeout = %s", c.timeout)
	}
	c, _ = New(testKey, WithTimeout(5*time.Second))
	if c.timeout != 5*time.Second {
		t.Fatalf("WithTimeout = %s", c.timeout)
	}
}

// timeoutReply fires the client's per-attempt timer, then waits for the
// client to abandon the request.
func timeoutReply(fc *testutil.FakeClock) testutil.Reply {
	return testutil.Reply{Handler: func(w http.ResponseWriter, r *http.Request) {
		fc.FireTimers()
		<-r.Context().Done()
	}}
}

func TestTimeout_C_TIMEOUT_02_C_TIMEOUT_04_FreshTimeoutPerRetry(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, fc := newTestClient(t, srv)
	srv.Enqueue(timeoutReply(fc), timeoutReply(fc), testutil.JSON(200, okResource))
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatalf("third attempt should succeed: %v", err)
	}

	srv.Enqueue(timeoutReply(fc), timeoutReply(fc), timeoutReply(fc))
	_, err := c.Resources.Get(context.Background(), "r1")
	var ce *ConnectionError
	if !errors.As(err, &ce) || !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ConnectionError with ErrTimeout", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Error("timeout reported as cancellation")
	}
}

func TestTimeout_C_TIMEOUT_03_CallerDeadlineNotRetried(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	srv.Enqueue(testutil.Reply{Handler: func(w http.ResponseWriter, r *http.Request) {
		cancel() // the caller's own limit ends first
		<-r.Context().Done()
	}})
	_, err := c.Resources.Get(ctx, "r1")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("caller cancellation retried: %d attempts", n)
	}
}
