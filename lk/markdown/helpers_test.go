package markdown

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/swiftj/golk/lk/blocks"
)

// shown is the JSON of v without the fields Markdown doesn't show (block,
// envelope, row and item ids and ranks), for comparing a parse without Base
// with the page it was rendered from.
func shown(t testing.TB, v any) string {
	t.Helper()
	b, err := hiddenStripped(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustJSON(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// goldenPage is the sandbox golden page of lk/blocks.
func goldenPage(t testing.TB) (blocks.List, string) {
	t.Helper()
	data, err := os.ReadFile("../blocks/testdata/plaintext_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Text   string      `json:"text"`
		Blocks blocks.List `json:"blocks"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}
	return g.Blocks, g.Text
}

// roundTrip checks M-API-01 for page: without Base the parse equals page
// apart from hidden fields; with IDs it also keeps every id it writes; with
// Base it is byte-identical. It returns the Markdown.
func roundTrip(t testing.TB, page blocks.List) string {
	t.Helper()
	md, err := ToMarkdown(page, ToOptions{})
	if err != nil {
		t.Fatalf("render: %v\npage %s", err, mustJSON(t, page))
	}
	again, err := ToMarkdown(page, ToOptions{})
	if err != nil || again != md {
		t.Fatalf("M-API-02: rendering is not deterministic")
	}
	got, err := parse(md)
	if err != nil {
		t.Fatalf("parse: %v\nmarkdown:\n%s", err, md)
	}
	if a, b := shown(t, got), shown(t, page); a != b {
		t.Fatalf("round trip differs\nmarkdown:\n%s\ngot  %s\nwant %s", md, a, b)
	}
	mdIDs, err := ToMarkdown(page, ToOptions{IDs: true})
	if err != nil {
		t.Fatalf("render with ids: %v", err)
	}
	gotIDs, err := parse(mdIDs)
	if err != nil {
		t.Fatalf("parse with ids: %v\nmarkdown:\n%s", err, mdIDs)
	}
	if a, b := shown(t, gotIDs), shown(t, page); a != b {
		t.Fatalf("round trip with ids differs\nmarkdown:\n%s\ngot  %s\nwant %s", mdIDs, a, b)
	}
	base, _, err := FromMarkdown(md, FromOptions{Base: page})
	if err != nil {
		t.Fatalf("FromMarkdown with Base: %v\nmarkdown:\n%s", err, md)
	}
	if a, b := mustJSON(t, base), mustJSON(t, page); a != b {
		t.Fatalf("M-API-01 with Base: not byte-identical\nmarkdown:\n%s\ngot  %s\nwant %s", md, a, b)
	}
	return md
}

func hasRaw(md string) bool { return strings.Contains(md, "lk-raw") }

func listOf(t testing.TB, s string) blocks.List {
	t.Helper()
	var l blocks.List
	if err := json.Unmarshal([]byte(s), &l); err != nil {
		t.Fatal(err)
	}
	return l
}
