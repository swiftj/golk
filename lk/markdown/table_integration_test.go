//go:build integration

package markdown_test

import (
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

// TestIntegration_StatBlockTable_M_SAFE_04 writes stat-block tables the way
// Claude writes them, reads them back as Markdown without lk-raw, and edits
// a row and column away without AllowRemoveOpaque (DECISIONS.md D-023).
func TestIntegration_StatBlockTable_M_SAFE_04(t *testing.T) {
	c := sandbox.Client(t)
	cases := map[string]struct{ src, edited string }{
		"gfm": {
			src:    "| STR | DEX | CON |\n| --- | --- | --- |\n| 18 (+4) | 12 (+1) | 16 (+3) |\n| x | y | z |",
			edited: "| STR | DEX |\n| --- | --- |\n| 19 (+4) | 12 (+1) |",
		},
		"marks": {
			src:    "| Trait | Value |\n| --- | --- |\n| **Armor Class** | 15 (natural armor) |\n| *Hit Points* | 85 (10d10 + 30) |\n|  | empty first cell |",
			edited: "| Trait | Value |\n| --- | --- |\n| **Armor Class** | 16 (natural armor) |",
		},
		"in-secret": {
			src:    ":::secret\n| STR | DEX |\n| --- | --- |\n| 18 (+4) | 12 (+1) |\n| 1 | 2 |\n:::",
			edited: ":::secret\n| STR |\n| --- |\n| 18 (+4) |\n:::",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := sandbox.Ctx(t)
			content, _, err := markdown.FromMarkdown(tc.src, markdown.FromOptions{})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			r, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name("Stat block table " + name)})
			if err != nil {
				t.Fatal(err)
			}
			sandbox.Cleanup(t, r.ID)
			doc := lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}
			if _, err := c.Documents.ReplaceBlocks(ctx, doc, content); err != nil {
				t.Fatalf("write: %v", err)
			}
			page, err := markdown.Get(ctx, c, doc)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(page.Markdown, "lk-raw") {
				t.Fatalf("table read back as lk-raw:\n%s", page.Markdown)
			}
			if _, _, err := markdown.Replace(ctx, c, doc, page, tc.edited, markdown.ReplaceOptions{}); err != nil {
				t.Fatalf("edit without AllowRemoveOpaque: %v", err)
			}
		})
	}
}
