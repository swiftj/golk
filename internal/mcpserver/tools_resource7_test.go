package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

// X-RES-01: update_resource reorders a resource among its siblings with
// after or before (moveResource), keeping its parent.
func TestUpdateResource_X_RES_01_Position(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtRes))
	got := wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "after": "r7"})
	if !strings.Contains(got, "place it after r7") {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/r1/move", "", `{"after":"r7"}`)

	h.lk.Enqueue(wtOK(wtRes), wtOK(wtRes))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "parent_id": "r9", "before": "r8"})
	wtReq(t, h.writes()[1], http.MethodPost, wtPath+"/r1/move", "", `{"parentId":"r9","before":"r8"}`)

	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "after": "r7", "before": "r8"}, "not both")
}

// X-RES-02: icons, lock, banner and permissions are sent as given; the
// banner keeps the fields not given.
func TestUpdateResource_X_RES_02_Fields(t *testing.T) {
	h := newHarness(t)
	res := strings.Replace(wtRes, `"isHidden":false,`, `"isHidden":false,"banner":{"enabled":true,"url":"https://assets.example/b.png","yPosition":0.3},`, 1)
	h.lk.Enqueue(wtOK(res), wtOK(res))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "icon_glyph": "castle", "icon_shape": "shield", "is_locked": true,
		"banner": map[string]any{"y_position": 0.6}, "permissions": map[string]any{"ADMIN": 3}})
	wtReq(t, h.writes()[0], http.MethodPatch, wtPath+"/r1", "",
		`{"iconGlyph":"castle","iconShape":"shield","isLocked":true,"banner":{"enabled":true,"url":"https://assets.example/b.png","yPosition":0.6},"permissions":{"ADMIN":3}}`)
	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "permissions": map[string]any{"MEMBER": 9}}, "a level is 0 to 5")
}

// X-RES-02, X-GUARD-13: giving players (MEMBER) access checks everything
// the resource would show, like unhiding; taking it away isn't checked.
func TestUpdateResource_X_GUARD_13_MemberAccess(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	props := `{"data":[{"kind":"textField","id":"q1","title":"Lore","children":[{"kind":"paragraph","content":[{"text":"Zorblax"}]}]}],"hasMore":false}`
	h.lk.Enqueue(wtOK(wtResPages), wtOK(blocksJSON), wtOK(props))
	wtRefused(t, h, wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "permissions": map[string]any{"MEMBER": 1}}, "spoiler guard"))

	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtResPages), wtOK(wtResPages))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "permissions": map[string]any{"MEMBER": 0}})
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

// X-RES-03: create_resource sends its new fields, and documents of any type.
func TestCreateResource_X_RES_03_Fields(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes))
	wtMustOK(t, h, "create_resource", map[string]any{"name": "Ooo", "parent_id": "r0", "before": "r5", "id": "abcd1234",
		"icon_glyph": "tree", "icon_size": "small", "is_locked": false, "banner": map[string]any{"url": "https://assets.example/b.png"},
		"documents": []any{
			map[string]any{"name": "Lore", "markdown": "Hi\n"},
			map[string]any{"name": "Atlas", "type": "map"},
			map[string]any{"name": "History", "type": "time", "calendar_id": "gregorian"},
		}})
	wtReq(t, h.writes()[0], http.MethodPost, wtPath, "",
		`{"id":"abcd1234","name":"Ooo","parentId":"r0","before":"r5","iconGlyph":"tree","iconSize":"small","isLocked":false,`+
			`"banner":{"enabled":true,"url":"https://assets.example/b.png","yPosition":0.5},`+
			`"documents":[{"name":"Lore","blocks":[{"kind":"paragraph","content":[{"text":"Hi"}]}]},{"type":"map","name":"Atlas"},{"type":"time","name":"History","calendarId":"gregorian"}]}`)
	wtMustFail(t, h, "create_resource", map[string]any{"name": "Ooo", "page_name": "Main", "documents": []any{map[string]any{"name": "x"}}}, "not both")
}

// X-RES-04: create_resources sends one batch; each visible item is checked,
// hidden ones aren't; a refusal sends nothing.
func TestCreateResources_X_RES_04(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"resources":[{"id":"r1","name":"Finn","documents":[]},{"id":"r2","name":"Lich","isHidden":true,"documents":[]}]}`))
	got := wtMustOK(t, h, "create_resources", map[string]any{"resources": []any{
		map[string]any{"name": "Finn", "tags": []string{"hero"}},
		map[string]any{"name": "Lich", "is_hidden": true},
	}})
	if !strings.Contains(got, `Created 2 resource(s):`) || !strings.Contains(got, `"Lich" (r2), hidden from players`) {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.writes()[0], http.MethodPost, wtPath+"/batch", "", `{"resources":[{"name":"Finn","tags":["hero"]},{"name":"Lich","isHidden":true}]}`)
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], "Finn") {
		t.Errorf("guard checked %v", h.guard.checked)
	}

	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	wtRefused(t, h, wtMustFail(t, h, "create_resources", map[string]any{"resources": []any{
		map[string]any{"name": "Finn"}, map[string]any{"name": "Zorblax"},
	}}, "spoiler guard"))
	wtMustFail(t, h, "create_resources", map[string]any{"resources": []any{}}, "1 to 100")
}

// X-RES-05: list_resources shows each sibling's rank in sidebar order, and
// get_resource shows rank, icon, banner, permissions and the tab order.
func TestReadResources_X_RES_05(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"data":[{"id":"b","name":"Beta","rank":"b","documents":[]},{"id":"a","name":"Alpha","rank":"a","documents":[]}],"hasMore":false}`))
	got := text(h.call("list_resources", map[string]any{"parent_id": "r1"}))
	if !strings.Contains(got, "- Alpha (a) rank a\n- Beta (b) rank b\n") {
		t.Errorf("text %q", got)
	}
	h.lk.Enqueue(wtOK(`{"id":"r1","name":"Ooo","rank":"c","iconGlyph":"tree","iconShape":"pin","banner":{"enabled":true,"url":"https://assets.example/b.png","yPosition":0.5},` +
		`"permissions":{"MEMBER":1,"ADMIN":3},"documents":[{"id":"d2","name":"Two","type":"page","rank":"b"},{"id":"d1","name":"One","type":"page","rank":"a","isFullWidth":true}],"properties":[]}`))
	got = text(h.call("get_resource", map[string]any{"resource_id": "r1"}))
	for _, want := range []string{"Sidebar rank: c", "Icon: glyph tree", "Banner: https://assets.example/b.png", "ADMIN=3, MEMBER=1",
		"- One (page, d1) rank a [full width]\n- Two (page, d2) rank b"} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
}
