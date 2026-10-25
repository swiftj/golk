package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

const (
	bkPath = "/api/v3/projects/p1/resources/r1/docs/d1/blocks"
	// bkPage has a public paragraph p1 and a secret s1 holding paragraph p2.
	bkPage = `{"id":"d1","version":"v7","blocks":[
{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]},
{"kind":"secret","id":"s1","children":[{"kind":"paragraph","id":"p2","content":[{"text":"Zorblax lurks"}]}]}]}`
	bkPara = `{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}],"version":"v7"}`
)

// X-BLOCK-01: read_block renders the block with ids and the page version.
func TestReadBlock_X_BLOCK_01(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(bkPara))
	got := wtMustOK(t, h, "read_block", map[string]any{"resource_id": "r1", "document_id": "d1", "block_id": "p1"})
	if !strings.Contains(got, "page version v7") || !strings.Contains(got, "Hi\n{#p1}") {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[0], http.MethodGet, bkPath+"/p1", "", "")
}

// X-BLOCK-02: insert_block sends one parsed block with its anchor and
// If-Match; a block inside a secret isn't checked, one in the open is.
func TestInsertBlock_X_BLOCK_02(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(bkPage), wtOK(`{"kind":"paragraph","id":"n1","content":[{"text":"Zorblax waits"}],"version":"v8"}`))
	wtMustOK(t, h, "insert_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "parent": "s1", "markdown": "Zorblax waits"})
	w := h.writes()
	wtReq(t, w[0], http.MethodPost, bkPath, "", `{"block":{"kind":"paragraph","content":[{"text":"Zorblax waits"}]},"parent":"s1"}`)
	if w[0].Header.Get("If-Match") != "v7" {
		t.Errorf("If-Match %q", w[0].Header.Get("If-Match"))
	}
	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(bkPage))
	wtRefused(t, h, wtMustFail(t, h, "insert_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "after": "p1", "markdown": "Zorblax waits"}, "spoiler guard"))
	wtMustFail(t, h, "insert_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "markdown": "One\n\nTwo"}, "exactly one block")
	wtMustFail(t, h, "insert_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "after": "p1", "parent": "s1", "markdown": "x"}, "at most one")
}

// X-BLOCK-03: replace_block and update_block send If-Match; update_block's
// patch uses API names, with content_markdown as runs.
func TestReplaceUpdateBlock_X_BLOCK_03(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(bkPage), wtOK(`{"kind":"heading","id":"p1","level":2,"content":[{"text":"Hi"}],"version":"v8"}`))
	wtMustOK(t, h, "replace_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1", "markdown": "## Hi"})
	wtReq(t, h.writes()[0], http.MethodPut, bkPath+"/p1", "", `{"kind":"heading","level":2,"content":[{"text":"Hi"}]}`)

	h = newHarness(t)
	h.lk.Enqueue(wtOK(`{"kind":"callout","id":"c1","tone":"info","children":[],"version":"v7"}`), wtOK(wtRes), wtOK(bkPage),
		wtOK(`{"kind":"callout","id":"c1","tone":"warning","children":[],"version":"v8"}`))
	wtMustOK(t, h, "update_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "c1", "patch": map[string]any{"tone": "warning"}})
	wtReq(t, h.writes()[0], http.MethodPatch, bkPath+"/c1", "", `{"tone":"warning"}`)

	h.lk.Enqueue(wtOK(bkPara), wtOK(wtRes), wtOK(bkPage), wtOK(bkPara))
	wtMustOK(t, h, "update_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1", "content_markdown": "Hello **you**"})
	wtReq(t, h.writes()[1], http.MethodPatch, bkPath+"/p1", "", `{"content":[{"text":"Hello "},{"text":"you","bold":true}]}`)

	h.lk.Enqueue(wtOK(bkPara))
	wtMustFail(t, h, "update_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1", "patch": map[string]any{"glow": true}}, `unknown field "glow"`)
}

// X-BLOCK-04, X-GUARD-15: move_block sends one anchor; moving a block out
// of a secret into the open is checked. delete_block's dry run shows it.
func TestMoveDeleteBlock_X_BLOCK_04_X_GUARD_15(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(bkPage))
	wtRefused(t, h, wtMustFail(t, h, "move_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p2", "after": "p1"}, "spoiler guard"))

	h.lk.Enqueue(wtOK(wtRes), wtOK(bkPage), wtOK(`{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}],"version":"v8"}`))
	wtMustOK(t, h, "move_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1", "parent": "s1"})
	wtReq(t, h.writes()[0], http.MethodPost, bkPath+"/p1/move", "", `{"parent":"s1"}`)

	h.lk.Enqueue(wtOK(bkPara))
	if got := wtMustOK(t, h, "delete_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1", "dry_run": true}); !strings.Contains(got, "would delete this paragraph block") {
		t.Errorf("text %q", got)
	}
	h.lk.Enqueue(wtOK(bkPara), noContentReply())
	wtMustOK(t, h, "delete_block", map[string]any{"resource_id": "r1", "document_id": "d1", "version": "v7", "block_id": "p1"})
	wtReq(t, h.writes()[1], http.MethodDelete, bkPath+"/p1", "", "")
}
