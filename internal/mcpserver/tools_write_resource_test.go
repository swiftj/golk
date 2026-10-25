package mcpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// Fixtures of the write-tool tests. All names and terms are made up.
const (
	wtRes = `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"d2","name":"DM Notes","type":"page","rank":"b","isHidden":true},
{"id":"m1","name":"Map","type":"map","rank":"c"}]}`
	// wtResPages is wtRes without its map document, which the guard can't
	// read and so won't show.
	wtResPages = `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"d2","name":"DM Notes","type":"page","rank":"b","isHidden":true}]}`
	wtResHidden = `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"d2","name":"DM Notes","type":"page","rank":"b","isHidden":true}]}`
	wtNoProps = `{"data":[],"hasMore":false}`
	wtNoKids  = `{"data":[],"hasMore":false,"nextCursor":null}`
	wtPath    = "/api/v3/projects/p1/resources"
)

func wtOK(body string) testutil.Reply { return testutil.JSON(http.StatusOK, body) }

// wtReq asserts one recorded request: method, path, query and body (JSON
// compared after normalising; "" means no body).
func wtReq(t *testing.T, r testutil.Recorded, method, path, query, body string) {
	t.Helper()
	if r.Method != method || r.Path != path || r.RawQuery != query {
		t.Errorf("request %s %s?%s, want %s %s?%s", r.Method, r.Path, r.RawQuery, method, path, query)
	}
	if got, want := wtNorm(t, r.Body), wtNorm(t, []byte(body)); got != want {
		t.Errorf("%s %s body\n%s\nwant\n%s", method, path, got, want)
	}
	if method == http.MethodPost && r.Header.Get("Idempotency-Key") == "" {
		t.Errorf("%s %s has no Idempotency-Key", method, path)
	}
}

func wtNorm(t *testing.T, b []byte) string {
	t.Helper()
	if len(bytes.TrimSpace(b)) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("body %q: %v", b, err)
	}
	return string(mustJSON(v))
}

// wtRefused asserts a spoiler-guard refusal that sent no write.
func wtRefused(t *testing.T, h *harness, res string) {
	t.Helper()
	if !strings.Contains(res, "spoiler guard") {
		t.Errorf("want a guard refusal, got: %s", res)
	}
	if len(h.writes()) != 0 {
		t.Errorf("a refused write sent %+v", h.writes())
	}
}

func wtMustOK(t *testing.T, h *harness, name string, args map[string]any) string {
	t.Helper()
	r := h.call(name, args)
	if r.IsError {
		t.Fatalf("%s: %s", name, text(r))
	}
	return text(r)
}

func wtMustFail(t *testing.T, h *harness, name string, args map[string]any, want string) string {
	t.Helper()
	r := h.call(name, args)
	if !r.IsError || !strings.Contains(text(r), want) {
		t.Fatalf("%s: want an error containing %q, got %v: %s", name, want, r.IsError, text(r))
	}
	return text(r)
}

