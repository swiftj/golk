package lk

import (
	"context"
	"errors"
	"strings"
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

func TestCore2_C_CORE2_01_VersionFromBodyOrETag(t *testing.T) {
	withETag := func(body, etag string) testutil.Reply {
		r := testutil.JSON(200, body)
		r.Header["ETag"] = etag
		return r
	}
	srv := testutil.NewFakeServer(t,
		withETag(`{"kind":"rule","id":"b1","version":"v-body"}`, `W/"v-etag"`),
		withETag(`{"kind":"rule","id":"b1"}`, `W/"v-etag"`))
	c, _ := newTestClient(t, srv)
	for _, want := range []string{"v-body", "v-etag"} {
		var out versionedBody
		if err := sendVersioned(t, c, &out); err != nil {
			t.Fatal(err)
		}
		if out.version != want {
			t.Errorf("version = %q, want %q", out.version, want)
		}
		// The value decodes without the version field.
		assertJSON(t, out.raw, `{"id":"b1","kind":"rule"}`)
	}
}

func TestCore2_C_ERR_12_CurrentVersionFromMessage(t *testing.T) {
	for msg, want := range map[string]string{
		"Document version is 90a1ea8e2700df9f, If-Match was 666d8c8987139620": "90a1ea8e2700df9f",
		"Board version is 1, If-Match was 2":                                  "",
		"Version mismatch":                                                    "",
	} {
		srv := testutil.NewFakeServer(t, testutil.JSON(412, `{"message":"`+msg+`","code":"VERSION_MISMATCH","requestId":"r"}`))
		c, _ := newTestClient(t, srv)
		err := sendVersioned(t, c, &versionedBody{}, IfMatch("666d8c8987139620"))
		var e *APIError
		if !errors.As(err, &e) || e.CurrentVersion != want {
			t.Errorf("%q: CurrentVersion = %q, want %q (err %v)", msg, e.CurrentVersion, want, err)
		}
	}
}

func TestCore2_C_CORE2_04_BodyLimit(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{}`))
	c, _ := newTestClient(t, srv)
	// The JSON encoding adds the quotes and the {"name":} wrapper.
	overhead := len(`{"name":""}`)
	big := ResourceCreate{Name: strings.Repeat("x", maxBodyBytes-overhead+1)}
	if _, err := c.Resources.Create(context.Background(), big); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("over the limit: err = %v", err)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("%d requests sent for an oversized body", n)
	}
	// Exactly at the limit is sent.
	srv = testutil.NewFakeServer(t, testutil.JSON(201, okResource))
	c, _ = newTestClient(t, srv)
	big.Name = big.Name[1:]
	if _, err := c.Resources.Create(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if n := len(srv.Requests()[0].Body); n != maxBodyBytes {
		t.Fatalf("body = %d bytes", n)
	}
}

func TestCore2_C_PAGE_05_SinglePage(t *testing.T) {
	p := singlePage([]int{1, 2}, false)
	if p.HasMore || p.NextCursor != "" {
		t.Fatalf("page = %+v", p)
	}
	var got []int
	for v, err := range p.All(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	// A whole-collection list that claims more is an error, not a truncation.
	var errs int
	for _, err := range singlePage([]int{1}, true).All(context.Background()) {
		if errors.Is(err, ErrPaginationInconsistent) {
			errs++
		}
	}
	if errs != 1 {
		t.Fatalf("inconsistent single page: %d errors", errs)
	}
}
