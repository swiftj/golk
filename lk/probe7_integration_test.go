//go:build integration

package lk_test

// showPropertyBar probe. Run with
//
//	make test-integration ARGS='-run TestProbe7 -v ./lk/'
//
// The spec has Resource.showPropertyBar (read) but no write field for it.
// This records what the server does with it on create and update.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

func TestProbe7_ShowPropertyBar(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	pid := sandbox.ProjectID(t)
	r := create(t, c, lk.ResourceCreate{Name: sandbox.Name("Probe property bar")})
	t.Logf("new resource showPropertyBar=%v", r.ShowPropertyBar)
	path := "/api/v3/projects/" + pid + "/resources/" + r.ID
	for _, v := range []bool{true, false} {
		var out json.RawMessage
		err := c.Do(ctx, lk.Request{Method: http.MethodPatch, Path: path, Body: map[string]any{"showPropertyBar": v}}, &out)
		t.Logf("PATCH showPropertyBar=%v: err=%v", v, err)
		got, gerr := c.Resources.Get(ctx, r.ID)
		if gerr != nil {
			t.Fatal(gerr)
		}
		t.Logf("  after: showPropertyBar=%v", got.ShowPropertyBar)
	}
	var out json.RawMessage
	err := c.Do(ctx, lk.Request{Method: http.MethodPost, Path: "/api/v3/projects/" + pid + "/resources",
		Body: map[string]any{"name": sandbox.Name("Probe property bar 2"), "showPropertyBar": true}}, &out)
	t.Logf("POST create with showPropertyBar=true: err=%v", err)
	if err == nil {
		var made struct {
			ID              string `json:"id"`
			ShowPropertyBar bool   `json:"showPropertyBar"`
		}
		_ = json.Unmarshal(out, &made)
		sandbox.Cleanup(t, made.ID)
		t.Logf("  created showPropertyBar=%v", made.ShowPropertyBar)
	}
}
