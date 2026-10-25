package mcpserver

import (
	"github.com/swiftj/golk/internal/testutil"
	"net/http"
	"strings"
	"testing"
)

const dtDoc = `{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false}`

// X-DOC-01: create_document takes every type, full width and a timeline's
// calendar.
func TestCreateDocument_X_DOC_01_Types(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(`{"id":"t9","name":"History","type":"time","rank":"d"}`))
	wtMustOK(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "History", "type": "time", "calendar_id": "gregorian", "is_full_width": true})
	wtReq(t, h.writes()[0], http.MethodPost, wtPath+"/r1/docs", "", `{"name":"History","type":"time","isFullWidth":true,"calendarId":"gregorian"}`)
	wtMustFail(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "X", "type": "scroll"}, "not a document type")
	wtMustFail(t, h, "create_document", map[string]any{"resource_id": "r1", "name": "X", "type": "map", "calendar_id": "gregorian"}, "only for a time document")
}

// X-DOC-02: update_document sets the tab rank and full width.
func TestUpdateDocument_X_DOC_02_RankAndWidth(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(dtDoc))
	wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "d1", "rank": "0", "is_full_width": true})
	wtReq(t, h.writes()[0], http.MethodPatch, wtPath+"/r1/docs/d1", "", `{"rank":"0","isFullWidth":true}`)
}

// X-DOC-03: delete_document deletes; its dry run sends nothing.
func TestDeleteDocument_X_DOC_03(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes))
	if got := wtMustOK(t, h, "delete_document", map[string]any{"resource_id": "r1", "document_id": "d1", "dry_run": true}); !strings.Contains(got, `would delete page document "Main" (d1)`) {
		t.Errorf("text %q", got)
	}
	h.lk.Enqueue(wtOK(wtRes), noContentReply())
	wtMustOK(t, h, "delete_document", map[string]any{"resource_id": "r1", "document_id": "d1"})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodDelete, wtPath+"/r1/docs/d1", "", "")
}

// X-DOC-03, X-GUARD-14: restore_document with the guard on restores, hides,
// checks and shows again; a refusal leaves it hidden; with no terms file it
// only restores.
func TestRestoreDocument_X_GUARD_14(t *testing.T) {
	restored := `{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false}`
	h := newHarness(t)
	h.lk.Enqueue(wtOK(restored), wtOK(wtResPages), wtOK(dtDoc), wtOK(blocksJSON), wtOK(dtDoc))
	if got := wtMustOK(t, h, "restore_document", map[string]any{"resource_id": "r1", "document_id": "d1"}); !strings.Contains(got, "visible to players (checked") {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 3 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/r1/docs/d1/restore", "", "")
	wtReq(t, w[1], http.MethodPatch, wtPath+"/r1/docs/d1", "", `{"isHidden":true}`)
	wtReq(t, w[2], http.MethodPatch, wtPath+"/r1/docs/d1", "", `{"isHidden":false}`)

	h = newHarness(t)
	h.guard.terms = []string{"Hi"}
	h.lk.Enqueue(wtOK(restored), wtOK(wtResPages), wtOK(dtDoc), wtOK(blocksJSON))
	got := wtMustFail(t, h, "restore_document", map[string]any{"resource_id": "r1", "document_id": "d1"}, "left it hidden")
	if !strings.Contains(got, "spoiler guard") || len(h.writes()) != 2 {
		t.Errorf("text %q, writes %+v", got, h.writes())
	}

	h = newHarness(t, noGuard())
	h.lk.Enqueue(wtOK(restored))
	wtMustOK(t, h, "restore_document", map[string]any{"resource_id": "r1", "document_id": "d1"})
	if len(h.writes()) != 1 {
		t.Errorf("writes %+v", h.writes())
	}
}

// X-DOC-04: assign_block_ids posts to block-ids and reports the count.
func TestAssignBlockIDs_X_DOC_04(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"assigned":3,"version":"v9"}`))
	if got := wtMustOK(t, h, "assign_block_ids", map[string]any{"resource_id": "r1", "document_id": "d1"}); !strings.Contains(got, "Gave ids to 3 block(s)") {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.writes()[0], http.MethodPost, wtPath+"/r1/docs/d1/block-ids", "", "")
}

func noContentReply() testutil.Reply { return testutil.Reply{Status: http.StatusNoContent} }
