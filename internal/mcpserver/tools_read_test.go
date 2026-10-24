package mcpserver

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/testutil"
)

const resourceJSON = `{"id":"r1","name":"Candy Kingdom","isHidden":false,"documents":[
{"id":"m1","name":"Map","type":"map","rank":"a"},{"id":"d1","name":"Main","type":"page","rank":"b","isHidden":true}]}`

const blocksJSON = `{"id":"d1","version":"v7","schema":1,"text":"Hi","blocks":[
{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]},
{"kind":"secret","id":"s1","children":[{"kind":"paragraph","id":"p2","content":[{"text":"Hidden"}]}]}]}`

func TestReadPage_X_TOOL_01_FirstPageAsMarkdown(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON), testutil.JSON(http.StatusOK, blocksJSON))
	r := h.call("read_page", map[string]any{"resource_id": "r1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	want := "Candy Kingdom › Main (resource r1, document d1, version v7) [document hidden from players]\n\nHi\n\n:::secret\nHidden\n:::\n"
	if got := text(r); got != want {
		t.Errorf("text\n%q\nwant\n%q", got, want)
	}
	var p PageInfo
	structured(t, r, &p)
	if p != (PageInfo{ResourceID: "r1", ResourceName: "Candy Kingdom", DocumentID: "d1", DocumentName: "Main", DocumentHidden: true, Version: "v7"}) {
		t.Errorf("structured = %+v", p)
	}
	reqs := h.lk.Requests()
	paths := []string{reqs[0].Path, reqs[1].Path}
	if !slices.Equal(paths, []string{"/api/v3/projects/p1/resources/r1", "/api/v3/projects/p1/resources/r1/docs/d1/blocks"}) {
		t.Errorf("paths = %v", paths)
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestReadPage_X_TOOL_01_NotAPage(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON))
	r := h.call("read_page", map[string]any{"resource_id": "r1", "document_id": "m1"})
	if !r.IsError || !strings.Contains(text(r), "is a map document, not a page") {
		t.Errorf("result %v: %s", r.IsError, text(r))
	}
}

func TestTools_X_ERR_01_APIErrorHasCodeRequestIDAndHint(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.APIError(http.StatusNotFound, "NOT_FOUND", "req-42"))
	r := h.call("read_page", map[string]any{"resource_id": "nope"})
	got := text(r)
	for _, want := range []string{"NOT_FOUND", "HTTP 404", "req-42", "Hint:"} {
		if !r.IsError || !strings.Contains(got, want) {
			t.Errorf("error text lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, "test-key") {
		t.Error("error text holds the API key")
	}
}

func TestTools_X_TOOL_02_AnnotationsOnEveryTool(t *testing.T) {
	h := newHarness(t)
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		a := tool.Annotations
		if a == nil || a.Title == "" || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: annotations %+v", tool.Name, a)
			continue
		}
		if !a.ReadOnlyHint && a.DestructiveHint == nil {
			t.Errorf("%s: a write tool must say whether it is destructive", tool.Name)
		}
		if !a.ReadOnlyHint && !hasDryRun(tool) {
			t.Errorf("%s: a write tool must take dry_run", tool.Name)
		}
	}
}

func hasDryRun(t *mcp.Tool) bool {
	return strings.Contains(string(mustJSON(t.InputSchema)), `"dry_run"`)
}
