package markdown

import (
	"errors"
	"strings"
	"testing"

	"github.com/swiftj/golk/lk/blocks"
)

// mustParse parses src and fails the test on an error.
func mustParse(t *testing.T, src string) blocks.List {
	t.Helper()
	l, err := parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return l
}

// parseFails checks that src is rejected with a *SyntaxError, no blocks,
// and a reason containing want.
func parseFails(t *testing.T, src, want string) {
	t.Helper()
	l, err := parse(src)
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("parse %q: got %v, %s; want a *SyntaxError", src, err, mustJSON(t, l))
	}
	if l != nil {
		t.Fatalf("parse %q: error with blocks", src)
	}
	if !strings.Contains(se.Reason, want) {
		t.Fatalf("parse %q: reason %q, want it to contain %q", src, se.Reason, want)
	}
}

// The examples in LK_MARKDOWN.md §2.3 parse to the blocks they show.
func TestParse_M_DIR_01_SpecExamples(t *testing.T) {
	src := "" +
		":::callout tone=warning\nBeware the **Lich**.\n:::\n\n" +
		":::callout tone=custom icon=dragon color=#aa0000\nHere be dragons.\n:::\n\n" +
		":::expand title=\"Hidden details\"\nBody Markdown.\n:::\n\n" +
		":::layout\n:::column width=50 background=#3B82F6\nCandy Castle\n:::\n:::column width=50\nCandy Village\n:::\n:::\n\n" +
		":::secret\nThe Lich is not really gone.\n:::\n\n" +
		":::textfield title=\"Origin\"\nGot his powers from a magic mud puddle.\n:::\n\n" +
		":::keyvalue title=Stats display=grid\n- Species :: Magic dog\n- Plays :: **Video games**\n:::\n\n" +
		":::meter mode=bar title=Vitals\n- HP :: 3/5 icon=heart color=#ff0000\n:::\n\n" +
		":::links title=\"See also\"\n- @finn1\n- @jake1 doc=d9\n:::\n\n" +
		":::collection title=Residents isTitleHidden=true\n- @finn1\n:::\n\n" +
		":::collection\n```lk-filter\n{\"type\":\"filter\",\"id\":\"f1\",\"filterType\":\"tag\",\"operator\":\"has_any\",\"tags\":[\"Kingdom\"]}\n```\n:::\n\n" +
		"::embed provider=youtube url=\"https://www.youtube.com/watch?v=x\"\n" +
		"::map resourceId=atlas documentId=atlasMap displayHeight=400 viewport=\"12.5,-3,2\"\n" +
		"::map\n\n" +
		":::infobox layout=wrapRight width=40\n![Portrait](https://assets.legendkeeper.com/p.png){fill title=Portrait isTitleHidden=true}\n:::keyvalue title=Stats\n- Species :: Magic dog\n:::\n:::textfield title=Origin\nRolled in a magic mud puddle.\n:::\n:::\n"
	want := `[{"kind":"callout","tone":"warning","children":[{"kind":"paragraph","content":[{"text":"Beware the "},{"text":"Lich","bold":true},{"text":"."}]}]},` +
		`{"kind":"callout","tone":"custom","icon":"dragon","color":"#aa0000","children":[{"kind":"paragraph","content":[{"text":"Here be dragons."}]}]},` +
		`{"kind":"expand","title":"Hidden details","children":[{"kind":"paragraph","content":[{"text":"Body Markdown."}]}]},` +
		`{"kind":"layout","children":[{"kind":"layoutColumn","width":50,"background":"#3B82F6","children":[{"kind":"paragraph","content":[{"text":"Candy Castle"}]}]},{"kind":"layoutColumn","width":50,"children":[{"kind":"paragraph","content":[{"text":"Candy Village"}]}]}]},` +
		`{"kind":"secret","children":[{"kind":"paragraph","content":[{"text":"The Lich is not really gone."}]}]},` +
		`{"kind":"textField","title":"Origin","children":[{"kind":"paragraph","content":[{"text":"Got his powers from a magic mud puddle."}]}]},` +
		`{"kind":"keyValue","display":"grid","rows":[{"key":"Species","content":[{"text":"Magic dog"}]},{"key":"Plays","content":[{"text":"Video games","bold":true}]}],"title":"Stats"},` +
		`{"kind":"meter","mode":"bar","items":[{"name":"HP","icon":"heart","current":3,"max":5,"color":"#ff0000"}],"title":"Vitals"},` +
		`{"kind":"resourceLink","items":[{"resourceId":"finn1"},{"resourceId":"jake1","documentId":"d9"}],"title":"See also"},` +
		`{"kind":"resourceCollection","items":[{"resourceId":"finn1"}],"title":"Residents","isTitleHidden":true},` +
		`{"kind":"resourceCollection","filter":{"type":"filter","id":"f1","filterType":"tag","operator":"has_any","tags":["Kingdom"]}},` +
		`{"kind":"embed","provider":"youtube","url":"https://www.youtube.com/watch?v=x"},` +
		`{"kind":"map","resourceId":"atlas","documentId":"atlasMap","viewport":{"center":[12.5,-3],"zoom":2},"displayHeight":400},` +
		`{"kind":"map"},` +
		`{"kind":"infobox","layout":"wrapRight","width":40,"rows":[{"kind":"image","title":"Portrait","isTitleHidden":true,"url":"https://assets.legendkeeper.com/p.png","alt":"Portrait","fill":true},{"kind":"keyValue","title":"Stats","rows":[{"key":"Species","content":[{"text":"Magic dog"}]}]},{"kind":"textField","title":"Origin","children":[{"kind":"paragraph","content":[{"text":"Rolled in a magic mud puddle."}]}]}]}]`
	if got := mustJSON(t, mustParse(t, src)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestParse_M_DIR_01_AttributeValues(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`:::expand title="a \"b\" \\ c"` + "\n:::\n", `a "b" \ c`},
		{":::expand title=bare\n:::\n", "bare"},
		{`:::expand title="a\|b\}"` + "\n:::\n", "a|b}"},
		{":::expand title=\"\"\n:::\n", ""},
		{":::expand title=Château\n:::\n", "Château"},
	} {
		b := mustParse(t, c.src)[0].(*blocks.ExpandBlock)
		if b.Title != c.want {
			t.Errorf("%q: title %q, want %q", c.src, b.Title, c.want)
		}
	}
	// A bare flag means true; true and false can be written.
	tf := mustParse(t, ":::textfield isTitleHidden\n:::\n\n:::textfield isTitleHidden=true\n:::\n\n:::textfield isTitleHidden=false\n:::\n")
	for i, want := range []bool{true, true, false} {
		if v, ok := tf[i].(*blocks.TextFieldBlock).IsTitleHidden.Get(); !ok || v != want {
			t.Errorf("textfield %d: isTitleHidden %v %v, want %v", i, v, ok, want)
		}
	}
}

