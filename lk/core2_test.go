package lk

import (
	"context"
	"errors"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

func TestCore2_C_CORE2_05_CaptureResponse(t *testing.T) {
	ok := testutil.JSON(200, okResource)
	ok.Header["X-Request-Id"] = "req-ok"
	ok.Header["ETag"] = `"v9"`
	srv := testutil.NewFakeServer(t, ok)
	c, _ := newTestClient(t, srv)
	var meta ResponseMeta
	if _, err := c.Resources.Get(context.Background(), "r1", CaptureResponse(&meta)); err != nil {
		t.Fatal(err)
	}
	if meta.Status != 200 || meta.RequestID != "req-ok" || meta.ETag != `"v9"` || meta.Header.Get("Content-Type") != "application/json" {
		t.Errorf("success meta = %+v", meta)
	}

	// After a retried 503, meta describes the final response.
	srv = testutil.NewFakeServer(t, testutil.APIError(503, "INTERNAL", "req-1"), testutil.APIError(404, "NOT_FOUND", "req-2"))
	c, _ = newTestClient(t, srv)
	meta = ResponseMeta{}
	_, err := c.Resources.Get(context.Background(), "r1", CaptureResponse(&meta))
	if !errors.Is(err, ErrNotFound) || meta.Status != 404 || meta.RequestID != "req-2" {
		t.Errorf("error meta = %+v, err = %v", meta, err)
	}

	// The escape hatch takes call options too.
	srv = testutil.NewFakeServer(t, ok)
	c, _ = newTestClient(t, srv)
	meta = ResponseMeta{}
	if err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"}, nil, CaptureResponse(&meta)); err != nil || meta.ETag != `"v9"` {
		t.Errorf("Do meta = %+v, err = %v", meta, err)
	}

	// No response, no meta.
	c, _ = newTestClient(t, srv, WithBaseURL("http://127.0.0.1:1"), WithMaxRetries(0))
	meta = ResponseMeta{}
	var ce *ConnectionError
	if _, err := c.Resources.Get(context.Background(), "r1", CaptureResponse(&meta)); !errors.As(err, &ce) || meta.Status != 0 {
		t.Errorf("connection failure meta = %+v, err = %v", meta, err)
	}
}
