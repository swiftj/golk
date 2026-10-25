//go:build integration

package examples

import (
	"encoding/json"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// Port of the guide's §boards example.

func props(kv ...string) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = json.RawMessage(kv[i+1])
	}
	return m
}

// TestBoards ports "boards".
func TestBoards(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	board := page(ooo)

	empty, err := c.Boards.GetContent(ctx, board)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(empty.Pages), "pages,", len(empty.Shapes), "shapes")

	created, err := c.Boards.CreateShapes(ctx, board, lk.BoardRecords{
		Shapes: []lk.BoardShapeInput{
			{ID: lk.Some("shape:candy-kingdom"), Type: "geo", X: lk.Some(0.0), Y: lk.Some(0.0),
				Props: props("w", "220", "h", "120", "text", `"Candy Kingdom"`, "color", `"violet"`, "fill", `"solid"`)},
			{ID: lk.Some("shape:ice-kingdom"), Type: "geo", X: lk.Some(480.0), Y: lk.Some(0.0),
				Props: props("w", "220", "h", "120", "text", `"Ice Kingdom"`, "color", `"light-blue"`, "fill", `"solid"`)},
			{ID: lk.Some("shape:rivalry"), Type: "arrow", Props: props("text", `"kidnaps princesses from"`)},
			{Type: "note", X: lk.Some(0.0), Y: lk.Some(220.0), Props: props("text", `"Gunter was last seen here"`)},
		},
		Bindings: []lk.BoardBindingInput{
			{Type: "arrow", FromID: "shape:rivalry", ToID: "shape:ice-kingdom", Props: props("terminal", `"start"`)},
			{Type: "arrow", FromID: "shape:rivalry", ToID: "shape:candy-kingdom", Props: props("terminal", `"end"`)},
		},
	}, lk.IfMatch(empty.Version))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(created.Shapes), "shapes and", len(created.Bindings), "bindings")
	if len(created.Shapes) != 4 || len(created.Bindings) != 2 {
		t.Errorf("created = %+v", created)
	}

	updated, err := c.Boards.UpdateShapes(ctx, board, []lk.BoardShapePatch{
		{ID: "shape:ice-kingdom", Y: lk.Some(60.0), Props: props("color", `"blue"`)},
	}, lk.IfMatch(created.Version))
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := c.Boards.DeleteShapes(ctx, board, []string{"shape:rivalry"}, lk.IfMatch(updated.Version))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("deleted", deleted.ShapeIDs, "and", len(deleted.BindingIDs), "bindings")
	if len(deleted.BindingIDs) != 2 {
		t.Errorf("deleted = %+v, want the arrow's two bindings", deleted)
	}

	shapes, err := c.Boards.ListShapes(ctx, board, lk.BoardShapeListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shapes.Items {
		t.Logf("%s at %v,%v", s.Type, s.X, s.Y)
		if s.ID == "shape:ice-kingdom" && (s.Y != 60 || string(s.Props["color"]) != `"blue"` || string(s.Props["text"]) != `"Ice Kingdom"`) {
			t.Errorf("ice kingdom = %+v; props merge one level deep", s)
		}
	}

	replaced, err := c.Boards.ReplaceContent(ctx, board, lk.BoardRecords{
		Shapes: []lk.BoardShapeInput{{Type: "text", X: lk.Some(0.0), Y: lk.Some(0.0), Props: props("text", `"The Land of Ooo"`)}},
	}, lk.IfMatch(deleted.Version))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(replaced.Shapes), "shape after the replace")
	if len(replaced.Shapes) != 1 || len(replaced.Pages) != 1 {
		t.Errorf("replaced = %+v", replaced)
	}
}
