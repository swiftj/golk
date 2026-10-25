package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

// Fixtures of the board tool tests. All names and terms are made up.
const (
	btRes = `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"w1","name":"Rivalries","type":"board","rank":"b","isHidden":false},
{"id":"w2","name":"DM Board","type":"board","rank":"c","isHidden":true},
{"id":"b1","name":"Empty","type":"blank","rank":"d","isHidden":false}]}`
	btResHidden = `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"w1","name":"Rivalries","type":"board","rank":"b","isHidden":false}]}`
	btShape = `{"id":"shape:candy","typeName":"shape","type":"geo","x":0,"y":0,"rotation":0,"index":"a1","parentId":"page:page","isLocked":false,"opacity":1,"props":{"text":"Candy Kingdom","w":220},"meta":{}}`
	btBoard = `{"schema":{"schemaVersion":2,"sequences":{}},
"pages":[{"id":"page:page","typeName":"page","name":"Page 1","index":"a1","meta":{}}],
"shapes":[` + btShape + `,
{"id":"shape:ice","typeName":"shape","type":"geo","x":480,"y":0,"rotation":0,"index":"a2","parentId":"page:page","isLocked":true,"opacity":1,"props":{"text":"Ice Kingdom"},"meta":{}},
{"id":"shape:frame","typeName":"shape","type":"frame","x":0,"y":300,"rotation":0,"index":"a3","parentId":"page:page","isLocked":false,"opacity":1,"props":{"name":"Lairs"},"meta":{}},
{"id":"shape:cave","typeName":"shape","type":"note","x":10,"y":10,"rotation":0,"index":"a1","parentId":"shape:frame","isLocked":false,"opacity":1,"props":{"text":"Cave"},"meta":{}},
{"id":"shape:arrow","typeName":"shape","type":"arrow","x":0,"y":0,"rotation":0,"index":"a4","parentId":"page:page","isLocked":false,"opacity":1,"props":{"text":"kidnaps"},"meta":{}}],
"bindings":[
{"id":"binding:s","typeName":"binding","type":"arrow","fromId":"shape:arrow","toId":"shape:ice","props":{"terminal":"start"},"meta":{}},
{"id":"binding:e","typeName":"binding","type":"arrow","fromId":"shape:arrow","toId":"shape:candy","props":{"terminal":"end"},"meta":{}}],
"assets":[],"version":"bv1"}`
	btPath = "/api/v3/projects/p1/resources/r1/docs/w1"
)

func TestReadBoard_X_BOARD_01(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(btRes), wtOK(btBoard))
	r := h.call("read_board", map[string]any{"resource_id": "r1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	wtReq(t, h.lk.Requests()[1], http.MethodGet, btPath+"/board", "", "")
	got := text(r)
	for _, want := range []string{
		`Board "Rivalries" (w1) of "Ooo" (r1), version bv1: 5 shape(s), 2 binding(s), 0 asset(s).`,
		`Page "Page 1" (page:page)`,
		`- geo shape:candy at (0, 0) text "Candy Kingdom"` + "\n",
		`- geo shape:ice at (480, 0) text "Ice Kingdom" [locked]`,
		`- frame shape:frame at (0, 300) name "Lairs"`,
		`- note shape:cave at (10, 10) text "Cave" in shape:frame`,
		`- arrow shape:arrow at (0, 0) text "kidnaps" from shape:ice to shape:candy`,
		"Pass version bv1 to the next board write.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Records:") {
		t.Errorf("records without raw:\n%s", got)
	}
	var out BoardOut
	structured(t, r, &out)
	if out.DocumentID != "w1" || out.Version != "bv1" || out.Shapes != 5 || out.Bindings != 2 {
		t.Errorf("out = %+v", out)
	}

	h.lk.Enqueue(wtOK(btRes), wtOK(btBoard))
	r = h.call("read_board", map[string]any{"resource_id": "r1", "document_id": "w1", "raw": true})
	if r.IsError || !strings.Contains(text(r), `Records:`+"\n"+`{"pages":[{"id":"page:page","name":"Page 1","index":"a1","meta":{}}],"shapes":[{"id":"shape:candy","type":"geo"`) {
		t.Errorf("raw text:\n%s", text(r))
	}
}

func TestReadBoard_X_BOARD_01_RefusesNonBoard(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(btRes))
	if r := h.call("read_board", map[string]any{"resource_id": "r1", "document_id": "d1"}); !r.IsError || !strings.Contains(text(r), "is a page document, not a board") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
}

