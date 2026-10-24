package blocks

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRead_C_READ_01_PlainTextMatchesServer(t *testing.T) {
	g := loadGolden(t)
	var l List
	if err := json.Unmarshal(g.Blocks, &l); err != nil {
		t.Fatal(err)
	}
	if got := PlainText(l); got != g.Text {
		t.Errorf("PlainText\n got %q\nwant %q", got, g.Text)
	}
	if got := PlainText(nil); got != "" {
		t.Errorf("empty page = %q", got)
	}
	if got := PlainText(List{&UnsupportedBlock{ID: "u"}, &RawBlock{KindName: "x"}, P(""), P("a")}); got != "a" {
		t.Errorf("unsupported and empty = %q", got)
	}
}

func TestRead_C_READ_02_WalkPaths(t *testing.T) {
	page := List{
		P("intro"),
		Quote(Callout(ToneInfo, P("deep"))),
		Infobox(
			AsProperty(KeyValue(), Envelope{}),
			AsProperty(TextField("in a property"), Envelope{}),
		),
	}
	var paths []string
	Walk(page, func(path string, n Node) bool {
		paths = append(paths, path+" "+string(n.Kind()))
		return true
	})
	want := []string{
		"blocks[0] paragraph",
		"blocks[1] quote",
		"blocks[1].children[0] callout",
		"blocks[1].children[0].children[0] paragraph",
		"blocks[2] infobox",
		"blocks[2].rows[0] keyValue",
		"blocks[2].rows[1] textField",
		"blocks[2].rows[1].children[0] paragraph",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("paths\n got %q\nwant %q", paths, want)
	}

	// The paths the server reported for the same shapes (D-009, A-18).
	for _, p := range []string{"blocks[1].children[0].children[0]", "blocks[2].rows[1].children[0]"} {
		if !slices.ContainsFunc(paths, func(s string) bool { return strings.HasPrefix(s, p) }) {
			t.Errorf("server path %s not produced", p)
		}
	}

	var n int
	Walk(page, func(string, Node) bool { n++; return n < 3 })
	if n != 3 {
		t.Errorf("visited %d after stopping at 3", n)
	}
}

func TestRead_C_READ_03_FindBlock(t *testing.T) {
	page := List{
		WithID("a", P("first")),
		Quote(WithID("b", P("nested")), WithID("b", P("second b"))),
		Infobox(&TextFieldProperty{Envelope: Envelope{ID: "prop"}, Children: List{WithID("c", P("in prop"))}}),
	}
	if got, ok := FindBlock(page, "b").(*ParagraphBlock); !ok || RunsText(got.Content) != "nested" {
		t.Errorf("FindBlock(b) = %#v", got)
	}
	if got := FindBlock(page, "prop"); got == nil || got.Kind() != KindTextField {
		t.Errorf("FindBlock(prop) = %#v", got)
	}
	if got := FindBlock(page, "c"); got == nil {
		t.Error("block inside a property not found")
	}
	if got := FindBlock(page, "missing"); got != nil {
		t.Errorf("FindBlock(missing) = %#v", got)
	}
}

func TestRead_C_READ_02_EveryChildHolderIsWalked(t *testing.T) {
	check := func(name string, v any) {
		_, isContainer := v.(container)
		if hasChildren := fieldsOf(reflect.TypeOf(v).Elem())["children"]; hasChildren != isContainer {
			t.Errorf("%s: children field %v, walked %v", name, hasChildren, isContainer)
		}
	}
	for k, newBlock := range blockKinds {
		check(string(k)+" block", newBlock())
	}
	for k, newProp := range propertyKinds {
		check(string(k)+" property", newProp())
	}
}
