package markdown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/swiftj/golk/lk/blocks"
)

// seedMarkdown adds the golden files and a set of tricky inputs.
func seedMarkdown(f *testing.F) {
	for _, name := range []string{"golden_page.md", "golden_page_ids.md", "shapes.md"} {
		if b, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(string(b))
		}
	}
	for _, s := range []string{
		"", "a", ":::secret\nx\n:::\n", ":::secret\nx\n", ":::\n", "::paragraph\n",
		"> :::secret\n> x\ny\n> :::\n", "- :::secret\n  x\n:::\n", "    :::secret\nx\n    :::\n",
		"[a]{underline}", "[a]{mention=x}", "[[a]{bold}](u)", "![a](u){fill}", "# T {align=center}",
		"- [ ] {#t} a\n  - [x] b\n", "| a |\n| --- |\n| b |\n", "```lk-raw\n{\"kind\":\"x\"}\n```\n",
		":::collection\n- @a\n\n```lk-filter\n{\"type\":\"filter\",\"id\":\"f\",\"filterType\":\"tag\"}\n```\n:::\n",
		":::keyvalue\n- a :: b {text}\n:::\n", ":::meter mode=bar\n- a :: 1/2 icon=x\n:::\n",
		"[^1]", "[x]: http://y\n", "<div>", "a <b> c", "*a **b** c*", "\\", "{", "&#32;", "[",
		":::layout\n:::column width=50\nx\n:::\n:::\n", ":::table\n:::row\n:::cell header\nx\n:::\n:::\n:::\n",
		":::infobox\n![a](u){title=t}\n\n::unsupported id=u\n:::\n",
	} {
		f.Add(s)
	}
}

// slowFuzz flags an input that takes pathologically long (fuzz inputs are
// small, so anything near this is super-linear behaviour).
const slowFuzz = 2 * time.Second

// FuzzParse checks M-API-03 for parsing: no panic on any input, and any
// input that parses renders and parses back to the same blocks.
func FuzzParse(f *testing.F) {
	seedMarkdown(f)
	f.Fuzz(func(t *testing.T, src string) {
		start := time.Now()
		defer func() {
			if d := time.Since(start); d > slowFuzz {
				t.Fatalf("slow input (%v): %q", d, src)
			}
		}()
		got, err := parse(src)
		if err != nil {
			if got != nil {
				t.Fatalf("error %v with blocks", err)
			}
			return
		}
		md, err := render(got, ToOptions{})
		if err != nil {
			t.Fatalf("render of a parse failed: %v", err)
		}
		again, err := parse(md)
		if err != nil {
			t.Fatalf("re-parse: %v\ninput %q\nrendered %q", err, src, md)
		}
		if a, b := shown(t, again), shown(t, got); a != b {
			t.Fatalf("re-parse differs\ninput %q\nrendered %q\ngot  %s\nwant %s", src, md, a, b)
		}
		md2, _ := render(again, ToOptions{})
		if md2 != md {
			t.Fatalf("M-API-02: rendering not stable\n%q\n%q", md, md2)
		}
	})
}

// FuzzRenderRoundTrip checks M-API-01 and M-API-03 for rendering: any page
// the blocks package decodes renders, and parses back to the same blocks,
// exactly with Base and apart from hidden fields without it.
func FuzzRenderRoundTrip(f *testing.F) {
	if b, err := os.ReadFile("../blocks/testdata/plaintext_golden.json"); err == nil {
		var g struct {
			Blocks json.RawMessage `json:"blocks"`
		}
		if json.Unmarshal(b, &g) == nil {
			f.Add([]byte(g.Blocks))
		}
	}
	for _, s := range shapes {
		f.Add([]byte("[" + s.json + "]"))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var page blocks.List
		if json.Unmarshal(data, &page) != nil {
			return
		}
		if b, err := json.Marshal(page); err != nil || !utf8.Valid(b) {
			if err == nil {
				if _, err := ToMarkdown(page, ToOptions{}); err == nil {
					t.Fatal("rendered a page whose JSON is not valid UTF-8")
				}
			}
			return
		}
		if !uniqueIDs(page) || hasBareUnsupported(page) {
			return // a server page never repeats an id or has an unsupported block without one
		}
		roundTrip(t, page)
	})
}

// hasBareUnsupported reports whether page has an unsupported block or row
// without an id, which base matching rejects (M-SAFE-06).
func hasBareUnsupported(page blocks.List) bool {
	bare := false
	blocks.Walk(page, func(_ string, n blocks.Node) bool {
		if n.Kind() == blocks.KindUnsupported && n.NodeID() == "" {
			bare = true
		}
		return !bare
	})
	return bare
}
