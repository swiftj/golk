//go:build integration

package lk_test

// Phase 5 probes (SDK_CONTRACT.md A-38 to A-40). Run with
//
//	make test-integration ARGS='-run TestProbe5 -v ./lk/'
//
// and copy the observations into DECISIONS.md D-015.

import (
	"net/http"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestProbe5_Board covers A-38 (body version vs ETag), A-39 (a blank
// document read as a board, then written with that version) and A-40
// (listShapes' version vs getContent's).
func TestProbe5_Board(t *testing.T) {
	p := newProber(t)
	_, doc := p.page(sandbox.Name("Probe board"))
	empty, meta := p.must(http.MethodGet, doc+"/board", nil)
	t.Logf("A-39 blank read: version %q ETag %q pages %s shapes %s schema present %v", empty["version"], meta.ETag, show(empty["pages"]), show(empty["shapes"]), empty["schema"] != nil)
	v0, _ := empty["version"].(string)

	created, meta := p.must(http.MethodPost, doc+"/board/shapes", map[string]any{
		"shapes": []any{
			map[string]any{"id": "shape:a", "type": "geo", "x": 0, "y": 0, "props": map[string]any{"w": 100, "h": 50, "text": "Alpha"}},
			map[string]any{"id": "shape:b", "type": "geo", "x": 300, "y": 0, "props": map[string]any{"w": 100, "h": 50}},
			map[string]any{"id": "shape:ab", "type": "arrow"},
		},
		"bindings": []any{
			map[string]any{"type": "arrow", "fromId": "shape:ab", "toId": "shape:a", "props": map[string]any{"terminal": "start"}},
			map[string]any{"type": "arrow", "fromId": "shape:ab", "toId": "shape:b", "props": map[string]any{"terminal": "end"}},
		},
	}, lk.IfMatch(v0))
	t.Logf("A-39 first create with the blank read's version: status %d; A-38 body version %q ETag %q", meta.Status, created["version"], meta.ETag)
	v1, _ := created["version"].(string)

	doc0, _ := p.must(http.MethodGet, doc, nil)
	t.Logf("A-39 document type after the first create: %v", doc0["type"])

	list, meta := p.must(http.MethodGet, doc+"/board/shapes", nil)
	content, cmeta := p.must(http.MethodGet, doc+"/board", nil)
	t.Logf("A-40 listShapes version %q ETag %q; getContent version %q ETag %q; equal to create's %v",
		list["version"], meta.ETag, content["version"], cmeta.ETag, list["version"] == v1 && content["version"] == v1)
	if shapes, ok := list["data"].([]any); ok && len(shapes) > 0 {
		t.Logf("a stored shape: %s", show(shapes[0]))
	}

	up, meta := p.must(http.MethodPatch, doc+"/board/shapes", map[string]any{"shapes": []any{map[string]any{"id": "shape:b", "y": 60}}}, lk.IfMatch(v1))
	t.Logf("A-38 update: body version %q ETag %q", up["version"], meta.ETag)
	_, _, err := p.do(http.MethodPatch, doc+"/board/shapes", map[string]any{"shapes": []any{map[string]any{"id": "shape:b", "y": 70}}}, lk.IfMatch(v1))
	e := apiErr(t, err)
	t.Logf("stale If-Match: %d %s %q CurrentVersion %q", e.Status, e.Code, e.Message, e.CurrentVersion)
	v2, _ := up["version"].(string)

	del, meta := p.must(http.MethodPost, doc+"/board/shapes/delete", map[string]any{"ids": []string{"shape:a"}}, lk.IfMatch(v2))
	t.Logf("A-38 delete: body version %q ETag %q; shapeIds %s bindingIds %s", del["version"], meta.ETag, show(del["shapeIds"]), show(del["bindingIds"]))
	v3, _ := del["version"].(string)

	rep, meta := p.must(http.MethodPut, doc+"/board", map[string]any{"shapes": []any{map[string]any{"type": "text", "props": map[string]any{"text": "Omega"}}}}, lk.IfMatch(v3))
	t.Logf("A-38 replace: body version %q ETag %q; shapes %d pages %d", rep["version"], meta.ETag, len(rep["shapes"].([]any)), len(rep["pages"].([]any)))
}
