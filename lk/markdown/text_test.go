package markdown

import (
	"strings"
	"sync"
	"testing"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// visibleText derives the text a reader sees in LK Markdown straight from
// the goldmark tree, without the converter: one line per paragraph,
// heading, task item and code block; nothing from attributes, rows, items,
// images or lk-raw (M-TXT-01).
func visibleText(t *testing.T, md string) string {
	t.Helper()
	src := []byte(md)
	doc := gm().Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	var lines []string
	add := func(s string) {
		if s != "" {
			lines = append(lines, s)
		}
	}
	var inline func(n ast.Node, b *strings.Builder)
	inline = func(n ast.Node, b *strings.Builder) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Text:
				raw := v.Segment.Value(src)
				// The space that separates an attribute set isn't text.
				if _, ok := v.NextSibling().(*attrSetNode); ok {
					raw = []byte(strings.TrimSuffix(string(raw), " "))
				}
				if _, ok := v.PreviousSibling().(*attrSetNode); ok {
					raw = []byte(strings.TrimPrefix(string(raw), " "))
				}
				if v.IsRaw() {
					b.Write(raw)
				} else {
					b.WriteString(decodeText(raw))
				}
				if v.SoftLineBreak() {
					b.WriteByte(' ')
				}
				if v.HardLineBreak() {
					b.WriteByte('\n')
				}
			case *ast.CodeSpan:
				var s strings.Builder
				inline(v, &s)
				b.WriteString(strings.ReplaceAll(s.String(), "\n", " "))
			case *attrSetNode, *east.TaskCheckBox, *ast.Image:
			default:
				inline(v, b)
			}
		}
	}
	var block func(n ast.Node)
	block = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Paragraph, *ast.TextBlock, *ast.Heading, *east.TableCell:
				var b strings.Builder
				inline(v, &b)
				add(b.String())
				block(v) // a task item's nested lists
			case *ast.FencedCodeBlock:
				info := ""
				if v.Info != nil {
					info = string(v.Info.Segment.Value(src))
				}
				if !strings.HasPrefix(info, "lk-") {
					add(strings.TrimSuffix(string(v.Lines().Value(src)), "\n"))
				}
			case *directiveNode:
				switch v.name {
				case "keyvalue", "meter", "links", "collection", "infobox":
				default:
					block(v)
				}
			case *attrLineNode:
			default:
				block(v)
			}
		}
	}
	block(doc)
	return strings.Join(lines, "\n")
}

func TestRender_M_TXT_01_PlainTextParity(t *testing.T) {
	page, want := goldenPage(t)
	if got := blocks.PlainText(page); got != want {
		t.Fatalf("fixture: PlainText %q, want %q", got, want)
	}
	md, err := ToMarkdown(page, ToOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := visibleText(t, md); got != want {
		t.Errorf("visible text of the Markdown:\n%q\nwant\n%q", got, want)
	}
	ids, _ := ToMarkdown(page, ToOptions{IDs: true})
	if got := visibleText(t, ids); got != want {
		t.Errorf("visible text with ids:\n%q\nwant\n%q", got, want)
	}
	// And for every shape rendered in Markdown form.
	for _, s := range shapes {
		if s.want != "md" || strings.Contains(s.json, "hologram") || strings.Contains(s.json, `"extra"`) {
			continue
		}
		l := listOf(t, "["+s.json+"]")
		md, _ := ToMarkdown(l, ToOptions{})
		if got, want := visibleText(t, md), blocks.PlainText(l); got != want {
			t.Errorf("%s: visible %q, PlainText %q\n%s", s.name, got, want, md)
		}
	}
}

// M-API-01 on the golden page, and on pages built with the blocks builders.
func TestRoundTrip_M_API_01(t *testing.T) {
	page, _ := goldenPage(t)
	roundTrip(t, page)
	built := blocks.List{
		blocks.H(1, "The ", blocks.Bold("Candy"), " Kingdom"),
		blocks.P("Ruled by ", blocks.MentionOf("pb1", "PB"), ". See ", blocks.Link("lk://x", "the map"), "."),
		blocks.Quote("A quote", blocks.UL("one", "two")),
		blocks.Callout(blocks.ToneWarning, "Beware"),
		blocks.Secret("The Lich ", blocks.Italic("is"), " back."),
		blocks.Layout(blocks.Column(50, "Left"), blocks.Column(50, "Right")),
		blocks.Table(blocks.TR(blocks.TH("A"), blocks.TH("B")), blocks.TR(blocks.TD("1"), blocks.TD("2"))),
		blocks.Tasks(blocks.Task(true, "done"), blocks.Task(false, "todo")),
		blocks.KeyValue(blocks.KV("Species", "Dog"), blocks.KV("Age", blocks.Bold("old"))),
		blocks.Infobox(blocks.AsProperty(blocks.Image("https://assets.legendkeeper.com/a.png", "A"), blocks.Envelope{}),
			blocks.AsProperty(blocks.KeyValue(blocks.KV("k", "v")), blocks.Envelope{})),
		blocks.Meter(blocks.MeterBar, blocks.MeterItem{Name: "HP", Icon: "heart", Current: 1, Max: 2}),
		blocks.Links("a", "b"), blocks.Map("r", "d"), blocks.EmptyMap(), blocks.Code("x := 1", "go"), blocks.Rule(),
	}
	md := roundTrip(t, built)
	if hasRaw(md) {
		t.Errorf("builder page went lk-raw:\n%s", md)
	}
}

// ToOptions.IDs writes every id of the golden page, so a parse without Base
// gets them all back; only ranks (which Markdown never shows) differ.
func TestRender_M_API_01_IDs(t *testing.T) {
	page, _ := goldenPage(t)
	md, err := ToMarkdown(page, ToOptions{IDs: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := parse(md)
	if err != nil {
		t.Fatal(err)
	}
	noRanks := func(v any) string {
		s := mustJSON(t, v)
		for _, r := range []string{`"rank":"C",`, `"rank":"F",`, `"rank":"c",`, `"rank":"f",`, `"rank":"X",`} {
			s = strings.ReplaceAll(s, r, "")
		}
		return s
	}
	if a, b := noRanks(got), noRanks(page); a != b {
		t.Errorf("ids lost\ngot  %s\nwant %s", a, b)
	}
}

// M-API-03: no panic on hostile input, and the codec is safe for
// concurrent use (it holds no mutable state).
func TestParse_M_API_03_HostileAndConcurrent(t *testing.T) {
	for _, src := range []string{
		"\x00", "\r", "\xff", ":::", "::", "{", "}", "[", "]", "[]{", "[x]{", "![", "```", "```lk-raw", "> :::", "- :::", "| a |\n| - |",
		strings.Repeat("> ", 100) + ":::secret", strings.Repeat("[", 100) + strings.Repeat("]{underline}", 100),
	} {
		_, _ = parse(src)
	}
	page, _ := goldenPage(t)
	md, _ := ToMarkdown(page, ToOptions{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				out, err := ToMarkdown(page, ToOptions{})
				if err != nil || out != md {
					t.Error("concurrent render differs")
				}
				if _, err := parse(md); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}