func TestCreateResource_X_TOOL_03_SendsOnlyGivenFields(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusCreated, wtRes))
	wtMustOK(t, h, "create_resource", map[string]any{"name": "Ooo", "parent_id": "r0", "tags": []string{"land"},
		"page_markdown": "Hello **there**\n", "page_name": "Lore"})
	reqs := h.lk.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests %+v", reqs)
	}
	wtReq(t, reqs[0], "POST", wtPath, "", `{"name":"Ooo","parentId":"r0","tags":["land"],"documents":[{"type":"page","name":"Lore",
"blocks":[{"kind":"paragraph","content":[{"text":"Hello "},{"text":"there","bold":true}]}]}]}`)
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], `"blocks"`) || !strings.Contains(h.guard.checked[0], `"Ooo"`) {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestCreateResource_X_GUARD_07_RefusalSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	r := h.call("create_resource", map[string]any{"name": "Ooo", "aliases": []string{"Zorblax"}})
	wtRefused(t, h, text(r))
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

func TestCreateResource_X_GUARD_04_HiddenIsNotChecked(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(testutil.JSON(http.StatusCreated, wtResHidden))
	wtMustOK(t, h, "create_resource", map[string]any{"name": "Zorblax", "is_hidden": true})
	wtReq(t, h.lk.Requests()[0], "POST", wtPath, "", `{"name":"Zorblax","isHidden":true}`)
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestCreateResource_X_TOOL_04_DryRunSendsNothing(t *testing.T) {
	h := newHarness(t)
	got := wtMustOK(t, h, "create_resource", map[string]any{"name": "Ooo", "page_markdown": "Hi\n", "dry_run": true})
	if !strings.HasPrefix(got, "Dry run: would create resource \"Ooo\"") || !strings.HasSuffix(got, "Nothing was written.") {
		t.Errorf("text %q", got)
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

func TestUpdateResource_X_TOOL_03_FieldsThenMove(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtRes), wtOK(wtRes))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "name": "Ooo", "tags": []string{"x"}, "parent_id": "r9"})
	w := h.writes()
	if len(w) != 2 {
		t.Fatalf("writes %+v", w)
	}
	// The name is unchanged, so only the tags are sent.
	wtReq(t, w[0], "PATCH", wtPath+"/r1", "", `{"tags":["x"]}`)
	wtReq(t, w[1], "POST", wtPath+"/r1/move", "", `{"parentId":"r9"}`)
	if len(h.guard.checked) != 1 || h.guard.checked[0] != `{"tags":["x"]}` {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestUpdateResource_X_TOOL_03_MoveToRootSendsNull(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(strings.Replace(wtRes, `"isHidden":false,`, `"isHidden":false,"parentId":"r0",`, 1)), wtOK(wtRes))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "move_to_root": true})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", wtPath+"/r1/move", "", `{"parentId":null}`)
	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "move_to_root": true, "parent_id": "r2"}, "not both")
}

func TestUpdateResource_X_GUARD_04_StaysHiddenNotChecked(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(wtResHidden), wtOK(wtResHidden))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "name": "Zorblax"})
	wtReq(t, h.writes()[0], "PATCH", wtPath+"/r1", "", `{"name":"Zorblax"}`)
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestUpdateResource_X_GUARD_05_UnhideScansEverything(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	props := `{"data":[{"kind":"textField","id":"q1","title":"Lore","children":[{"kind":"paragraph","content":[{"text":"Zorblax"}]}]}],"hasMore":false}`
	h.lk.Enqueue(wtOK(wtResHidden), wtOK(blocksJSON), wtOK(props))
	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "is_hidden": false}, "spoiler guard")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
	reqs := h.lk.Requests()
	// The visible page is read; the hidden document d2 is not.
	if len(reqs) != 3 || reqs[1].Path != wtPath+"/r1/docs/d1/blocks" || reqs[2].Path != wtPath+"/r1/properties" {
		t.Errorf("requests %+v", reqs)
	}
	c := h.guard.checked
	if len(c) != 1 || !strings.Contains(c[0], `"Hi"`) || !strings.Contains(c[0], `"Lore"`) || !strings.Contains(c[0], `"name":"Ooo"`) || strings.Contains(c[0], "DM Notes") {
		t.Errorf("guard checked %v", c)
	}
}

