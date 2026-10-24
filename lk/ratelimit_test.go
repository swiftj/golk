package lk

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

func TestRate_C_RATE_01_C_RATE_03_RequestAndWriteLimiters(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	for range 6 {
		srv.Enqueue(testutil.JSON(200, okResource))
	}
	c, fc := newTestClient(t, srv, WithRateLimits(RateLimits{RequestsPerMinute: 60, WritesPerMinute: 20, RequestBurst: 1, WriteBurst: 1}))
	ctx := context.Background()
	for range 3 {
		if _, err := c.Resources.Get(ctx, "r1"); err != nil {
			t.Fatal(err)
		}
	}
	// 60/min with burst 1: the 2nd and 3rd reads wait 1s each.
	if got := fc.Waits(); !slices.Equal(got, []time.Duration{time.Second, time.Second}) {
		t.Fatalf("read waits = %v", got)
	}
	// Writes draw on both limiters. The 1st write (t=2s) waits 1s for a
	// request token. The 2nd (t=3s) needs a request token at 4s but a write
	// token at 5s (20/min = every 3s): it waits 2s.
	for range 2 {
		if _, err := c.Resources.Update(ctx, "r1", ResourceUpdate{Name: Some("x")}); err != nil {
			t.Fatal(err)
		}
	}
	waits := fc.Waits()[2:]
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Fatalf("write waits = %v, want [1s 2s]", waits)
	}
}

func TestRate_C_RATE_02_Defaults(t *testing.T) {
	d := DefaultRateLimits
	if d.RequestsPerMinute <= 0 || d.RequestsPerMinute >= 300 || d.WritesPerMinute <= 0 || d.WritesPerMinute >= 120 {
		t.Fatalf("defaults not safely under 300/120: %+v", d)
	}
	// Even a full burst stays under the minute tiers.
	if float64(d.RequestBurst)+d.RequestsPerMinute > 300 || float64(d.WriteBurst)+d.WritesPerMinute > 120 {
		t.Fatalf("burst pushes a minute over the limit: %+v", d)
	}
	c, _ := New(testKey)
	if c.limiter == nil {
		t.Fatal("client has no default limiter")
	}
}

func TestRate_C_RATE_03_RetriesWaitOnLimiter(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(503, "INTERNAL", "r"), testutil.JSON(200, okResource))
	c, fc := newTestClient(t, srv, WithRateLimits(RateLimits{RequestsPerMinute: 60, WritesPerMinute: 60}))
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	// Backoff 375ms, then the limiter holds the retry until 1s after the first.
	if got := fc.Waits(); !slices.Equal(got, []time.Duration{375 * time.Millisecond, 625 * time.Millisecond}) {
		t.Fatalf("waits = %v", got)
	}
}

func TestRate_C_RATE_03_WaitRespectsCancel(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := newTestClient(t, srv, WithRateLimits(RateLimits{RequestsPerMinute: 1}))
	if _, err := c.Resources.Get(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	c.clock = stuckClock{testutil.NewFakeClock(epoch), cancel}
	_, err := c.Resources.Get(ctx, "r1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("%d requests", n)
	}
}

func TestRate_C_RATE_04_SharedAndDisabled(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource), testutil.JSON(200, okResource), testutil.JSON(200, okResource))
	shared := NewRateLimiter(RateLimits{RequestsPerMinute: 60, WritesPerMinute: 60})
	fc := testutil.NewFakeClock(epoch)
	a, _ := newTestClient(t, srv, WithRateLimiter(shared), withClock(fc))
	b, _ := newTestClient(t, srv, WithRateLimiter(shared), withClock(fc))
	ctx := context.Background()
	_, _ = a.Resources.Get(ctx, "r1")
	_, _ = b.Resources.Get(ctx, "r1")
	if got := fc.Waits(); !slices.Equal(got, []time.Duration{time.Second}) {
		t.Fatalf("shared limiter waits = %v", got)
	}
	off, offClock := newTestClient(t, srv, WithRateLimiter(nil))
	_, _ = off.Resources.Get(ctx, "r1")
	if len(offClock.Waits()) != 0 {
		t.Fatal("disabled limiter waited")
	}
}

func TestRate_C_RATE_05_ExhaustedRateLimit(t *testing.T) {
	rl := testutil.Reply{Status: 429, Header: map[string]string{"Retry-After": "1"}, Body: `{"message":"m","code":"RATE_LIMITED","requestId":"r"}`}
	srv := testutil.NewFakeServer(t, rl, rl, rl)
	c, _ := newTestClient(t, srv)
	_, err := c.Resources.Get(context.Background(), "r1")
	var e *APIError
	if !errors.Is(err, ErrRateLimited) || !errors.As(err, &e) || e.RetryAfter != time.Second {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.Requests()); n != 3 {
		t.Fatalf("%d attempts", n)
	}
}

func TestRate_C_RATE_06_CreateManyIsOneWrite(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(201, `{"resources":[]}`), testutil.JSON(201, okResource))
	c, fc := newTestClient(t, srv, WithRateLimits(RateLimits{RequestsPerMinute: 1000, WritesPerMinute: 1}))
	items := make([]ResourceCreate, 100)
	for i := range items {
		items[i] = ResourceCreate{Name: "r"}
	}
	if _, err := c.Resources.CreateMany(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if len(fc.Waits()) != 0 || len(srv.Requests()) != 1 {
		t.Fatalf("100 items took %d requests and %v waits", len(srv.Requests()), fc.Waits())
	}
	// The next write waits for the single write token the batch used.
	_, _ = c.Resources.Create(context.Background(), ResourceCreate{Name: "x"})
	if got := fc.Waits(); len(got) != 1 || got[0] != time.Minute {
		t.Fatalf("waits = %v, want [1m]", got)
	}
}
