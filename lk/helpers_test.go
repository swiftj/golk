package lk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

// testKey is a made-up credential; it is never valid anywhere.
const testKey = "test-key-not-real"

var epoch = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// newTestClient returns a client pointed at srv with a fake clock, a fixed
// jitter of 0.5, no client-side pacing and project "p1", plus the clock.
// Later options override these.
func newTestClient(t *testing.T, srv *testutil.FakeServer, opts ...Option) (*Client, *testutil.FakeClock) {
	t.Helper()
	fc := testutil.NewFakeClock(epoch)
	base := []Option{
		WithBaseURL(srv.URL),
		WithProjectID("p1"),
		withClock(fc),
		withRand(func() float64 { return 0.5 }),
		WithRateLimiter(nil),
	}
	c, err := New(testKey, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c, fc
}

// assertJSON fails unless got is exactly want (byte-for-byte).
func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	if string(got) != want {
		t.Errorf("body\n got: %s\nwant: %s", got, want)
	}
}

// mustJSON compacts a JSON literal so tests can write it readably.
func mustJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

const resourceJSON = `{"id":"r1","name":"The Tree Fort","tags":[],"aliases":[],"rank":"a0","isHidden":false,
"permissions":{},"documents":[{"id":"d1","name":"Main","type":"blank","rank":"a0","isHidden":false}],"properties":[]}`

const okResource = `{"id":"r1","name":"The Tree Fort","tags":[],"aliases":[],"rank":"a0","isHidden":false,"permissions":{},"documents":[],"properties":[]}`