func TestUpdateResource_X_TOOL_04_DryRunUnhide(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtResHidden), wtOK(blocksJSON), wtOK(wtNoProps))
	got := wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "is_hidden": false, "dry_run": true})
	if got != `Dry run: would show it to players (resource "Ooo", r1). Nothing was written.` {
		t.Errorf("text %q", got)
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestDeleteResource_X_TOOL_08_ChildrenNeedCascade(t *testing.T) {
	h := newHarness(t)
	kids := `{"data":[{"id":"k1","name":"Kid","isHidden":false}],"hasMore":true,"nextCursor":"c2"}`
	h.lk.Enqueue(wtOK(wtRes), wtOK(kids))
	wtMustFail(t, h, "delete_resource", map[string]any{"id": "r1", "dry_run": true}, "at least 1 child resource(s); nothing was deleted")
	h.lk.Enqueue(wtOK(wtRes), wtOK(kids))
	got := wtMustOK(t, h, "delete_resource", map[string]any{"id": "r1", "cascade": true, "dry_run": true})
	if got != `Dry run: would delete resource "Ooo" (r1) with 3 document(s) and at least 1 child resource(s) and their descendants. Nothing was written.` {
		t.Errorf("text %q", got)
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
	if q := h.lk.Requests()[1].RawQuery; q != "limit=100&parentId=r1" {
		t.Errorf("list query %q", q)
	}
}

func TestDeleteResource_X_TOOL_03_Cascade(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtNoKids), testutil.Reply{Status: http.StatusNoContent})
	wtMustOK(t, h, "delete_resource", map[string]any{"id": "r1", "cascade": true})
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "DELETE", wtPath+"/r1", "cascade=true", "")
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestRestoreResource_X_TOOL_08_CleanRestore(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), // restore
		wtOK(wtRes),                                         // hide r1
		wtOK(wtNoKids),                                      // r1 has no children
		wtOK(wtResPages), wtOK(blocksJSON), wtOK(wtNoProps), // recheck r1
		wtOK(wtRes)) // show r1 again
	got := wtMustOK(t, h, "restore_resource", map[string]any{"id": "r1"})
	if !strings.Contains(got, `Restored resource "Ooo" (r1)`) || !strings.Contains(got, "checked 1 visible resource(s)") {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 3 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", wtPath+"/r1/restore", "", "")
	wtReq(t, w[1], "PATCH", wtPath+"/r1", "", `{"isHidden":true}`)
	wtReq(t, w[2], "PATCH", wtPath+"/r1", "", `{"isHidden":false}`)
}

// Every visible resource of the restored tree is hidden before the guard
// reads it, and a flagged one stays hidden.
func TestRestoreResource_X_GUARD_05_HiddenWhileChecked(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	kids := `{"data":[{"id":"k1","name":"Kid","isHidden":false}],"hasMore":false,"nextCursor":null}`
	kid := `{"id":"k1","name":"Zorblax Kid","isHidden":true,"parentId":"r1","documents":[]}`
	h.lk.Enqueue(wtOK(wtRes),
		wtOK(wtRes), wtOK(kids), wtOK(kid), wtOK(wtNoKids), // hide r1, list, hide k1, list k1
		wtOK(wtResPages), wtOK(blocksJSON), wtOK(wtNoProps), wtOK(wtRes), // r1 clean: shown again
		wtOK(kid), wtOK(wtNoProps)) // k1 flagged: stays hidden
	got := wtMustFail(t, h, "restore_resource", map[string]any{"id": "r1"}, "stay hidden from players")
	if !strings.Contains(got, `"Kid" (k1)`) || strings.Contains(got, `(r1):`) {
		t.Errorf("text %q", got)
	}
	w := h.writes()
	if len(w) != 4 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", wtPath+"/r1/restore", "", "")
	wtReq(t, w[1], "PATCH", wtPath+"/r1", "", `{"isHidden":true}`)
	wtReq(t, w[2], "PATCH", wtPath+"/k1", "", `{"isHidden":true}`)
	wtReq(t, w[3], "PATCH", wtPath+"/r1", "", `{"isHidden":false}`)
	// The guard read nothing before both were hidden.
	if n := len(h.lk.Requests()); n != 11 {
		t.Errorf("%d requests", n)
	}
}

func TestRestoreResource_X_TOOL_04_DryRunAndNoGuard(t *testing.T) {
	h := newHarness(t)
	got := wtMustOK(t, h, "restore_resource", map[string]any{"id": "r1", "dry_run": true})
	if !strings.HasPrefix(got, "Dry run: would restore resource r1") || !strings.HasSuffix(got, "Nothing was written.") {
		t.Errorf("text %q", got)
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests sent")
	}
	// Without a terms file a restore is not hidden and checked (D-017).
	ng := newHarness(t, noGuard())
	ng.lk.Enqueue(wtOK(`{"id":"r1","name":"Ooo","isHidden":false,"documents":[]}`))
	if got := wtMustOK(t, ng, "restore_resource", map[string]any{"id": "r1"}); !strings.Contains(got, `Restored resource "Ooo" (r1)`) || len(ng.writes()) != 1 {
		t.Errorf("text %q, writes %+v", got, ng.writes())
	}
}
