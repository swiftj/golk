package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

const (
	rowList  = `{"data":[{"id":"w1","rank":"a","key":"Age","content":[{"text":"9"}]},{"id":"w2","rank":"b","key":"Home","content":[{"text":"Fort"}]}],"version":"rv1"}`
	rowReply = `{"id":"w3","key":"Pet","content":[{"text":"Gunter"}],"version":"rv2"}`
)

// X-ROW-01: set_row adds a row to a sidebar property (placed with after),
// with the rows' version as If-Match; the row is checked.
func TestSetRow_X_ROW_01_SidebarCreate(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(rowList), wtOK(wtRes), wtOK(wtPropKV), wtOK(rowReply))
	got := wtMustOK(t, h, "set_row", map[string]any{"resource_id": "r1", "property_id": "q2", "key": "Pet", "value": "**Gunter**", "after": "w1"})
	if !strings.Contains(got, `Added row "Pet" (w3) in sidebar property q2`) {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[0], http.MethodGet, wtPath+"/r1/properties/q2/rows", "", "")
	w := h.writes()
	wtReq(t, w[0], http.MethodPost, wtPath+"/r1/properties/q2/rows", "", `{"key":"Pet","content":[{"text":"Gunter","bold":true}],"after":"w1"}`)
	if w[0].Header.Get("If-Match") != "rv1" {
		t.Errorf("If-Match %q", w[0].Header.Get("If-Match"))
	}
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], "Gunter") {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

// X-ROW-02: set_row matches by key and patches only what is given, on a
// keyValue block and on an infobox property.
func TestSetRow_X_ROW_02_UpdateBlockAndInfobox(t *testing.T) {
	blk := "/api/v3/projects/p1/resources/r1/docs/d1/blocks/b1"
	h := newHarness(t)
	h.lk.Enqueue(wtOK(rowList), wtOK(wtRes), wtOK(`{"id":"w2","key":"House","content":[{"text":"Fort"}],"version":"rv2"}`))
	wtMustOK(t, h, "set_row", map[string]any{"resource_id": "r1", "document_id": "d1", "block_id": "b1", "key": "Home", "new_key": "House"})
	wtReq(t, h.writes()[0], http.MethodPatch, blk+"/rows/w2", "", `{"key":"House"}`)

	h = newHarness(t)
	h.lk.Enqueue(wtOK(rowList), wtOK(wtRes), wtOK(`{"kind":"keyValue","id":"q5","title":"Stats","rows":[],"version":"v3"}`), wtOK(`{"id":"w1","key":"Age","content":[{"text":"10"}],"version":"rv2"}`))
	wtMustOK(t, h, "set_row", map[string]any{"resource_id": "r1", "document_id": "d1", "block_id": "b1", "property_id": "q5", "row_id": "w1", "value": "10"})
	wtReq(t, h.writes()[0], http.MethodPatch, blk+"/properties/q5/rows/w1", "", `{"content":[{"text":"10"}]}`)
}

// X-ROW-03: delete_row deletes by key or id; a hidden property's rows
// aren't checked on set.
func TestDeleteRow_X_ROW_03(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(rowList), noContentReply())
	wtMustOK(t, h, "delete_row", map[string]any{"resource_id": "r1", "property_id": "q2", "key": "Age"})
	w := h.writes()
	wtReq(t, w[0], http.MethodDelete, wtPath+"/r1/properties/q2/rows/w1", "", "")
	if w[0].Header.Get("If-Match") != "rv1" {
		t.Errorf("If-Match %q", w[0].Header.Get("If-Match"))
	}
	wtMustFail(t, h, "delete_row", map[string]any{"resource_id": "r1", "property_id": "q2"}, "row_id or key")

	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(rowList), wtOK(wtRes), wtOK(wtPropText), wtOK(rowReply))
	wtMustOK(t, h, "set_row", map[string]any{"resource_id": "r1", "property_id": "q1", "key": "Zorblax", "value": "x"})
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v; q1 is hidden", h.guard.checked)
	}
}
