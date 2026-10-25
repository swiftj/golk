//go:build integration

package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestIntegration_MCPBoards drives the board tools against the sandbox
// through MCP, with the real spoiler guard.
func TestIntegration_MCPBoards(t *testing.T) {
	cs, c, ctx := session(t), sandbox.Client(t), sandbox.Ctx(t)

	cr := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Ooo Rivalries")})
	rid, _ := cr.out["resource_id"].(string)
	if rid == "" {
		t.Fatalf("create_resource output %v", cr.out)
	}
	sandbox.Cleanup(t, rid)
	r, err := c.Resources.Get(ctx, rid)
	if err != nil || len(r.Documents) == 0 {
		t.Fatalf("resource %+v, %v", r, err)
	}
	doc := r.Documents[0].ID
	ref := lk.DocRef{ResourceID: rid, DocumentID: doc}
	empty, err := c.Boards.GetContent(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}

	// X-GUARD-12: a term on a visible board is refused; nothing is written.
	mustRefuse(t, cs, "add_board_shapes", map[string]any{"resource_id": rid, "document_id": doc, "version": empty.Version,
		"shapes": []any{map[string]any{"type": "note", "props": map[string]any{"text": "Zorblax was here"}}}}, "spoiler guard")

	// X-BOARD-02: a dry run writes nothing, then the real add.
	shapes := []any{
		map[string]any{"id": "shape:candy", "type": "geo", "x": 0, "y": 0, "props": map[string]any{"w": 220, "h": 120, "text": "Candy Kingdom"}},
		map[string]any{"id": "shape:ice", "type": "geo", "x": 480, "y": 0, "props": map[string]any{"w": 220, "h": 120, "text": "Ice Kingdom"}},
		map[string]any{"id": "shape:rivalry", "type": "arrow", "props": map[string]any{"text": "kidnaps princesses from"}},
	}
	bindings := []any{
		map[string]any{"type": "arrow", "fromId": "shape:rivalry", "toId": "shape:ice", "props": map[string]any{"terminal": "start"}},
		map[string]any{"type": "arrow", "fromId": "shape:rivalry", "toId": "shape:candy", "props": map[string]any{"terminal": "end"}},
	}
	mustOK(t, cs, "add_board_shapes", map[string]any{"resource_id": rid, "document_id": doc, "version": empty.Version, "shapes": shapes, "bindings": bindings, "dry_run": true})
	if d, err := c.Documents.Get(ctx, ref); err != nil || d.Type != lk.DocumentBlank {
		t.Fatalf("after the dry run: %+v, %v", d, err)
	}
	add := mustOK(t, cs, "add_board_shapes", map[string]any{"resource_id": rid, "document_id": doc, "version": empty.Version, "shapes": shapes, "bindings": bindings})
	v, _ := add.out["version"].(string)

	// X-BOARD-01.
	rb := mustOK(t, cs, "read_board", map[string]any{"resource_id": rid})
	if !strings.Contains(rb.text, `text "kidnaps princesses from" from shape:ice to shape:candy`) || rb.out["version"] != v || rb.out["shapes"] != float64(3) {
		t.Fatalf("read_board %v:\n%s", rb.out, rb.text)
	}

	up := mustOK(t, cs, "update_board_shapes", map[string]any{"resource_id": rid, "version": v,
		"shapes": []any{map[string]any{"id": "shape:ice", "y": 60, "props": map[string]any{"color": "blue"}}}})
	v, _ = up.out["version"].(string)
	// A stale version is refused with the hint.
	mustRefuse(t, cs, "update_board_shapes", map[string]any{"resource_id": rid, "version": empty.Version,
		"shapes": []any{map[string]any{"id": "shape:ice", "y": 70}}}, "read_board")

	dd := mustOK(t, cs, "delete_board_shapes", map[string]any{"resource_id": rid, "version": v, "ids": []any{"shape:ice"}, "dry_run": true})
	if !strings.Contains(dd.text, "would delete 1 shape(s) (shape:ice) and 1 binding(s)") {
		t.Errorf("delete dry run: %s", dd.text)
	}
	del := mustOK(t, cs, "delete_board_shapes", map[string]any{"resource_id": rid, "version": v, "ids": []any{"shape:ice"}})
	v, _ = del.out["version"].(string)

	// Replace with the board's own records, read raw, changes nothing.
	b, err := c.Boards.GetContent(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	rep := mustOK(t, cs, "replace_board", map[string]any{"resource_id": rid, "version": v, "shapes": []any{
		map[string]any{"type": "text", "props": map[string]any{"text": "The Land of Ooo"}},
	}})
	if rep.out["shapes"] != float64(1) || len(b.Shapes) != 2 {
		t.Errorf("replace %v (before: %d shapes)", rep.out, len(b.Shapes))
	}

	// X-GUARD-12: a hidden board takes the term; showing it is refused.
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": doc, "is_hidden": true})
	rv, _ := rep.out["version"].(string)
	mustOK(t, cs, "add_board_shapes", map[string]any{"resource_id": rid, "document_id": doc, "version": rv,
		"shapes": []any{map[string]any{"type": "note", "props": map[string]any{"text": "Zorblax was here"}}}})
	mustRefuse(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": doc, "is_hidden": false}, "spoiler guard")
	if d, err := c.Documents.Get(ctx, ref); err != nil || !d.IsHidden {
		t.Fatalf("the board was shown: %+v, %v", d, err)
	}
}