// X-BOARD-02: each write sends exactly the records given, with If-Match
// (and an Idempotency-Key on the POSTs).
func TestBoardWrites_X_BOARD_02(t *testing.T) {
	cases := []struct {
		tool        string
		args        map[string]any
		reply       string
		method      string
		path        string
		body        string
		keyed       bool
		wantText    string
		guardChecks bool
	}{
		{"add_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1",
			"shapes":   []any{map[string]any{"id": "shape:n", "type": "note", "x": 5, "props": map[string]any{"text": "Gunter"}}},
			"bindings": []any{map[string]any{"type": "arrow", "fromId": "shape:arrow", "toId": "shape:n", "props": map[string]any{"terminal": "end"}}}},
			`{"shapes":[{"id":"shape:n"}],"bindings":[],"assets":[],"version":"bv2"}`,
			http.MethodPost, btPath + "/board/shapes",
			`{"shapes":[{"id":"shape:n","type":"note","x":5,"props":{"text":"Gunter"}}],"bindings":[{"type":"arrow","fromId":"shape:arrow","toId":"shape:n","props":{"terminal":"end"}}]}`,
			true, "Shape ids: shape:n. New version bv2.", true},
		{"update_board_shapes", map[string]any{"resource_id": "r1", "document_id": "w1", "version": "bv1",
			"shapes": []any{map[string]any{"id": "shape:ice", "y": 60, "props": map[string]any{"color": "blue"}}}},
			`{"shapes":[],"version":"bv2"}`,
			http.MethodPatch, btPath + "/board/shapes", `{"shapes":[{"id":"shape:ice","y":60,"props":{"color":"blue"}}]}`,
			false, "Updated 1 shape(s)", true},
		{"delete_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "ids": []any{"shape:frame"}},
			`{"shapeIds":["shape:frame","shape:cave"],"bindingIds":[],"version":"bv2"}`,
			http.MethodPost, btPath + "/board/shapes/delete", `{"ids":["shape:frame"]}`,
			true, "Deleted 2 shape(s) (shape:frame, shape:cave)", false},
		{"replace_board", map[string]any{"resource_id": "r1", "version": "bv1",
			"shapes": []any{map[string]any{"typeName": "shape", "type": "text", "props": map[string]any{"text": "The Land"}}}},
			btBoard,
			http.MethodPut, btPath + "/board", `{"shapes":[{"type":"text","props":{"text":"The Land"}}]}`,
			false, "New version bv1.", true},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			h := newHarness(t)
			h.lk.Enqueue(wtOK(btRes), wtOK(tc.reply))
			got := wtMustOK(t, h, tc.tool, tc.args)
			if !strings.Contains(got, tc.wantText) {
				t.Errorf("text lacks %q: %s", tc.wantText, got)
			}
			w := h.writes()
			if len(w) != 1 {
				t.Fatalf("writes %+v", w)
			}
			wtReq(t, w[0], tc.method, tc.path, "", tc.body)
			if im := w[0].Header.Get("If-Match"); im != "bv1" {
				t.Errorf("If-Match %q", im)
			}
			if k := w[0].Header.Get("Idempotency-Key"); (k != "") != tc.keyed {
				t.Errorf("Idempotency-Key %q, want keyed %v", k, tc.keyed)
			}
			if checked := len(h.guard.checked) > 0; checked != tc.guardChecks {
				t.Errorf("guard checked %v, want %v", h.guard.checked, tc.guardChecks)
			}
		})
	}
}

func TestBoardWrites_X_BOARD_02_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
		want string
	}{
		"no version":    {"add_board_shapes", map[string]any{"resource_id": "r1", "shapes": []any{map[string]any{"type": "note"}}}, `missing properties: ["version"]`},
		"unknown field": {"add_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "shapes": []any{map[string]any{"type": "note", "colour": "red"}}}, `unknown field "colour"`},
		"empty shapes":  {"add_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "shapes": []any{}}, "nothing to add"},
		"empty version": {"add_board_shapes", map[string]any{"resource_id": "r1", "version": "", "shapes": []any{map[string]any{"type": "note"}}}, "version is required"},
		"patch no id":   {"update_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "shapes": []any{map[string]any{"x": 1}}}, "shapes[0] has no id"},
		"no ids":        {"delete_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "ids": []any{}}, "ids is required"},
		"replace nil":   {"replace_board", map[string]any{"resource_id": "r1", "version": "bv1"}, `missing properties: ["shapes"]`},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			wtMustFail(t, h, tc.tool, tc.args, tc.want)
			if n := len(h.lk.Requests()); n != 0 {
				t.Errorf("%d requests sent", n)
			}
		})
	}
	// A blank document is written only when named: it becomes a board.
	h := newHarness(t)
	h.lk.Enqueue(wtOK(btRes), wtOK(`{"shapes":[],"bindings":[],"assets":[],"version":"bv2"}`))
	wtMustOK(t, h, "add_board_shapes", map[string]any{"resource_id": "r1", "document_id": "b1", "version": "bv1", "shapes": []any{map[string]any{"type": "note"}}})
	wtReq(t, h.writes()[0], http.MethodPost, "/api/v3/projects/p1/resources/r1/docs/b1/board/shapes", "", `{"shapes":[{"type":"note"}]}`)
}

// X-BOARD-02: the destructive dry runs read the board and name what would
// go; nothing is written.
func TestBoardWrites_X_BOARD_02_DryRuns(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(btRes), wtOK(btBoard))
	got := wtMustOK(t, h, "delete_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "ids": []any{"shape:frame", "shape:ice"}, "dry_run": true})
	if !strings.Contains(got, "would delete 3 shape(s) (shape:ice, shape:frame, shape:cave) and 1 binding(s)") || strings.Contains(got, "Warning") {
		t.Errorf("text %s", got)
	}

	h.lk.Enqueue(wtOK(btRes), wtOK(btBoard))
	got = wtMustOK(t, h, "replace_board", map[string]any{"resource_id": "r1", "version": "bv0", "shapes": []any{}, "dry_run": true})
	if !strings.Contains(got, "would replace the 5 shape(s), 2 binding(s) and 0 asset(s) of board \"Rivalries\"") ||
		!strings.Contains(got, "now at version bv1, not bv0") {
		t.Errorf("text %s", got)
	}

	h.lk.Enqueue(wtOK(btRes))
	got = wtMustOK(t, h, "add_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1", "shapes": []any{map[string]any{"type": "note"}}, "dry_run": true})
	if !strings.Contains(got, "Dry run: would add 1 shape(s)") {
		t.Errorf("text %s", got)
	}
	if len(h.writes()) != 0 {
		t.Errorf("a dry run wrote %+v", h.writes())
	}
}