// A field not written is unset, never a default.
func TestParse_M_DIR_02_UnwrittenFieldsUnset(t *testing.T) {
	got := mustJSON(t, mustParse(t, ":::callout tone=info\n:::\n\n![](u)\n\n::embed provider=spotify url=u\n\n:::meter mode=bar\n:::\n\n# H\n\nP\n"))
	want := `[{"kind":"callout","tone":"info","children":[]},{"kind":"image","url":"u"},{"kind":"embed","provider":"spotify","url":"u"},{"kind":"meter","mode":"bar","items":[]},{"kind":"heading","level":1,"content":[{"text":"H"}]},{"kind":"paragraph","content":[{"text":"P"}]}]`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// Required fields must be written.
	for _, src := range []string{":::callout\n:::\n", ":::expand\n:::\n", "::embed url=u\n", "::embed provider=youtube\n", ":::meter\n:::\n", ":::meter mode=bar\n- a :: 1/2\n:::\n"} {
		parseFails(t, src, "missing attribute")
	}
}

func TestParse_M_DIR_03_ItemForms(t *testing.T) {
	got := mustJSON(t, mustParse(t, ":::keyvalue\n"+
		"- Plain :: value with *marks* and [a link](u)\n"+
		"- Escaped \\:\\: key :: a :: b\n"+
		"- Break :: one\\\n  two {#r1 text}\n"+
		"- Text :: x {text=\"other\"}\n"+
		"- Only key\n"+
		"- Empty ::\n"+
		":::\n\n"+
		":::meter mode=pool\n- HP :: 2.5/10 #m1 icon=heart ratingIcon=star color=#ff0000\n:::\n\n"+
		":::links\n- @r1 #i1 doc=d1\n- @\"r 2\"\n:::\n"))
	want := `[{"kind":"keyValue","rows":[` +
		`{"key":"Plain","content":[{"text":"value with "},{"text":"marks","italic":true},{"text":" and "},{"text":"a link","link":"u"}]},` +
		`{"key":"Escaped :: key","content":[{"text":"a :: b"}]},` +
		`{"id":"r1","key":"Break","content":[{"text":"one"},{"break":true},{"text":"two"}],"text":"one\ntwo"},` +
		`{"key":"Text","content":[{"text":"x"}],"text":"other"},` +
		`{"key":"Only key"},` +
		`{"key":"Empty","content":[]}]},` +
		`{"kind":"meter","mode":"pool","items":[{"id":"m1","name":"HP","icon":"heart","ratingIcon":"star","current":2.5,"max":10,"color":"#ff0000"}]},` +
		`{"kind":"resourceLink","items":[{"id":"i1","resourceId":"r1","documentId":"d1"},{"resourceId":"r 2"}]}]`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	for _, c := range []struct{ src, want string }{
		{":::keyvalue\n- *bold key* :: v\n:::\n", "a key is plain text"},
		{":::keyvalue\n- a :: b\n\n  second paragraph\n:::\n", "each row is one line"},
		{":::keyvalue\nnot a list\n:::\n", "holds only a list"},
		{":::keyvalue\n1. a :: b\n:::\n", "holds only a list"},
		{":::keyvalue\n- k {text}\n:::\n", "needs a value"},
		{":::meter mode=bar\n- HP :: 3 icon=x\n:::\n", "current/max"},
		{":::meter mode=bar\n- HP 3/5 icon=x\n:::\n", "one line"},
		{":::links\n- finn1\n:::\n", "@resourceId"},
		{":::links\n- @a bogus=1\n:::\n", "unknown attribute bogus"},
		{":::collection\n```js\nx\n```\n:::\n", "lk-filter"},
		{":::collection\n```lk-filter\n[1]\n```\n:::\n", "exactly one JSON object"},
	} {
		t.Run(c.want, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
}

func TestParse_M_DIR_04_AttributeSets(t *testing.T) {
	got := mustJSON(t, mustParse(t, "# Title {#h1 align=center background=#ffffff}\n\n"+
		"Para\n{#p1 align=right}\n\n"+
		"> q\n{color=#ff0000}\n\n"+
		"```js\nx\n```\n{#c1}\n\n---\n{#r1}\n\n- a\n{#l1}\n\n"+
		"![alt](u){#i1 fill width=40}\n"))
	want := `[{"kind":"heading","id":"h1","level":1,"align":"center","background":"#ffffff","content":[{"text":"Title"}]},` +
		`{"kind":"paragraph","id":"p1","align":"right","content":[{"text":"Para"}]},` +
		`{"kind":"quote","color":"#ff0000","children":[{"kind":"paragraph","content":[{"text":"q"}]}]},` +
		`{"kind":"code","id":"c1","language":"js","text":"x"},{"kind":"rule","id":"r1"},` +
		`{"kind":"bulletList","id":"l1","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"a"}]}]}]},` +
		`{"kind":"image","id":"i1","url":"u","alt":"alt","fill":true,"width":40}]`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	for _, c := range []struct{ src, want string }{
		{"Para\n\n{align=center}\n", "must directly follow"},
		{"{align=center}\n", "must directly follow"},
		{"# H\n{align=center}\n", "must directly follow"},
		{":::secret\nx\n:::\n{#s}\n", "must directly follow"},
		{"Para\n{align=center}\n{#p}\n", "must directly follow"},
		{"Para\n{color=#fff}\n", "unknown attribute color"},
		{"Para\n{align=justify}\n", "not one of"},
		{"# Title{align=center}\n", "one space before"},
		{"Para {align=center}\n", "isn't allowed here"},
		{"![a](u) {fill}\n", "alone in its paragraph"},
		{"![a](u){fill}\n{#x}\n", "must directly follow"},
	} {
		t.Run(c.want, func(t *testing.T) { parseFails(t, c.src, c.want) })
	}
}
