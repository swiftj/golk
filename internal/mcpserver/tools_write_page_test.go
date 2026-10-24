package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const wtBlocksV8 = `{"id":"d1","version":"v8","schema":1,"text":"Hi","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]}]}`

func TestEditPage_X_TOOL_05_StaleVersionRefused(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(blocksJSON))
	wtMustFail(t, h, "edit_page", map[string]any{"resource_id": "r1", "base_version": "v6", "markdown": "Hi\n"}, "call read_page again")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestEditPage_X_TOOL_03_X_TOOL_05_WritesWithIfMatchAndReports(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(blocksJSON), wtOK(wtBlocksV8))
	r := h.call("edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": "Hi\n"})
	if r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "PUT", wtPath+"/r1/docs/d1/blocks", "", `{"blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]}]}`)
	if got := w[0].Header.Get("If-Match"); got != `"v7"` && got != "v7" {
		t.Errorf("If-Match = %q", got)
	}
	got := text(r)
	for _, want := range []string{"1 block(s) kept, 0 changed, 0 added, 1 removed.", "WARNING: this removes 1 secret block(s)", "secret s1", "New version v8."} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q: %s", want, got)
		}
	}
	var out EditPageOut
	structured(t, r, &out)
	if out != (EditPageOut{ResourceID: "r1", DocumentID: "d1", Version: "v8", Kept: 1, Removed: 1, RemovedSecrets: 1}) {
		t.Errorf("structured %+v", out)
	}
	// The whole outgoing page is checked.
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], `"Hi"`) {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestEditPage_X_TOOL_04_DryRunSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(blocksJSON))
	got := wtMustOK(t, h, "edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": "Hi\n\nMore\n", "dry_run": true})
	if !strings.HasPrefix(got, `Dry run: would write page "Main"`) || !strings.Contains(got, "1 added") || !strings.HasSuffix(got, "Nothing was written.") {
		t.Errorf("text %q", got)
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestEditPage_X_GUARD_07_RefusalSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(blocksJSON))
	r := h.call("edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": "Hi Zorblax\n"})
	wtRefused(t, h, text(r))
}

func TestEditPage_X_GUARD_04_HiddenDocumentNotChecked(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(blocksJSON), wtOK(wtBlocksV8))
	wtMustOK(t, h, "edit_page", map[string]any{"resource_id": "r1", "document_id": "d2", "base_version": "v7", "markdown": "Zorblax\n"})
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
	if w := h.writes(); len(w) != 1 || w[0].Path != wtPath+"/r1/docs/d2/blocks" {
		t.Errorf("writes %+v", w)
	}
}

func TestEditPage_RemoveOpaqueNeedsFlag(t *testing.T) {
	h := newHarness(t)
	opaque := `{"id":"d1","version":"v7","schema":1,"text":"","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]},{"kind":"unsupported","id":"u1"}]}`
	h.lk.Enqueue(wtOK(wtRes), wtOK(opaque))
	wtMustFail(t, h, "edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": "Hi\n"}, "allow_remove_opaque")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestCreateDocument_X_TOOL_03_CreateThenContent(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), testutil.JSON(http.StatusCreated, `{"id":"d9","name":"Lore","type":"page","rank":"z","isHidden":false}`), wtOK(wtBlocksV8))
	got := wtMustOK(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "Lore", "markdown": "Hi\n"})
	if !strings.Contains(got, `Created page document "Lore" (d9) with 1 block(s)`) {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 2 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", wtPath+"/r1/docs", "", `{"name":"Lore","type":"page"}`)
	wtReq(t, w[1], "PUT", wtPath+"/r1/docs/d9/blocks", "", `{"blocks":[{"kind":"paragraph","content":[{"text":"Hi"}]}]}`)
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], `"blocks"`) {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestCreateDocument_X_TOOL_04_X_GUARD_07_X_GUARD_04(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes))
	got := wtMustOK(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "Lore", "type": "blank", "dry_run": true})
	if got != `Dry run: would create blank document "Lore" on "Ooo" (r1), visible to players. Nothing was written.` {
		t.Errorf("text %q", got)
	}
	// Refused before either write.
	h.lk.Enqueue(wtOK(wtRes))
	r := h.call("create_document", map[string]any{"resource_id": "r1", "name": "Lore", "markdown": "Zorblax\n"})
	wtRefused(t, h, text(r))
	// Hidden: not checked, and is_hidden is sent.
	n := len(h.guard.checked)
	h.lk.Enqueue(wtOK(wtRes), testutil.JSON(http.StatusCreated, `{"id":"d9","name":"Zorblax","type":"page","rank":"z","isHidden":true}`))
	wtMustOK(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "Zorblax", "is_hidden": true})
	if len(h.guard.checked) != n {
		t.Errorf("guard checked %v", h.guard.checked[n:])
	}
	wtReq(t, h.writes()[0], "POST", wtPath+"/r1/docs", "", `{"name":"Zorblax","type":"page","isHidden":true}`)
	wtMustFail(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "x", "type": "blank", "markdown": "Hi\n"}, "only for a page")
}
