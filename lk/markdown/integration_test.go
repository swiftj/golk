//go:build integration

package markdown_test

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

func TestMain(m *testing.M) { os.Exit(sandbox.Main(m)) }

func ids(l blocks.List) []string {
	var out []string
	blocks.Walk(l, func(_ string, n blocks.Node) bool {
		out = append(out, n.NodeID())
		return true
	})
	return out
}

// TestIntegration_MarkdownEdit reads a page as Markdown, edits one
// paragraph and writes it back: every other block keeps its id and
// content, and a second write from the stale base is refused.
func TestIntegration_MarkdownEdit(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	pb, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name("Princess Bubblegum")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, pb.ID)
	candy, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name("Candy Kingdom")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, candy.ID)
	doc := lk.DocRef{ResourceID: candy.ID, DocumentID: candy.Documents[0].ID}

	stats := blocks.KeyValue(blocks.KV("Ruler", blocks.MentionOf(pb.ID, "")), blocks.KV("Capital", "Candy Castle"))
	stats.Title = lk.Some("Stats")
	page := blocks.List{
		blocks.H(1, "Candy Kingdom"),
		blocks.P("Ruled by ", blocks.MentionOf(pb.ID, ""), ", ", blocks.Bold("wisely"), "."),
		blocks.P("The castle is pink."),
		blocks.Quote("Science is the best kind of magic."),
		blocks.UL("Gumdrops", blocks.LI("Peppermint", blocks.UL("Butler"))),
		blocks.Callout(blocks.ToneWarning, "Beware the Lich."),
		blocks.Secret(blocks.P("The Lich is not really gone.")),
		blocks.Table(blocks.TR(blocks.TH("Name"), blocks.TH("Role")), blocks.TR(blocks.TD("PB"), blocks.TD("Ruler"))),
		stats,
		blocks.Infobox(blocks.AsProperty(blocks.TextField("Rolled in candy."), blocks.Envelope{Title: lk.Some("Origin")})),
	}
	if _, err := c.Documents.ReplaceBlocks(ctx, doc, page); err != nil {
		t.Fatal(err)
	}

	base, err := markdown.Get(ctx, c, doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(base.Markdown, "lk-raw") {
		t.Errorf("a page of modelled blocks rendered an lk-raw fence:\n%s", base.Markdown)
	}

	// Unchanged Markdown writes the page back exactly.
	same, rep, err := markdown.Replace(ctx, c, doc, base, base.Markdown, markdown.ReplaceOptions{})
	if err != nil {
		t.Fatalf("%v\n%s", err, base.Markdown)
	}
	if a, b := mustJSON(t, base.Blocks), mustJSON(t, same.Blocks); a != b {
		t.Errorf("unchanged Markdown changed the page:\nbefore %s\nafter  %s", a, b)
	}
	if len(rep.Changed)+len(rep.Added)+len(rep.Removed) != 0 {
		t.Errorf("unchanged Markdown report = %+v", rep)
	}

	base, err = markdown.Get(ctx, c, doc)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(base.Markdown, "The castle is pink.", "The castle is very pink.", 1)
	if edited == base.Markdown {
		t.Fatalf("paragraph not found in\n%s", base.Markdown)
	}
	got, rep, err := markdown.Replace(ctx, c, doc, base, edited, markdown.ReplaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	editedID := base.Blocks[2].NodeID()
	if !slices.Equal(rep.Changed, []string{editedID}) || len(rep.Added)+len(rep.Removed) != 0 {
		t.Errorf("report = %+v, want only %s changed", rep, editedID)
	}
	if a, b := ids(base.Blocks), ids(got.Blocks); !slices.Equal(a, b) {
		t.Errorf("ids changed:\nbefore %v\nafter  %v", a, b)
	}
	if p := got.Blocks[2].(*blocks.ParagraphBlock); blocks.RunsText(p.Content) != "The castle is very pink." {
		t.Errorf("edited paragraph = %q", blocks.RunsText(p.Content))
	}
	for i, b := range got.Blocks {
		if i != 2 && mustJSON(t, b) != mustJSON(t, base.Blocks[i]) {
			t.Errorf("block %d changed:\nbefore %s\nafter  %s", i, mustJSON(t, base.Blocks[i]), mustJSON(t, b))
		}
	}

	// The base is now stale.
	_, _, err = markdown.Replace(ctx, c, doc, base, base.Markdown, markdown.ReplaceOptions{})
	if !errors.Is(err, lk.ErrVersionMismatch) {
		t.Errorf("stale base: err = %v, want ErrVersionMismatch", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
