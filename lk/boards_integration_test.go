//go:build integration

package lk_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestBoardsIntegration covers C-BOARD-01 to C-BOARD-07 against the
// sandbox: a blank document reads as an empty board, the first write makes
// it a board, a stale If-Match is refused, and a board replaced with its
// own records is unchanged.
func TestBoardsIntegration_C_BOARD_06(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	r := create(t, c, lk.ResourceCreate{Name: sandbox.Name("Board")})
	doc := lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}

	empty, err := c.Boards.GetContent(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Shapes) != 0 || empty.Version == "" {
		t.Fatalf("empty board = %+v", empty)
	}
	text := func(s string) map[string]json.RawMessage {
		b, _ := json.Marshal(s)
		return map[string]json.RawMessage{"text": b}
	}
	created, err := c.Boards.CreateShapes(ctx, doc, lk.BoardRecords{Shapes: []lk.BoardShapeInput{
		{ID: lk.Some("shape:frame"), Type: "frame", Props: map[string]json.RawMessage{"w": json.RawMessage("400"), "h": json.RawMessage("300")}},
		{ID: lk.Some("shape:inside"), Type: "note", ParentID: lk.Some("shape:frame"), Props: text("Inside")},
		{ID: lk.Some("shape:outside"), Type: "text", X: lk.Some(600.0), Props: text("Outside")},
	}}, lk.IfMatch(empty.Version))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.Documents.Get(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != lk.DocumentBoard {
		t.Errorf("document type = %s after the first create, want board", d.Type)
	}

	_, err = c.Boards.UpdateShapes(ctx, doc, []lk.BoardShapePatch{{ID: "shape:outside", X: lk.Some(1.0)}}, lk.IfMatch(empty.Version))
	var ae *lk.APIError
	if !errors.Is(err, lk.ErrVersionMismatch) || !errors.As(err, &ae) || ae.CurrentVersion != created.Version {
		t.Fatalf("stale update: err = %v, want ErrVersionMismatch with CurrentVersion %s", err, created.Version)
	}

	before, err := c.Boards.GetContent(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	var rec lk.BoardRecords
	for _, s := range before.Shapes {
		rec.Shapes = append(rec.Shapes, s.Input())
	}
	after, err := c.Boards.ReplaceContent(ctx, doc, rec, lk.IfMatch(before.Version))
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(before.Shapes)
	b2, _ := json.Marshal(after.Shapes)
	if string(b1) != string(b2) {
		t.Errorf("shapes changed by replacing them with themselves:\nbefore %s\nafter  %s", b1, b2)
	}

	del, err := c.Boards.DeleteShapes(ctx, doc, []string{"shape:frame"}, lk.IfMatch(after.Version))
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ShapeIDs) != 2 {
		t.Errorf("deleted %v, want the frame and the note inside it", del.ShapeIDs)
	}
	left, err := c.Boards.ListShapes(ctx, doc, lk.BoardShapeListParams{PageID: before.Pages[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(left.Items) != 1 || left.Items[0].ID != "shape:outside" || left.Version != del.Version {
		t.Errorf("left = %+v (version %s, want %s)", left.Items, left.Version, del.Version)
	}
}
