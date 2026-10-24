package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// Fixtures of the template tool tests. All names and terms are made up.
const (
	ttRes = `{"id":"r1","name":"Kingdom","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"d2","name":"DM Notes","type":"page","rank":"b","isHidden":true}]}`
	ttTpl = `{"id":"t1","parentId":"templates","name":"Kingdom Template","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false}]}`
	// k1 is a hidden child named with a term; k2, its visible child, is
	// not hidden by it (hidden is not inherited).
	ttKidsR1  = `{"data":[{"id":"k1","parentId":"r1","name":"Zorblax Crypt","isHidden":true,"hasChildren":true}],"hasMore":false,"nextCursor":null}`
	ttKidsK1  = `{"data":[{"id":"k2","parentId":"k1","name":"Castle","isHidden":false}],"hasMore":false,"nextCursor":null}`
	ttK2      = `{"id":"k2","parentId":"k1","name":"Castle","isHidden":false,"documents":[{"id":"d3","name":"Main","type":"page","rank":"a","isHidden":false}]}`
	ttBlocks  = `{"id":"d1","version":"v1","schema":1,"text":"Ruler","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Ruler, capital, allies."}]}]}`
	ttBlocksZ = `{"id":"d3","version":"v1","schema":1,"text":"Zorblax","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Zorblax waits below."}]}]}`
	ttCopy    = `{"id":"t9","parentId":"templates","name":"Kingdom Template","isHidden":false,"documents":[]}`
	ttTplList = `{"data":[
{"id":"t1","parentId":"templates","name":"Kingdom Template","isHidden":false,"hasChildren":true},
{"id":"k1","parentId":"t1","name":"Zorblax Crypt","isHidden":true,"hasChildren":true},
{"id":"k2","parentId":"k1","name":"Castle","isHidden":false},
{"id":"t2","parentId":"templates","name":"Other Template","isHidden":false}],"hasMore":false,"nextCursor":null}`
)

// saveTree enqueues save_as_template's reads of r1's tree, with k2's page
// as k2blocks.
func saveTree(h *harness, k2blocks string) {
	h.lk.Enqueue(wtOK(ttRes), wtOK(ttKidsR1), wtOK(ttKidsK1), wtOK(wtNoKids),
		wtOK(ttBlocks), wtOK(wtNoProps), wtOK(ttK2), wtOK(k2blocks), wtOK(wtNoProps))
}