// X-GUARD-12: a protected term in a board write is refused unless the board
// or its resource is hidden; keys exempt elsewhere exempt nothing here.
func TestBoardGuard_X_GUARD_12_Writes(t *testing.T) {
	note := func(props map[string]any) map[string]any {
		return map[string]any{"resource_id": "r1", "version": "bv1", "shapes": []any{map[string]any{"type": "note", "props": props}}}
	}
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(btRes))
	wtRefused(t, h, wtMustFail(t, h, "add_board_shapes", note(map[string]any{"text": "Zorblax lives here"}), "spoiler guard"))

	h.lk.Enqueue(wtOK(btRes))
	wtRefused(t, h, wtMustFail(t, h, "update_board_shapes", map[string]any{"resource_id": "r1", "version": "bv1",
		"shapes": []any{map[string]any{"id": "shape:ice", "props": map[string]any{"text": "Zorblax"}}}}, "spoiler guard"))

	// A secret block inside a shape's props is still checked.
	h.lk.Enqueue(wtOK(btRes))
	wtRefused(t, h, wtMustFail(t, h, "replace_board", note(map[string]any{"blocks": []any{map[string]any{"kind": "secret", "text": "Zorblax"}}}), "spoiler guard"))
	if c := h.guard.checked[len(h.guard.checked)-1]; !strings.Contains(c, `"board:blocks"`) {
		t.Errorf("checked %s, want the blocks key renamed", c)
	}

	// The hidden board, and a board of a hidden resource, are not checked.
	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(btRes), wtOK(`{"shapes":[],"bindings":[],"assets":[],"version":"bv2"}`))
	args := note(map[string]any{"text": "Zorblax"})
	args["document_id"] = "w2"
	wtMustOK(t, h, "add_board_shapes", args)
	h.lk.Enqueue(wtOK(btResHidden), wtOK(`{"shapes":[],"bindings":[],"assets":[],"version":"bv2"}`))
	wtMustOK(t, h, "add_board_shapes", note(map[string]any{"text": "Zorblax"}))
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

// X-GUARD-12: showing a board document reads and checks the whole board;
// it is no longer refused.
func TestBoardGuard_X_GUARD_12_Unhide(t *testing.T) {
	res := `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"w1","name":"Rivalries","type":"board","rank":"b","isHidden":true}]}`
	h := newHarness(t)
	h.guard.terms = []string{"Ice Kingdom"}
	h.lk.Enqueue(wtOK(res), wtOK(btBoard))
	wtRefused(t, h, wtMustFail(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "w1", "is_hidden": false}, "spoiler guard"))
	wtReq(t, h.lk.Requests()[1], http.MethodGet, btPath+"/board", "", "")

	h = newHarness(t)
	h.lk.Enqueue(wtOK(res), wtOK(btBoard), wtOK(`{"id":"w1","name":"Rivalries","type":"board","rank":"b","isHidden":false}`))
	wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "w1", "is_hidden": false})
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], "Candy Kingdom") {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestBoardTools_X_RO_01_ReadOnlyHasOnlyReadBoard(t *testing.T) {
	h := newHarness(t, readOnlyMode())
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tool := range res.Tools {
		have[tool.Name] = true
	}
	if !have["read_board"] || have["add_board_shapes"] || have["update_board_shapes"] || have["delete_board_shapes"] || have["replace_board"] {
		t.Errorf("read-only tools %v", have)
	}
}
