package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const (
	wtPropText = `{"kind":"textField","id":"q1","rank":"a","title":"Lore","isHidden":true,"children":[{"kind":"paragraph","id":"x1","content":[{"text":"Zorblax"}]}]}`
	wtPropKV   = `{"kind":"keyValue","id":"q2","rank":"b","title":"Facts","rows":[{"id":"w1","key":"Age","content":[{"text":"9"}]}]}`
	wtProps    = `{"data":[` + wtPropText + `,` + wtPropKV + `],"hasMore":false}`
)

func TestSetProperty_X_TOOL_06_CreatesTextField(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtNoProps), testutil.JSON(http.StatusCreated, `{"kind":"textField","id":"q9","title":"Bio","children":[]}`))
	got := wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Bio", "markdown": "Brave\n"})
	if !strings.Contains(got, `Created textField property "Bio" (q9)`) {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", wtPath+"/r1/properties", "", `{"kind":"textField","title":"Bio","children":[{"kind":"paragraph","content":[{"text":"Brave"}]}]}`)
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], `"Brave"`) {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestSetProperty_X_TOOL_06_UpdatesRowsOnly(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps), wtOK(wtPropKV))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts",
		"rows": []map[string]string{{"key": "Age", "value": "*ten*"}, {"key": "Home", "value": ""}}})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "PATCH", wtPath+"/r1/properties/q2", "", `{"rows":[{"key":"Age","content":[{"text":"ten","italic":true}]},{"key":"Home","content":[]}]}`)
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts", "rows": []map[string]string{{"key": "A", "value": "one\n\ntwo"}}}, "one paragraph")
}

func TestSetProperty_X_TOOL_06_DuplicateTitleAndKindChange(t *testing.T) {
	h := newHarness(t)
	dup := `{"data":[` + wtPropKV + `,` + strings.Replace(wtPropKV, `"q2"`, `"q3"`, 1) + `],"hasMore":false}`
	h.lk.Enqueue(wtOK(wtRes), wtOK(dup))
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts", "markdown": "x"}, "2 properties titled \"Facts\" (q2, q3)")
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps))
	wtMustFail(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts", "markdown": "x"}, "is a keyValue property, not a text field")
	// Title matching is exact: "facts" is a new property.
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "facts", "markdown": "x", "dry_run": true})
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestSetProperty_X_GUARD_05_UnhideChecksContent(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps))
	r := h.call("set_property", map[string]any{"resource_id": "r1", "title": "Lore", "is_hidden": false})
	wtRefused(t, h, text(r))
	c := h.guard.checked
	if len(c) != 1 || !strings.Contains(c[0], `"isHidden":false`) || !strings.Contains(c[0], "Zorblax") {
		t.Errorf("guard checked %v", c)
	}
}

func TestSetProperty_X_TOOL_04_DryRunAndX_TOOL_03_HideOnly(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps))
	got := wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts", "is_hidden": true, "dry_run": true})
	if got != `Dry run: would update keyValue property "Facts" (q2): hide it from players on "Ooo" (r1). Nothing was written.` {
		t.Errorf("text %q", got)
	}
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtProps), wtOK(wtPropKV))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Facts", "is_hidden": true})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "PATCH", wtPath+"/r1/properties/q2", "", `{"isHidden":true}`)
}

func TestSetProperty_X_GUARD_04_HiddenResourceNotChecked(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtResHidden), wtOK(wtProps), wtOK(wtPropText))
	wtMustOK(t, h, "set_property", map[string]any{"resource_id": "r1", "title": "Lore", "is_hidden": false})
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestDeleteProperty_X_TOOL_06_ByTitleAndID(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtProps))
	got := wtMustOK(t, h, "delete_property", map[string]any{"resource_id": "r1", "title": "Facts", "dry_run": true})
	if got != `Dry run: would delete keyValue property "Facts" (q2) of resource r1. Nothing was written.` {
		t.Errorf("text %q", got)
	}
	if len(h.writes()) != 0 {
		t.Errorf("dry run writes %+v", h.writes())
	}
	h.lk.Enqueue(wtOK(wtProps), testutil.Reply{Status: http.StatusNoContent})
	wtMustOK(t, h, "delete_property", map[string]any{"resource_id": "r1", "property_id": "q1"})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "DELETE", wtPath+"/r1/properties/q1", "", "")
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
	wtMustFail(t, h, "delete_property", map[string]any{"resource_id": "r1", "title": "a", "property_id": "b"}, "one of them")
}
