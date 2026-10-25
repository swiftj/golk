package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

const meterMD = ":::meter mode=bar title=Vitals\n- HP :: 3/5 icon=heart color=#ff0000\n:::"

// X-PROP-01: set_property creates a property of any kind from
// property_markdown, placed with after.
func TestSetProperty_X_PROP_01_AnyKind(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps), wtOK(`{"kind":"meter","id":"q9","rank":"c","title":"Vitals","mode":"bar","items":[]}`))
	got := wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "property_markdown": meterMD, "after": "q2"})
	if !strings.Contains(got, `Created meter property "Vitals" (q9)`) {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.writes()[0], http.MethodPost, wtPath+"/r1/properties", "",
		`{"kind":"meter","title":"Vitals","mode":"bar","items":[{"name":"HP","current":3,"max":5,"icon":"heart","color":"#ff0000"}],"after":"q2"}`)
}

// X-PROP-02: property_markdown on an existing property patches its content;
// rank moves it; a different kind is refused.
func TestSetProperty_X_PROP_02_UpdateAndRank(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps), wtOK(wtPropKV))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "rank": "0",
		"property_markdown": ":::keyvalue title=Facts display=grid\n- Age :: 10\n:::"})
	wtReq(t, h.writes()[0], http.MethodPatch, wtPath+"/r1/properties/q2", "",
		`{"rank":"0","display":"grid","rows":[{"key":"Age","content":[{"text":"10"}]}]}`)

	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps))
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "property_markdown": ":::meter mode=bar title=Facts\n- HP :: 1/2 icon=heart\n:::"}, "kind can't change")
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "x", "markdown": "a", "rows": []any{}}, "give one of")
}

// X-PROP-03: with document_id and block_id, set_property and
// delete_property work on an infobox's properties, sending the page version
// read with the list as If-Match; a hidden page isn't checked.
func TestSetProperty_X_PROP_03_Infobox(t *testing.T) {
	ib := "/api/v3/projects/p1/resources/r1/docs/d1/blocks/b1/properties"
	list := `{"data":[` + wtPropKV + `],"version":"v4"}`
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(list), wtOK(`{"kind":"textField","id":"q7","title":"Lore","children":[],"version":"v5"}`))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "document_id": "d1", "block_id": "b1", "title": "Lore", "markdown": "Old tale"})
	reqs := h.lk.Requests()
	wtReq(t, reqs[1], http.MethodGet, ib, "", "")
	w := h.writes()
	wtReq(t, w[0], http.MethodPost, ib, "", `{"kind":"textField","title":"Lore","children":[{"kind":"paragraph","content":[{"text":"Old tale"}]}]}`)
	if im := w[0].Header.Get("If-Match"); im != "v4" {
		t.Errorf("If-Match %q, want v4", im)
	}
	if len(h.guard.checked) != 1 {
		t.Errorf("guard checked %v", h.guard.checked)
	}

	h.lk.Enqueue(wtOK(list), noContentReply())
	wtMustOK(t, h, "delete_property", map[string]any{"resource_id": "r1", "document_id": "d1", "block_id": "b1", "property_id": "q2"})
	w = h.writes()
	wtReq(t, w[1], http.MethodDelete, ib+"/q2", "", "")
	if im := w[1].Header.Get("If-Match"); im != "v4" {
		t.Errorf("delete If-Match %q", im)
	}

	// d2 is a hidden page: nothing on it is checked.
	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(`{"data":[],"version":"v1"}`), wtOK(`{"kind":"textField","id":"q8","title":"Zorblax","children":[]}`))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "document_id": "d2", "block_id": "b2", "title": "Zorblax", "markdown": "x"})
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "block_id": "b2", "title": "x", "markdown": "y"}, "together")
}
