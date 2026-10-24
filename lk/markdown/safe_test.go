package markdown

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/lk/blocks"
)

func TestParse_M_SAFE_01_UnknownOrInvalid(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{":::secrets\nx\n:::\n", `unknown directive "secrets"`},
		{":::Secret\nx\n:::\n", `unknown directive "Secret"`},
		{"::glow\n", `unknown directive "glow"`},
		{":::callout tone=info glow\n:::\n", "unknown attribute glow"},
		{":::callout tone=info tone=note\n:::\n", "duplicate attribute tone"},
		{":::secret #a #b\nx\n:::\n", "duplicate attribute #id"},
		{":::callout tone=loud\n:::\n", `"loud" is not one of`},
		{":::secret layout=floating\nx\n:::\n", "not one of"},
		{"::embed provider=vimeo url=u\n", "not one of"},
		{":::meter mode=pie\n:::\n", "not one of"},
		{":::keyvalue display=table\n:::\n", "not one of"},
		{"# H {align=justify}\n", "not one of"},
		{":::secret width=wide\nx\n:::\n", "must be an integer"},
		{":::secret width=1.5\nx\n:::\n", "must be an integer"},
		{":::layout\n:::column width=NaN\n:::\n:::\n", "must be a number"},
		{":::layout\n:::column width=1e999\n:::\n:::\n", "must be a number"},
		{":::textfield isTitleHidden=yes\n:::\n", "true or false"},
		{":::expand title\n:::\n", "needs a value"},
		{":::expand title=\"open\n:::\n", "unclosed quoted value"},
		{":::expand title=a\"b\n:::\n", "unexpected"},
		{":::expand title=(x)\n:::\n", "missing attribute value"},
		{"::map viewport=1,2\n", "lat,lng,zoom"},
		{":::secret hidden text\n:::\n", "unknown attribute hidden"},
		{"::unsupported #u1\n", "id="},
		{":::column\nx\n:::\n", "belongs inside"},
		{":::layout\nx\n:::\n", "may hold only :::column"},
		{":::table\n:::cell\n:::\n:::\n", "may hold only :::row"},
		{"::callout tone=info\n", "holds content"},
		{":::embed provider=youtube url=u\n:::\n", "one-line directive"},
		{":::infobox\n:::callout tone=info\n:::\n:::\n", "can't be an infobox row"},
		{":::infobox\ntext\n:::\n", "not text"},
		{":::infobox\n::embed provider=youtube url=u layout=center\n:::\n", "unknown attribute layout"},
		{"[x]{mention=a bold}", "unknown attribute bold"},
		{"[x]{mention=a name=n}", "name= is only"},
	} {
		t.Run("M-SAFE-01 "+c.want, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
	// Enum values the SDK doesn't know are accepted inside lk-raw.
	got := mustParse(t, "```lk-raw\n{\"kind\":\"callout\",\"tone\":\"loud\",\"children\":[]}\n```\n")
	if c, ok := got[0].(*blocks.CalloutBlock); !ok || c.Tone != "loud" {
		t.Errorf("lk-raw with an unknown tone: %s", mustJSON(t, got))
	}
}

// secretCases are malformed or tricky secrets. Each must be rejected or
// give exactly the secret blocks, with the secret text nowhere else.
var secretCases = []struct {
	name, src string
	ok        bool // parses (to secrets holding every "Lich" mention)
}{
	{"unclosed", "Intro\n\n:::secret\nThe Lich lives.\n\nMore about the Lich.\n", false},
	{"unclosed at end of quote", "> :::secret\n> The Lich lives.\n\nAfter\n", false},
	{"unclosed in list item", "- :::secret\n  The Lich lives.\n- next\n", false},
	{"closed outside its list item", "- :::secret\n  The Lich lives.\n:::\n", false},
	{"closed outside its quote", "> :::secret\n> The Lich lives.\n:::\n", false},
	{"misnested with callout", ":::secret\n:::callout tone=info\nThe Lich lives.\n:::\n", false},
	{"stray close after", ":::secret\n:::\nThe Lich lives.\n:::\n", false},
	{"empty then text", ":::secret\n:::\nThe Lich lives.\n", false},
	{"typo name", ":::secrets\nThe Lich lives.\n:::\n", false},
	{"typo case", ":::Secret\nThe Lich lives.\n:::\n", false},
	{"space after colons", "::: secret\nThe Lich lives.\n:::\n", false},
	{"two colons", "::secret\nThe Lich lives.\n:::\n", false},
	{"text on the fence line", ":::secret The Lich lives.\n:::\n", false},
	{"attribute typo", ":::secret widht=3\nThe Lich lives.\n:::\n", false},
	{"indented 4", "    :::secret\n    The Lich lives.\n    :::\n", false},
	{"indented tab", "\t:::secret\nThe Lich lives.\n:::\n", false},
	{"lazy indented", "Intro\n    :::secret\nThe Lich lives.\n    :::\n", false},
	{"paragraph continuation", "Intro\n    :::secret\n    The Lich lives.\n    :::\n", false},
	{"in a code fence", "```\n:::secret\n```\nThe Lich lives.\n:::\n", false},
	{"unclosed code fence inside", ":::secret\n```\nThe Lich lives.\n:::\n", false},
	{"html comment", "<!-- :::secret -->\nThe Lich lives.\n", false},
	{"inline html", ":::secret\n<span>The Lich lives.</span>\n:::\n", false},
	{"fence text is code", ":::secret\n```\n:::\n```\nThe Lich lives.\n:::\n", true},
	{"closing fence in body text", ":::secret\nThe ::: Lich lives.\n:::\n", true},
	{"escaped fence in body", ":::secret\n\\:::\nThe Lich lives.\n:::\n", true},
	{"lazy line in quoted secret", "> :::secret\n> The Lich\nlives on.\n> :::\n", true},
	{"nested", ":::secret\n:::secret\nThe Lich lives.\n:::\n\nThe Lich again.\n:::\n", true},
	{"setext heading", ":::secret\nThe Lich\n===\n:::\n", true},
	{"long fences", "::::secret\nThe Lich lives.\n::::\n", true},
	{"closing fence with spaces", ":::secret\nThe Lich lives.\n:::   \n", true},
	{"in list item", "- :::secret\n  The Lich lives.\n  :::\n", true},
	{"in quote", "> :::secret\n> The Lich lives.\n> :::\n", true},
	{"lk-raw", "```lk-raw\n{\"kind\":\"secret\",\"children\":[{\"kind\":\"paragraph\",\"content\":[{\"text\":\"The Lich lives.\"}]}]}\n```\n", true},
}

// M-SAFE-02: malformed secret syntax never yields secret text outside a
// secret block. Text the author fences as code stays code (that's what a
// fence means), and it never ends a secret.
func TestParse_M_SAFE_02_Secrets(t *testing.T) {
	for _, c := range secretCases {
		t.Run("M-SAFE-02 "+c.name, func(t *testing.T) {
			got, err := parse(c.src)
			if !c.ok {
				var se *SyntaxError
				if !errors.As(err, &se) || got != nil {
					t.Fatalf("want a *SyntaxError and no blocks, got %v %s", err, mustJSON(t, got))
				}
				if se.Line == 0 {
					t.Errorf("error has no line: %v", se)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if leaked := secretOutside(got); leaked != "" {
				t.Fatalf("secret text outside a secret: %q in %s", leaked, mustJSON(t, got))
			}
		})
	}
}

// secretOutside returns text holding "Lich" in a block that isn't inside
// a secret.
func secretOutside(l blocks.List) string {
	var leaked string
	for _, b := range l {
		if _, ok := b.(*blocks.SecretBlock); ok {
			continue
		}
		if kids, ok := blocks.Children(b); ok {
			if s := secretOutside(kids); s != "" {
				leaked = s
			}
			continue
		}
		if s := blocks.PlainText(blocks.List{b}); strings.Contains(s, "Lich") {
			leaked = s
		}
	}
	return leaked
}

func TestParse_M_SAFE_02_UnclosedAndStray(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{":::callout tone=info\nx\n", "never closed"},
		{":::layout\n:::column\nx\n:::\n", "never closed"},
		{"x\n\n:::\n", "closing ::: with no open directive"},
		{":::callout tone=info\n:::\n:::\n", "closing ::: with no open directive"},
		{"```\ncode\n", "unclosed code fence"},
		{"```lk-raw\n{\"kind\":\"rule\"}\n", "unclosed code fence"},
	} {
		t.Run("M-SAFE-02 "+c.want, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
	// The line number points at the problem.
	_, err := parse("a\n\nb\n\n:::secret\nc\n")
	var se *SyntaxError
	if !errors.As(err, &se) || se.Line != 5 {
		t.Errorf("got %v, want line 5", err)
	}
}

func TestParse_M_SAFE_03_HTMLReferencesFootnotes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"<div>\nx\n</div>\n", "raw HTML"},
		{"<!-- comment -->\n", "raw HTML"},
		{"a <b>bold</b> c\n", "raw HTML"},
		{"a <br/> c\n", "raw HTML"},
		{"[x]: https://example.com\n", "link reference definitions"},
		{"Text\n\n[x]: https://example.com \"T\"\n", "link reference definitions"},
		{"Note[^1]\n", "footnotes"},
		{"[^1]: The note.\n", "footnotes"},
		{"[a](u \"title\")\n", "link titles"},
		{"![a](u \"title\")\n", "image titles"},
	} {
		t.Run("M-SAFE-03 "+c.want, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
	// Escaped, they're text; an undefined reference is literal text.
	got := mustJSON(t, mustParse(t, "\\<div> \\[^1] [x][y]\n"))
	if want := `[{"kind":"paragraph","content":[{"text":"\u003cdiv\u003e [^1] [x][y]"}]}]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParse_M_SAFE_05_Images(t *testing.T) {
	for _, src := range []string{
		"Look: ![a](u)\n",
		"![a](u) and text\n",
		"![a](u)![b](v)\n",
		"# Title ![a](u)\n",
		"| h |\n| --- |\n| ![a](u) |\n",
		"[![a](u)](v)\n",
		"**![a](u)**\n",
		"- item ![a](u)\n",
	} {
		t.Run("M-SAFE-05 "+src, func(t *testing.T) { parseFails(t, src, "image") })
	}
	if _, ok := mustParse(t, "![a](u)\n")[0].(*blocks.ImageBlock); !ok {
		t.Error("an image alone in its paragraph is an image block")
	}
}

func TestParse_M_SAFE_07_Limits(t *testing.T) {
	t.Run("M-SAFE-07 over 1 MiB", func(t *testing.T) {
		parseFails(t, strings.Repeat("a", 1<<20+1), "over 1 MiB")
		mustParse(t, strings.Repeat("a", 1<<20))
	})
	deep := func(n int, open, close string) string {
		return strings.Repeat(open, n) + "x\n" + strings.Repeat(close, n)
	}
	for _, c := range []struct {
		name string
		src  func(n int) string
	}{
		{"quotes", func(n int) string { return strings.Repeat(">", n) + " x\n" }},
		{"directives", func(n int) string { return deep(n, ":::secret\n", ":::\n") }},
		{"lists", func(n int) string {
			var b strings.Builder
			for i := range n {
				b.WriteString(strings.Repeat("  ", i) + "- x\n")
			}
			return b.String()
		}},
	} {
		t.Run("M-SAFE-07 depth "+c.name, func(t *testing.T) {
			// A paragraph inside n containers is at depth n+1; lists count
			// list and item.
			if _, err := parse(c.src(maxDepth - 1)); err != nil && c.name != "lists" {
				t.Errorf("depth %d rejected: %v", maxDepth, err)
			}
			parseFails(t, c.src(maxDepth+1), "nested deeper than 64")
		})
	}
	t.Run("M-SAFE-07 inline depth", func(t *testing.T) {
		parseFails(t, deep(200, "[", "]{underline}"), "nested deeper than 64")
	})
	// Pathological 1 MiB inputs are rejected or parsed quickly, with no
	// stack overflow.
	for name, src := range map[string]string{
		"quotes":     strings.Repeat(">", 1<<20-2) + "x",
		"lists":      strings.Repeat("- ", 1<<19-1) + "x",
		"directives": strings.Repeat(":::secret\n", 1<<20/10),
		"brackets":   strings.Repeat("[", 1<<20),
		"spans":      strings.Repeat("[a]{underline}", 1<<20/14),
		"nested":     strings.Repeat("[", 1<<19-10) + strings.Repeat("]{x}", 1<<17),
		"emphasis":   strings.Repeat("*a ", 1<<20/3),
		"braces":     strings.Repeat("{\"", 1<<19),
		"backticks":  strings.Repeat("`a", 1<<19),
		"lines":      strings.Repeat("a\n", 1<<19),
	} {
		t.Run("M-SAFE-07 large "+name, func(t *testing.T) {
			start := time.Now()
			_, _ = parse(src)
			if d := time.Since(start); d > 10*time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
}

// M-OPQ-02: an lk-raw fence holds exactly one JSON object with a string
// kind.
func TestParse_M_OPQ_02_RawFence(t *testing.T) {
	for _, body := range []string{"", "[]", "{}", `{"kind":1}`, `{"kind":"rule"} {"kind":"rule"}`, `{"kind":`, "null", `"rule"`, `{"kind":"rule"},`} {
		t.Run("M-OPQ-02 "+body, func(t *testing.T) {
			if _, err := parse("```lk-raw\n" + body + "\n```\n"); err == nil {
				t.Error("accepted")
			}
			if _, err := parse(":::infobox\n```lk-raw\n" + body + "\n```\n:::\n"); err == nil {
				t.Error("accepted in an infobox")
			}
		})
	}
	got := mustParse(t, "```lk-raw\n{\"kind\":\"hologram\",\"id\":\"h\",\"x\":[1]}\n```\n")
	if r, ok := got[0].(*blocks.RawBlock); !ok || r.KindName != "hologram" {
		t.Errorf("got %s", mustJSON(t, got))
	}
	// lk-raw info strings are exact.
	parseFails(t, "```lk-raw extra\n{\"kind\":\"rule\"}\n```\n", "info string alone")
}

func TestParse_M_INL_03_UnknownSpanAndHTML(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"[x]{glow}", "unknown attribute glow"},
		{"[x]{underline glow=1}", "unknown attribute glow"},
		{"[x]{#id}", "unknown attribute #id"},
		{"[x]{underline underline}", "duplicate attribute"},
		{"[x]{color}", "needs a value"},
		{"a <span>b</span>", "raw HTML"},
		{"a {b", `unescaped "{"`},
		{"{a} b", "isn't allowed here"},
		{"**[x]{mention=a}**", "a mention can't be inside"},
		{"[[x]{mention=a}](u)", "a mention can't be inside"},
		{"[**x**]{mention=a}", "plain text"},
	} {
		t.Run("M-INL-03 "+c.src, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
}
