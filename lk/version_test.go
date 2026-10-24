package lk

import (
	"context"
	"errors"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// versionedResult stands in for a versioned operation's result until the
// wiki surface (phase 2) adds real ones.
type versionedResult struct {
	Version string `json:"version"`
}

func (v *versionedResult) setVersionIfEmpty(s string) {
	if v.Version == "" {
		v.Version = s
	}
}

func sendVersioned(t *testing.T, c *Client, out any, opts ...CallOption) error {
	t.Helper()
	return c.call(context.Background(), "PUT", "/api/v3/projects/{projectId}/doc", nil,
		request{body: map[string]any{"blocks": []any{}}, out: out, versioned: true}, opts)
}

func TestVer_C_VER_01_IfMatchHeader(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"version":"v2"}`), testutil.JSON(200, `{"version":"v3"}`))
	c, _ := newTestClient(t, srv)
	var out versionedResult
	if err := sendVersioned(t, c, &out, IfMatch(`v1"opaque`)); err != nil {
		t.Fatal(err)
	}
	if err := sendVersioned(t, c, &out); err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()
	if got := r[0].Header.Get("If-Match"); got != `v1"opaque` {
		t.Errorf("If-Match = %q", got)
	}
	if _, ok := r[1].Header["If-Match"]; ok {
		t.Error("If-Match sent without IfMatch")
	}
	// Unversioned operations refuse IfMatch rather than ignore it.
	if _, err := c.Resources.Update(context.Background(), "r1", ResourceUpdate{}, IfMatch("v1")); !errors.Is(err, ErrIfMatchNotAllowed) {
		t.Errorf("unversioned IfMatch err = %v", err)
	}
}

func TestVer_C_VER_02_MismatchNotRetried(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(412, "VERSION_MISMATCH", "r"))
	c, _ := newTestClient(t, srv)
	err := sendVersioned(t, c, &versionedResult{}, IfMatch("v1"))
	if !errors.Is(err, ErrVersionMismatch) || !IsVersionMismatch(err) {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("412 retried: %d attempts", n)
	}
}

func TestVer_C_VER_03_C_VER_04_VersionOpaqueAndFromETag(t *testing.T) {
	for _, tc := range []struct {
		name, body, etag, want string
	}{
		{"body wins over ETag", `{"version":"from-body"}`, `"from-etag"`, "from-body"},
		{"strong ETag unquoted", `{}`, `"Zx9/+=="`, "Zx9/+=="},
		{"weak ETag (the server's form) unwrapped", `{}`, `W/"abc"`, `abc`},
		{"unquoted ETag kept", `{}`, `abc`, "abc"},
		{"no ETag, no version", `{}`, ``, ""},
		{"opaque body version untouched", `{"version":"  0042 "}`, ``, "  0042 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testutil.JSON(200, tc.body)
			if tc.etag != "" {
				r.Header["ETag"] = tc.etag
			}
			srv := testutil.NewFakeServer(t, r)
			c, _ := newTestClient(t, srv)
			var out versionedResult
			if err := sendVersioned(t, c, &out); err != nil {
				t.Fatal(err)
			}
			if out.Version != tc.want {
				t.Fatalf("Version = %q, want %q", out.Version, tc.want)
			}
		})
	}
}