func TestListTemplates_X_TPL_01_Paged(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"data":[{"id":"t1","parentId":"templates","name":"Kingdom Template","hasChildren":true},
{"id":"k1","parentId":"t1","name":"Crypt","isHidden":true}],"hasMore":true,"nextCursor":"c2"}`))
	r := h.call("list_templates", map[string]any{"limit": 2, "cursor": "c1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	wtReq(t, h.lk.Requests()[0], http.MethodGet, "/api/v3/projects/p1/templates", "cursor=c1&limit=2", "")
	got := text(r)
	for _, want := range []string{"- Kingdom Template (t1) [has children]", "- Crypt (k1) [hidden] in template t1;", `pass cursor "c2"`} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	var out ListTemplatesOut
	structured(t, r, &out)
	if len(out.Templates) != 2 || out.NextCursor != "c2" || !out.HasMore {
		t.Errorf("out = %+v", out)
	}
}

func TestSaveAsTemplate_X_TPL_01_SendsOnlyGivenFields(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(ttRes), wtOK(ttBlocks), wtOK(wtNoProps), testutil.JSON(http.StatusCreated, ttCopy))
	r := h.call("save_as_template", map[string]any{"resource_id": "r1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/r1/save-as-template", "", `{}`)
	var out TemplateCopyOut
	structured(t, r, &out)
	if out.ResourceID != "t9" || out.Copied != 1 || out.Checked != 1 || out.ParentID != "templates" {
		t.Errorf("out = %+v", out)
	}
}

// X-GUARD-10: every copied resource is checked by its own flag before the
// write: the hidden child is skipped, its visible child is not.
func TestSaveAsTemplate_X_GUARD_10_ChecksTheTree(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	saveTree(h, ttBlocksZ)
	r := h.call("save_as_template", map[string]any{"resource_id": "r1", "name": "Kingdom Template", "include_children": true})
	wtRefused(t, h, text(r))
	if !strings.Contains(text(r), `"Castle" (k2)`) {
		t.Errorf("refusal doesn't name the resource: %s", text(r))
	}

	// A clean tree is saved; the hidden child named with a term is fine.
	saveTree(h, ttBlocks)
	h.lk.Enqueue(testutil.JSON(http.StatusCreated, ttCopy))
	r = h.call("save_as_template", map[string]any{"resource_id": "r1", "name": "Kingdom Template", "include_children": true})
	if r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/r1/save-as-template", "", `{"name":"Kingdom Template","includeChildren":true}`)
	reqs := h.lk.Requests()
	wtReq(t, reqs[len(reqs)-9], http.MethodGet, wtPath, "limit=100&parentId=r1", "")
	var out TemplateCopyOut
	structured(t, r, &out)
	if out.Copied != 3 || out.Checked != 2 {
		t.Errorf("out = %+v", out)
	}
}

func TestSaveAsTemplate_X_GUARD_10_NameIsChecked(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(ttRes))
	wtRefused(t, h, text(h.call("save_as_template", map[string]any{"resource_id": "r1", "name": "Zorblax Template"})))
}

func TestSaveAsTemplate_X_TOOL_04_DryRunSendsNothing(t *testing.T) {
	h := newHarness(t)
	saveTree(h, ttBlocks)
	r := h.call("save_as_template", map[string]any{"resource_id": "r1", "include_children": true, "dry_run": true})
	if r.IsError || !strings.Contains(text(r), "Dry run: would save \"Kingdom\" (r1) as template \"Kingdom\", copying 3 resource(s), 2 of them") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("dry run sent %+v", h.writes())
	}
}

func TestSaveAsTemplate_HiddenRootIsSkipped(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(strings.Replace(strings.Replace(ttRes, `"isHidden":false,"documents"`, `"isHidden":true,"documents"`, 1), "Main", "Zorblax", 1)),
		testutil.JSON(http.StatusCreated, ttCopy))
	if r := h.call("save_as_template", map[string]any{"resource_id": "r1", "name": "Kingdom Template"}); r.IsError {
		t.Fatal(text(r))
	}
}

// create_from_template walks the template tree from templates.list.
func TestCreateFromTemplate_X_TPL_01_X_GUARD_10(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	enqueue := func(k2blocks string) {
		h.lk.Enqueue(wtOK(ttTpl), wtOK(ttTplList), wtOK(ttBlocks), wtOK(wtNoProps), wtOK(ttK2), wtOK(k2blocks), wtOK(wtNoProps))
	}
	args := map[string]any{"template_id": "t1", "name": "Realm", "parent_id": "r9", "include_children": true}
	enqueue(ttBlocksZ)
	wtRefused(t, h, text(h.call("create_from_template", args)))

	enqueue(ttBlocks)
	h.lk.Enqueue(testutil.JSON(http.StatusCreated, `{"id":"n1","parentId":"r9","name":"Realm","isHidden":false,"documents":[]}`))
	r := h.call("create_from_template", args)
	if r.IsError {
		t.Fatal(text(r))
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[len(reqs)-7], http.MethodGet, "/api/v3/projects/p1/templates", "limit=100", "")
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/from-template", "", `{"templateId":"t1","name":"Realm","parentId":"r9","includeChildren":true}`)
	var out TemplateCopyOut
	structured(t, r, &out)
	if out.ResourceID != "n1" || out.ParentID != "r9" || out.Copied != 3 || out.Checked != 2 {
		t.Errorf("out = %+v", out)
	}

	// The new name is checked.
	h = rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(ttTpl))
	wtRefused(t, h, text(h.call("create_from_template", map[string]any{"template_id": "t1", "name": "Zorblax Realm"})))
}

func TestCreateFromTemplate_X_TOOL_04_DryRunAndMinimalBody(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(ttTpl), wtOK(ttBlocks), wtOK(wtNoProps))
	r := h.call("create_from_template", map[string]any{"template_id": "t1", "dry_run": true})
	if r.IsError || !strings.Contains(text(r), "Dry run: would create \"Kingdom Template\" at the top level") || len(h.writes()) != 0 {
		t.Fatalf("%v: %s; writes %+v", r.IsError, text(r), h.writes())
	}
	h.lk.Enqueue(wtOK(ttTpl), wtOK(ttBlocks), wtOK(wtNoProps), testutil.JSON(http.StatusCreated, `{"id":"n1","name":"Kingdom","isHidden":false}`))
	if r := h.call("create_from_template", map[string]any{"template_id": "t1"}); r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPost, wtPath+"/from-template", "", `{"templateId":"t1"}`)
}
