package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const docJSON = `{"id":"d1","name":"Secrets","type":"page","rank":"b","isHidden":false}`

func TestUpdateDocument_X_TOOL_03_RenameSendsOnlyName(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON), testutil.JSON(http.StatusOK, docJSON))
	r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "name": "Secrets"})
	if r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 || w[0].Method != http.MethodPatch || w[0].Path != "/api/v3/projects/p1/resources/r1/docs/d1" || string(w[0].Body) != `{"name":"Secrets"}` {
		t.Fatalf("writes = %+v", w)
	}
	// The document stays hidden, so nothing players see is checked.
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestUpdateDocument_X_GUARD_05_UnhideScansThePage(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Hidden"}
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON), testutil.JSON(http.StatusOK, blocksJSON))
	r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "is_hidden": false})
	if !r.IsError || !strings.Contains(text(r), "spoiler guard") {
		t.Fatalf("want a guard refusal, got %v: %s", r.IsError, text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("a refused write sent %+v", h.writes())
	}
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], `"blocks"`) {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestUpdateDocument_X_TOOL_04_DryRunSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON), testutil.JSON(http.StatusOK, blocksJSON))
	r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "is_hidden": false, "dry_run": true})
	if r.IsError || !strings.Contains(text(r), "Dry run: would show it to players") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("dry run sent %+v", h.writes())
	}
}

// Without a terms file nothing is protected: writes go through unchecked
// (D-017), and list_projects says so.
func TestWrites_X_GUARD_06_NoTermsFileWrites(t *testing.T) {
	h := newHarness(t, noGuard())
	h.lk.Enqueue(testutil.JSON(http.StatusOK, resourceJSON), testutil.JSON(http.StatusOK, `{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false}`))
	r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "is_hidden": false})
	if r.IsError || len(h.writes()) != 1 {
		t.Fatalf("%v: %s; writes %+v", r.IsError, text(r), h.writes())
	}
	h.lk.Enqueue(testutil.JSON(http.StatusOK, `{"id":"p1","name":"Ooo"}`))
	lp := h.call("list_projects", map[string]any{})
	var out ListProjectsOut
	structured(t, lp, &out)
	if out.Guard != "off (no terms file)" || !strings.Contains(text(lp), "spoiler guard: off (no terms file)") {
		t.Errorf("guard state = %q", out.Guard)
	}
}

func TestReadOnly_X_RO_01_NoWriteTools(t *testing.T) {
	h := newHarness(t, readOnlyMode())
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("read-only mode lists %s", tool.Name)
		}
	}
	r, err := h.cs.CallTool(t.Context(), callParams("update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "name": "x"}))
	if err == nil && !r.IsError {
		t.Error("a write tool ran in read-only mode")
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests sent: %+v", h.lk.Requests())
	}
}
