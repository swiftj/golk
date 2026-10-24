package markdown

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
)

// randomRuns makes normalized runs (no empty text, no mergeable
// neighbours) from an alphabet of everything that could start syntax.
func randomRuns(r *rand.Rand) blocks.Runs {
	const alphabet = "ab \t#*_~`[]{}()<>|\\!&;:.-+=1\"'@^$%/xé日"
	var rs blocks.Runs
	n := 1 + r.IntN(6)
	for range n {
		switch r.IntN(10) {
		case 0:
			if len(rs) > 0 && len(rs) < n-1 {
				rs = append(rs, &blocks.Break{})
			}
			continue
		case 1:
			m := &blocks.Mention{ResourceID: "r" + string(alphabet[r.IntN(len(alphabet))])}
			if r.IntN(2) == 0 {
				m.Label = opt.Some(randomText(r, alphabet))
			}
			if r.IntN(2) == 0 {
				m.Name = opt.Some(randomText(r, alphabet))
			}
			rs = append(rs, m)
			continue
		}
		t := &blocks.Text{Text: randomText(r, alphabet)}
		t.Bold, t.Italic, t.Strike = r.IntN(3) == 0, r.IntN(3) == 0, r.IntN(4) == 0
		t.Underline, t.Code = r.IntN(4) == 0, r.IntN(5) == 0
		if r.IntN(4) == 0 {
			t.Link = opt.Some("https://x.test/" + randomText(r, alphabet))
		}
		if r.IntN(5) == 0 {
			t.Color = opt.Some("#ff0000")
		}
		if p, ok := lastText(rs); ok && sameMarks(p, t) {
			p.Text += t.Text
			continue
		}
		rs = append(rs, t)
	}
	if len(rs) > 0 {
		if _, ok := rs[len(rs)-1].(*blocks.Break); ok {
			rs = rs[:len(rs)-1]
		}
	}
	return rs
}

func lastText(rs blocks.Runs) (*blocks.Text, bool) {
	if len(rs) == 0 {
		return nil, false
	}
	t, ok := rs[len(rs)-1].(*blocks.Text)
	return t, ok
}

func randomText(r *rand.Rand, alphabet string) string {
	runes := []rune(alphabet)
	var b strings.Builder
	for range 1 + r.IntN(8) {
		b.WriteRune(runes[r.IntN(len(runes))])
	}
	return b.String()
}

// M-INL-01: arbitrary text, with any marks, round-trips verbatim, in
// paragraphs, headings, task items, rows and table cells. Only text no
// Markdown construct can hold falls back to lk-raw.
func TestRender_M_INL_01_ArbitraryText(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	raw := 0
	const n = 3000
	for i := range n {
		rs := randomRuns(r)
		var noBreak blocks.Runs
		for _, x := range rs {
			if _, ok := x.(*blocks.Break); !ok {
				noBreak = append(noBreak, x)
			}
		}
		noBreak = mergeRuns(noBreak)
		cell := &blocks.TableCellBlock{Header: opt.Some(true), Children: blocks.List{&blocks.ParagraphBlock{Content: noBreak}}}
		page := blocks.List{
			&blocks.ParagraphBlock{Content: rs},
			&blocks.HeadingBlock{Level: 2, Content: noBreak},
			&blocks.TaskListBlock{Children: blocks.List{&blocks.TaskItemBlock{Content: rs}}},
			&blocks.KeyValueBlock{Rows: blocks.KVRows{{Key: randomText(r, "a:b \\*#-"), Content: rs}}},
			&blocks.TableBlock{Children: blocks.List{&blocks.TableRowBlock{Children: blocks.List{cell}}}},
		}
		md := roundTrip(t, page)
		if hasRaw(md) {
			raw++
			if testing.Verbose() {
				t.Logf("case %d went lk-raw:\n%s", i, md)
			}
		}
	}
	// Code spans of 3+ backticks at the start of a line are fences, so a
	// few cases must go lk-raw; anything more is an escaping gap.
	if raw > n/100 {
		t.Errorf("%d of %d random pages needed lk-raw", raw, n)
	}
}

func TestRender_M_INL_01_Escapes(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"a*b_c", `a\*b\_c`},
		{"[x](y)", `\[x\](y)`},
		{"{#id}", `\{#id\}`},
		{`back\slash`, `back\\slash`},
		{"# h", `\# h`},
		{"- li", `\- li`},
		{"+ li", `\+ li`},
		{"1. li", `1\. li`},
		{"2) li", `2\) li`},
		{"> q", `\> q`},
		{"::map", `\::map`},
		{"&amp; &x", `\&amp; \&x`},
		{"a & b", "a & b"},
		{"  lead", "&#32;&#32;lead"},
		{"trail ", "trail&#32;"},
		{"<b>", `\<b>`},
		{"a|b", `a\|b`},
		{"~~s~~", `\~\~s\~\~`},
		{"`c`", "\\`c\\`"},
		{"Château", "Château"},
	} {
		md, err := ToMarkdown(blocks.List{blocks.P(c.text)}, ToOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSuffix(md, "\n"); got != c.want {
			t.Errorf("%q renders %q, want %q", c.text, got, c.want)
		}
		roundTrip(t, blocks.List{blocks.P(c.text)})
	}
	// Spaces at a mark's edge stay inside it: a bracketed span holds them.
	md, _ := ToMarkdown(blocks.List{blocks.P("Hello", blocks.Bold(" world"))}, ToOptions{})
	if md != "Hello[ world]{bold}\n" {
		t.Errorf("edge space: %q", md)
	}
}

