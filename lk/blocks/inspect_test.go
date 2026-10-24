package blocks

import (
	"encoding/json"
	"testing"
)

func TestInspect_ChildrenAndUnmodelled(t *testing.T) {
	q := Quote(P("a"))
	if l, ok := Children(q); !ok || len(l) != 1 {
		t.Fatalf("Children(quote) = %v, %v", l, ok)
	}
	if !SetChildren(q, List{P("b"), P("c")}) || len(q.Children) != 2 {
		t.Fatalf("SetChildren did not replace the children: %v", q.Children)
	}
	if _, ok := Children(P("x")); ok {
		t.Error("Children(paragraph) reported a children list")
	}
	if SetChildren(P("x"), nil) {
		t.Error("SetChildren(paragraph) reported success")
	}

	var plain, extra ParagraphBlock
	if err := json.Unmarshal([]byte(`{"kind":"paragraph","content":[]}`), &plain); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"kind":"paragraph","content":[],"glow":1}`), &extra); err != nil {
		t.Fatal(err)
	}
	if HasUnmodelled(&plain) || !HasUnmodelled(&extra) {
		t.Errorf("HasUnmodelled = %v, %v; want false, true", HasUnmodelled(&plain), HasUnmodelled(&extra))
	}
	var m Mention
	if err := json.Unmarshal([]byte(`{"mention":{"resourceId":"r1","glow":1}}`), &m); err != nil {
		t.Fatal(err)
	}
	if !HasUnmodelled(&m) {
		t.Error("HasUnmodelled(mention with an unknown ref field) = false")
	}
	if HasUnmodelled("x") {
		t.Error("HasUnmodelled(string) = true")
	}
}
