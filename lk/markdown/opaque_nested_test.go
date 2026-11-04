package markdown

import (
	"errors"
	"testing"
)

// TestNestedRemoval_M_SAFE_04 checks that removing rows, cells, list items,
// task items and layout columns is not mistaken for removing content
// Markdown can't show. Those kinds have no Markdown form of their own, so
// judged alone they used to render as lk-raw and every such removal needed
// AllowRemoveOpaque. A node that really is opaque (unmodelled fields, or an
// unsupported block inside) must still be caught.
func TestNestedRemoval_M_SAFE_04(t *testing.T) {
	cases := []struct {
		name   string
		base   string // JSON block list
		edited string // Markdown
		opaque bool   // want ErrRemovesOpaque
	}{
		{
			name: "table row and column removed",
			base: `[{"kind":"table","id":"t","children":[
				{"kind":"tableRow","id":"r1","children":[{"kind":"tableCell","id":"c1","header":true,"children":[{"kind":"paragraph","id":"p1","content":[{"text":"STR"}]}]},{"kind":"tableCell","id":"c2","header":true,"children":[{"kind":"paragraph","id":"p2","content":[{"text":"DEX"}]}]}]},
				{"kind":"tableRow","id":"r2","children":[{"kind":"tableCell","id":"c3","children":[{"kind":"paragraph","id":"p3","content":[{"text":"18 (+4)"}]}]},{"kind":"tableCell","id":"c4","children":[{"kind":"paragraph","id":"p4","content":[{"text":"12 (+1)"}]}]}]},
				{"kind":"tableRow","id":"r3","children":[{"kind":"tableCell","id":"c5","children":[{"kind":"paragraph","id":"p5","content":[{"text":"x"}]}]},{"kind":"tableCell","id":"c6","children":[{"kind":"paragraph","id":"p6","content":[{"text":"y"}]}]}]}]}]`,
			edited: "| STR |\n| --- |\n| 19 (+4) |",
		},
		{
			name:   "whole table replaced by a paragraph",
			base:   `[{"kind":"table","id":"t","children":[{"kind":"tableRow","id":"r1","children":[{"kind":"tableCell","id":"c1","header":true,"children":[{"kind":"paragraph","id":"p1","content":[{"text":"A"}]}]}]}]}]`,
			edited: "No table now.",
		},
		{
			name:   "list item removed",
			base:   `[{"kind":"bulletList","id":"l","children":[{"kind":"listItem","id":"i1","children":[{"kind":"paragraph","id":"p1","content":[{"text":"a"}]}]},{"kind":"listItem","id":"i2","children":[{"kind":"paragraph","id":"p2","content":[{"text":"b"}]}]}]}]`,
			edited: "- a",
		},
		{
			name:   "task item removed",
			base:   `[{"kind":"taskList","id":"l","children":[{"kind":"taskItem","id":"i1","checked":false,"content":[{"text":"a"}]},{"kind":"taskItem","id":"i2","checked":true,"content":[{"text":"b"}]}]}]`,
			edited: "- [ ] a",
		},
		{
			name:   "layout column removed",
			base:   `[{"kind":"layout","id":"l","children":[{"kind":"layoutColumn","id":"c1","children":[{"kind":"paragraph","id":"p1","content":[{"text":"left"}]}]},{"kind":"layoutColumn","id":"c2","children":[{"kind":"paragraph","id":"p2","content":[{"text":"right"}]}]}]}]`,
			edited: ":::layout\n:::column\nleft\n:::\n:::",
		},
		{
			name:   "row with an unmodelled field is still opaque",
			base:   `[{"kind":"table","id":"t","children":[{"kind":"tableRow","id":"r1","children":[{"kind":"tableCell","id":"c1","header":true,"children":[{"kind":"paragraph","id":"p1","content":[{"text":"A"}]}]}]},{"kind":"tableRow","id":"r2","mystery":1,"children":[{"kind":"tableCell","id":"c2","children":[{"kind":"paragraph","id":"p2","content":[{"text":"1"}]}]}]}]}]`,
			edited: "No table now.",
			opaque: true,
		},
		{
			name:   "unsupported block inside a removed cell is still opaque",
			base:   `[{"kind":"table","id":"t","children":[{"kind":"tableRow","id":"r1","children":[{"kind":"tableCell","id":"c1","header":true,"children":[{"kind":"unsupported","id":"u1"}]}]}]}]`,
			edited: "No table now.",
			opaque: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed, _, err := FromMarkdown(c.edited, FromOptions{})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, _, err = reconcile(parsed, decodeList(t, c.base), codec, false)
			if got := errors.Is(err, ErrRemovesOpaque); got != c.opaque {
				t.Fatalf("ErrRemovesOpaque = %v, want %v (err %v)", got, c.opaque, err)
			}
			if !c.opaque && err != nil {
				t.Fatalf("reconcile: %v", err)
			}
		})
	}
}