// M-INL-02: marks nest in a fixed order, so runs that mean the same render
// the same however they were built.
func TestRender_M_INL_02_MarkOrder(t *testing.T) {
	all := &blocks.Text{Text: "z", Bold: true, Italic: true, Strike: true, Underline: true, Code: true, Link: opt.Some("u"), Color: opt.Some("#000000")}
	md, _ := ToMarkdown(blocks.List{&blocks.ParagraphBlock{Content: blocks.Runs{all}}}, ToOptions{})
	if want := "[***~~[`z`]{underline color=#000000}~~***](u)\n"; md != want {
		t.Errorf("got %q, want %q", md, want)
	}
	for _, c := range []struct {
		runs blocks.Runs
		want string
	}{
		{blocks.R(blocks.Link("u", blocks.Bold("a")), blocks.Link("u", "b")), "[**a**b](u)\n"},
		{blocks.R(blocks.Bold("a"), blocks.Bold(blocks.Italic("b"))), "**a*b***\n"},
		{blocks.R(blocks.Underline(blocks.Bold("a"))), "**[a]{underline}**\n"},
		{blocks.R(blocks.Mono("x"), &blocks.Text{Text: "y", Code: true, Underline: true}), "`x`[`y`]{underline}\n"},
	} {
		md, _ := ToMarkdown(blocks.List{&blocks.ParagraphBlock{Content: c.runs}}, ToOptions{})
		if md != c.want {
			t.Errorf("got %q, want %q", md, c.want)
		}
		roundTrip(t, blocks.List{&blocks.ParagraphBlock{Content: c.runs}})
	}
	// Markdown written in another order parses to the same runs and
	// renders canonically.
	for _, src := range []string{"*__a__*", "**_a_**", "***a***", "[[*a*]{bold}](u)"} {
		got := mustParse(t, src)
		md, _ := ToMarkdown(got, ToOptions{})
		again := mustParse(t, md)
		if mustJSON(t, again) != mustJSON(t, got) {
			t.Errorf("%q: canonical %q parses differently", src, md)
		}
	}
}

// Parsing inline Markdown: CommonMark forms, autolinks, line breaks.
func TestParse_M_INL_01_Inline(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"_a_ __b__ ~c~ ~~d~~", `[{"text":"a","italic":true},{"text":" "},{"text":"b","bold":true},{"text":" "},{"text":"c","strike":true},{"text":" "},{"text":"d","strike":true}]`},
		{"a\nb", `[{"text":"a b"}]`},
		{"a  \nb", `[{"text":"a"},{"break":true},{"text":"b"}]`},
		{"a\\\nb", `[{"text":"a"},{"break":true},{"text":"b"}]`},
		{"<https://x.test>", `[{"text":"https://x.test","link":"https://x.test"}]`},
		{"<me@x.test>", `[{"text":"me@x.test","link":"mailto:me@x.test"}]`},
		{"https://x.test", `[{"text":"https://x.test"}]`},
		{"&copy; &#65; &#x42; \\&amp;", `[{"text":"© A B \u0026amp;"}]`},
		{"`` a ` b ``", "[{\"text\":\"a ` b\",\"code\":true}]"},
		{"[t](<a b>)", `[{"text":"t","link":"a b"}]`},
		{"[t]()", `[{"text":"t","link":""}]`},
		{"[x]{sub sup}", `[{"text":"x","sub":true,"sup":true}]`},
		{"[x]{bold italic strike}", `[{"text":"x","bold":true,"italic":true,"strike":true}]`},
		{"[x]{underline=false}", `[{"text":"x"}]`},
		{"[a [b]{underline} c]{color=#ff0000}", `[{"text":"a ","color":"#ff0000"},{"text":"b","underline":true,"color":"#ff0000"},{"text":" c","color":"#ff0000"}]`},
		{"[PB]{mention=pb1 label doc=d1}", `[{"mention":{"resourceId":"pb1","documentId":"d1","text":"PB"}}]`},
		{"[Princess]{mention=pb1}", `[{"mention":{"resourceId":"pb1","name":"Princess"}}]`},
		{"[]{mention=pb1}", `[{"mention":{"resourceId":"pb1"}}]`},
		{"[x] not a span", `[{"text":"[x] not a span"}]`},
		{"[x] {not}", ""},
	} {
		got, err := parse(c.src)
		if c.want == "" {
			if err == nil {
				t.Errorf("%q: accepted", c.src)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		p, ok := got[0].(*blocks.ParagraphBlock)
		if !ok || len(got) != 1 {
			t.Errorf("%q: %s", c.src, mustJSON(t, got))
			continue
		}
		if s := mustJSON(t, p.Content); s != c.want {
			t.Errorf("%q:\ngot  %s\nwant %s", c.src, s, c.want)
		}
	}
}
