package markdown

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// shape is one block (as wire JSON) and how it must render: in Markdown
// form ("md"), as lk-raw ("raw"), or as ::unsupported ("unsupported").
type shape struct {
	name string
	json string
	want string
}

const (
	P  = `{"kind":"paragraph","content":[{"text":"x"}]}`
	P2 = `{"kind":"paragraph","content":[{"text":"y"}]}`
)

var shapes = []shape{
	// Paragraphs and inline text.
	{"paragraph", `{"kind":"paragraph","content":[{"text":"Hello"}]}`, "md"},
	{"paragraph empty", `{"kind":"paragraph","content":[]}`, "md"},
	{"paragraph empty with attrs", `{"kind":"paragraph","align":"center","background":"#ffffff","content":[]}`, "md"},
	{"paragraph align left", `{"kind":"paragraph","align":"left","content":[{"text":"a"}]}`, "md"},
	{"paragraph background empty string", `{"kind":"paragraph","background":"","content":[{"text":"a"}]}`, "md"},
	{"paragraph unknown align", `{"kind":"paragraph","align":"justify","content":[{"text":"a"}]}`, "raw"},
	{"paragraph unmodelled field", `{"kind":"paragraph","content":[{"text":"a"}],"legacy":1}`, "raw"},
	{"run unmodelled field", `{"kind":"paragraph","content":[{"text":"a","glow":true}]}`, "raw"},
	{"raw run", `{"kind":"paragraph","content":[{"emoji":"x"}]}`, "raw"},
	{"empty text run", `{"kind":"paragraph","content":[{"text":""}]}`, "raw"},
	{"adjacent equal runs", `{"kind":"paragraph","content":[{"text":"a"},{"text":"b"}]}`, "raw"},
	{"newline in text", `{"kind":"paragraph","content":[{"text":"a\nb"}]}`, "raw"},
	{"trailing break", `{"kind":"paragraph","content":[{"text":"a"},{"break":true}]}`, "raw"},
	{"leading break", `{"kind":"paragraph","content":[{"break":true},{"text":"a"}]}`, "md"},
	{"double break", `{"kind":"paragraph","content":[{"text":"a"},{"break":true},{"break":true},{"text":"b"}]}`, "md"},
	{"marks", `{"kind":"paragraph","content":[{"text":"b","bold":true},{"text":" i","italic":true},{"text":"s","strike":true},{"text":"c","code":true},{"text":"u","underline":true},{"text":"2","sub":true},{"text":"3","sup":true},{"text":"col","color":"#EC4899"},{"text":"hl","highlight":"#F59E0B"}]}`, "md"},
	{"all marks one run", `{"kind":"paragraph","content":[{"text":"z","bold":true,"italic":true,"underline":true,"strike":true,"code":true,"link":"https://x.test","sub":true,"sup":true,"color":"#000000","highlight":"#111111"}]}`, "md"},
	{"bold with edge spaces", `{"kind":"paragraph","content":[{"text":"Hello"},{"text":" world ","bold":true},{"text":"!"}]}`, "md"},
	{"nested marks", `{"kind":"paragraph","content":[{"text":"a","bold":true},{"text":"b","bold":true,"italic":true},{"text":"c","italic":true}]}`, "md"},
	{"bold punctuation inside word", `{"kind":"paragraph","content":[{"text":"a"},{"text":"\"q\"","bold":true},{"text":"b"}]}`, "md"},
	{"link", `{"kind":"paragraph","content":[{"text":"see "},{"text":"here","link":"https://x.test/a_(b)?c=1&d=2"}]}`, "md"},
	{"link with spaces and entity", `{"kind":"paragraph","content":[{"text":"x","link":"https://x.test/a b&amp;c<d>"}]}`, "md"},
	{"empty link", `{"kind":"paragraph","content":[{"text":"x","link":""}]}`, "md"},
	{"lk link", `{"kind":"paragraph","content":[{"text":"x","link":"lk://resource/abc"}]}`, "md"},
	{"two links", `{"kind":"paragraph","content":[{"text":"a","link":"u1"},{"text":"b","link":"u2"}]}`, "md"},
	{"code with backticks", "{\"kind\":\"paragraph\",\"content\":[{\"text\":\"a `` b\",\"code\":true}]}", "md"},
	{"code edge spaces", `{"kind":"paragraph","content":[{"text":" x ","code":true}]}`, "md"},
	{"code one space", `{"kind":"paragraph","content":[{"text":" ","code":true}]}`, "md"},
	{"code starting backtick", "{\"kind\":\"paragraph\",\"content\":[{\"text\":\"`a\",\"code\":true}]}", "md"},
	{"text needing escapes", `{"kind":"paragraph","content":[{"text":"# not *a* _heading_ [x](y) {z} <b> \\ ~s~ ` + "`c`" + ` | &amp; &#35; !"}]}`, "md"},
	{"line start markers", `{"kind":"paragraph","content":[{"text":"- a"},{"break":true},{"text":"1. b"},{"break":true},{"text":"> c"},{"break":true},{"text":"+ d"},{"break":true},{"text":"==="},{"break":true},{"text":":::callout"},{"break":true},{"text":"    code?"},{"break":true},{"text":"---"}]}`, "md"},
	{"edge whitespace", `{"kind":"paragraph","content":[{"text":"\t two  spaces  "}]}`, "md"},
	{"only spaces", `{"kind":"paragraph","content":[{"text":"   "}]}`, "md"},
	{"bang before span", `{"kind":"paragraph","content":[{"text":"wow!"},{"text":"x","underline":true}]}`, "md"},
	{"bang before link", `{"kind":"paragraph","content":[{"text":"wow!"},{"text":"x","link":"u"}]}`, "md"},
	{"unicode", `{"kind":"paragraph","content":[{"text":"Château — ★ 日本"}]}`, "md"},
	{"control character", `{"kind":"paragraph","content":[{"text":"a\u0001b"}]}`, "raw"},
	{"mention", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1","name":"Princess Bubblegum"}}]}`, "md"},
	{"mention no name", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1"}}]}`, "md"},
	{"mention empty name", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1","name":""}}]}`, "md"},
	{"mention label", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1","text":"PB"}}]}`, "md"},
	{"mention empty label", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1","text":""}}]}`, "md"},
	{"mention label name doc colors", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb 1","documentId":"d1","text":"PB [x]","name":"Bubble*gum"},"color":"#ff0000","highlight":"#00ff00"}]}`, "md"},
	{"mention unmodelled", `{"kind":"paragraph","content":[{"mention":{"resourceId":"pb1","extra":1}}]}`, "raw"},
	{"mentions adjacent", `{"kind":"paragraph","content":[{"mention":{"resourceId":"a"}},{"mention":{"resourceId":"b"}}]}`, "md"},

	// Headings.
	{"heading", `{"kind":"heading","level":2,"content":[{"text":"Title"}]}`, "md"},
	{"heading all levels 6", `{"kind":"heading","level":6,"content":[{"text":"T #1 #"}]}`, "md"},
	{"heading empty", `{"kind":"heading","level":1,"content":[]}`, "md"},
	{"heading empty attrs", `{"kind":"heading","level":3,"align":"center","content":[]}`, "md"},
	{"heading attrs", `{"kind":"heading","level":1,"align":"right","background":"#3B82F6","content":[{"text":"x ","bold":true},{"text":"y}"}]}`, "md"},
	{"heading level 7", `{"kind":"heading","level":7,"content":[{"text":"x"}]}`, "raw"},
	{"heading level 0", `{"kind":"heading","level":0,"content":[{"text":"x"}]}`, "raw"},
	{"heading break", `{"kind":"heading","level":1,"content":[{"text":"a"},{"break":true},{"text":"b"}]}`, "raw"},

	// Quotes, code, rules.
	{"quote", `{"kind":"quote","children":[` + P + `,` + P2 + `]}`, "md"},
	{"quote color", `{"kind":"quote","color":"#ff0000","children":[` + P + `]}`, "md"},
	{"quote empty", `{"kind":"quote","children":[]}`, "md"},
	{"quote nested", `{"kind":"quote","color":"#000000","children":[{"kind":"quote","color":"#111111","children":[` + P + `]}]}`, "md"},
	{"quote holding everything", `{"kind":"quote","children":[{"kind":"heading","level":2,"content":[{"text":"h"}]},{"kind":"code","text":"a\n::: b\n> b"},{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `]}]},{"kind":"callout","tone":"note","children":[` + P + `]},{"kind":"rule"}]}`, "md"},
	{"code", `{"kind":"code","language":"go","text":"func main() {}\n"}`, "md"},
	{"code no language", `{"kind":"code","text":"x"}`, "md"},
	{"code empty", `{"kind":"code","text":""}`, "md"},
	{"code newline only", `{"kind":"code","text":"\n"}`, "md"},
	{"code with fences", "{\"kind\":\"code\",\"language\":\"md\",\"text\":\"```\\n````\\n:::callout\\n\"}", "md"},
	// M-SAFE-02: a code line that would open a secret is rejected in a
	// fence (a dropped closing fence must not publish a secret as code),
	// so such code is lk-raw.
	{"code with a closing fence line", `{"kind":"code","text":"a\n:::\n> b"}`, "raw"},
	// M-SAFE-02: "::secret" is accepted only where it opens a secret, so
	// text holding it is exact only as lk-raw.
	{"text mentioning ::secret", `{"kind":"paragraph","content":[{"text":"The ::Secret word"}]}`, "raw"},
	{"code with a secret fence line", "{\"kind\":\"code\",\"language\":\"md\",\"text\":\"```\\n````\\n:::secret\\n\"}", "raw"},
	{"code with tabs", `{"kind":"code","text":"\tindented\n  two"}`, "md"},
	{"code language spaces", `{"kind":"code","language":"c sharp","text":"x"}`, "md"},
	{"code language empty", `{"kind":"code","language":"","text":"x"}`, "raw"},
	{"code language lk-raw", `{"kind":"code","language":"lk-raw","text":"{}"}`, "raw"},
	{"code CR", `{"kind":"code","text":"a\rb"}`, "raw"},
	{"rule", `{"kind":"rule"}`, "md"},

	// Lists.
	{"bullet list", `{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `]},{"kind":"listItem","children":[` + P2 + `]}]}`, "md"},
	{"bullet empty", `{"kind":"bulletList","children":[]}`, "raw"},
	{"bullet item empty", `{"kind":"bulletList","children":[{"kind":"listItem","children":[]},{"kind":"listItem","children":[` + P + `]}]}`, "md"},
	{"bullet item empty paragraph", `{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[]}]}]}`, "md"},
	{"bullet item two paragraphs", `{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `,` + P2 + `]}]}`, "md"},
	{"bullet item code first", `{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"code","text":"x"}]}]}`, "md"},
	{"bullet nested lists", `{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `,{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P2 + `]}]},{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `]}]},{"kind":"orderedList","start":3,"children":[{"kind":"listItem","children":[` + P + `]}]}]}]}`, "md"},
	{"bullet with break", `{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"a"},{"break":true},{"text":"- b"}]}]}]}`, "md"},
	{"bullet non-item child", `{"kind":"bulletList","children":[` + P + `]}`, "raw"},
	{"ordered", `{"kind":"orderedList","children":[{"kind":"listItem","children":[` + P + `]}]}`, "md"},
	{"ordered start 1", `{"kind":"orderedList","start":1,"children":[{"kind":"listItem","children":[` + P + `]}]}`, "raw"},
	{"ordered start 0", `{"kind":"orderedList","start":0,"children":[{"kind":"listItem","children":[` + P + `]}]}`, "md"},
	{"ordered start 9", `{"kind":"orderedList","start":9,"children":[{"kind":"listItem","children":[` + P + `]},{"kind":"listItem","children":[` + P2 + `]}]}`, "md"},
	{"ordered negative", `{"kind":"orderedList","start":-1,"children":[{"kind":"listItem","children":[` + P + `]}]}`, "raw"},
	{"task list", `{"kind":"taskList","children":[{"kind":"taskItem","checked":false,"content":[{"text":"a"}]},{"kind":"taskItem","checked":true,"content":[{"text":"[x] b","bold":true}]}]}`, "md"},
	{"task empty item", `{"kind":"taskList","children":[{"kind":"taskItem","checked":false,"content":[]}]}`, "md"},
	{"task item leading space", `{"kind":"taskList","children":[{"kind":"taskItem","checked":true,"content":[{"text":" a"}]}]}`, "md"},
	{"task nested", `{"kind":"taskList","children":[{"kind":"taskItem","checked":false,"content":[{"text":"a"}]},{"kind":"taskList","children":[{"kind":"taskItem","checked":true,"content":[{"text":"b"}]}]},{"kind":"taskList","children":[{"kind":"taskItem","checked":true,"content":[{"text":"c"}]},{"kind":"taskList","children":[{"kind":"taskItem","checked":true,"content":[{"text":"d"}]}]}]},{"kind":"taskItem","checked":false,"content":[{"text":"e"},{"break":true},{"text":"f"}]}]}`, "md"},
	{"task nested first", `{"kind":"taskList","children":[{"kind":"taskList","children":[{"kind":"taskItem","checked":false,"content":[{"text":"a"}]}]}]}`, "raw"},
	{"task empty", `{"kind":"taskList","children":[]}`, "raw"},

	// Tables.
	{"table gfm", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[{"kind":"paragraph","content":[{"text":"H|1","bold":true}]}]},{"kind":"tableCell","header":true,"children":[{"kind":"paragraph","content":[]}]}]},{"kind":"tableRow","children":[{"kind":"tableCell","children":[{"kind":"paragraph","content":[{"text":"a|b","code":true}]}]},{"kind":"tableCell","children":[{"kind":"paragraph","content":[{"text":" x ","link":"u|v"}]}]}]}]}`, "md"},
	{"table header only", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[` + P + `]}]}]}`, "md"},
	{"table spans", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"colspan":2,"rowspan":1,"children":[` + P + `]}]},{"kind":"tableRow","children":[{"kind":"tableCell","header":false,"children":[` + P + `,` + P2 + `]},{"kind":"tableCell","children":[]}]}]}`, "md"},
	{"table no header", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","children":[` + P + `]}]}]}`, "md"},
	{"table ragged", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[` + P + `]},{"kind":"tableCell","header":true,"children":[` + P + `]}]},{"kind":"tableRow","children":[{"kind":"tableCell","children":[` + P + `]}]}]}`, "md"},
	{"table empty", `{"kind":"table","children":[]}`, "md"},
	{"table cell attrs paragraph", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[{"kind":"paragraph","align":"center","content":[{"text":"x"}]}]}]}]}`, "md"},
	{"table cell break", `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[{"kind":"paragraph","content":[{"text":"a"},{"break":true},{"text":"b"}]}]}]}]}`, "md"},
	{"table row non-cell", `{"kind":"table","children":[{"kind":"tableRow","children":[` + P + `]}]}`, "raw"},

	// LegendKeeper blocks.
	{"callout", `{"kind":"callout","tone":"warning","children":[` + P + `]}`, "md"},
	{"callout custom", `{"kind":"callout","tone":"custom","icon":"dragon","color":"#aa0000","children":[` + P + `,{"kind":"callout","tone":"info","children":[]}]}`, "md"},
	{"callout unknown tone", `{"kind":"callout","tone":"loud","children":[]}`, "raw"},
	{"callout empty tone", `{"kind":"callout","tone":"","children":[]}`, "raw"},
	{"expand", `{"kind":"expand","title":"Hidden \"details\" {x}","children":[` + P + `]}`, "md"},
	{"expand empty title", `{"kind":"expand","title":"","children":[]}`, "md"},
	{"expand newline title", `{"kind":"expand","title":"a\nb","children":[]}`, "raw"},
	{"layout", `{"kind":"layout","children":[{"kind":"layoutColumn","width":50,"background":"#3B82F6","children":[` + P + `]},{"kind":"layoutColumn","width":33.333333333333336,"children":[]},{"kind":"layoutColumn","width":1e21,"children":[` + P2 + `]}]}`, "md"},
	{"layout non-column", `{"kind":"layout","children":[` + P + `]}`, "raw"},
	{"secret", `{"kind":"secret","layout":"wrapLeft","width":30,"children":[` + P + `]}`, "md"},
	{"secret empty", `{"kind":"secret","children":[]}`, "raw"},
	{"secret nested", `{"kind":"secret","children":[{"kind":"secret","children":[{"kind":"code","text":":::"}]},` + P + `]}`, "md"},
	{"textfield", `{"kind":"textField","title":"Origin","isTitleHidden":false,"layout":"center","width":100,"children":[` + P + `]}`, "md"},
	{"textfield empty", `{"kind":"textField","children":[]}`, "md"},
	{"image", `{"kind":"image","url":"https://assets.legendkeeper.com/p.png"}`, "md"},
	{"image all fields", `{"kind":"image","url":"https://assets.legendkeeper.com/p (1).png","alt":"A [map]","fill":false,"title":"","isTitleHidden":true,"layout":"wrapRight","width":40}`, "md"},
	{"image empty alt", `{"kind":"image","url":"u","alt":""}`, "md"},
	{"image empty url", `{"kind":"image","url":""}`, "md"},
	{"embed", `{"kind":"embed","provider":"youtube","url":"https://www.youtube.com/watch?v=x","title":"T","isTitleHidden":false,"layout":"default","width":0}`, "md"},
	{"embed unknown provider", `{"kind":"embed","provider":"vimeo","url":"u"}`, "raw"},
	{"keyvalue", `{"kind":"keyValue","display":"grid","title":"Stats","isTitleHidden":true,"layout":"alignEnd","width":10,"rows":[{"key":"Species","content":[{"text":"Magic dog"}]},{"key":"Plays","content":[{"text":"Video games","bold":true}],"text":"Video games"},{"key":"a :: b","content":[{"text":"c :: d"}],"text":"other"},{"key":"","content":[]},{"key":"Text only","text":"t"},{"key":"Nothing"},{"key":"  pad  ","content":[{"text":"{x}"}]}]}`, "md"},
	{"keyvalue empty", `{"kind":"keyValue","rows":[]}`, "md"},
	{"keyvalue row unmodelled", `{"kind":"keyValue","rows":[{"key":"a","flag":1}]}`, "raw"},
	{"keyvalue empty key no content", `{"kind":"keyValue","rows":[{"key":""}]}`, "raw"},
	{"meter", `{"kind":"meter","mode":"gauge-180","showText":true,"showIcon":false,"showMax":true,"title":"Vitals","items":[{"name":"HP","icon":"heart","current":3,"max":5,"color":"#ff0000"},{"name":"MP :: x","icon":"","ratingIcon":"star","current":0.5,"max":-1e-7}]}`, "md"},
	{"meter empty", `{"kind":"meter","mode":"bar","items":[]}`, "md"},
	{"meter unknown mode", `{"kind":"meter","mode":"pie","items":[]}`, "raw"},
	{"links", `{"kind":"resourceLink","title":"See also","items":[{"resourceId":"finn1"},{"resourceId":"jake 1","documentId":"d9"}]}`, "md"},
	{"links empty", `{"kind":"resourceLink","items":[]}`, "md"},
	{"collection items", `{"kind":"resourceCollection","title":"Residents","isTitleHidden":true,"items":[{"resourceId":"finn1"}]}`, "md"},
	{"collection filter", `{"kind":"resourceCollection","filter":{"type":"filter","id":"f1","filterType":"tag","operator":"has_any","tags":["Kingdom"],"future":{"x":[1,2]}}}`, "md"},
	{"collection items and filter", `{"kind":"resourceCollection","items":[{"resourceId":"a"}],"filter":{"type":"filter","id":"f1","filterType":"composite","operator":"and","filters":[{"type":"filter","id":"g","filterType":"text","value":"x` + "```" + `"}]}}`, "md"},
	{"collection empty items", `{"kind":"resourceCollection","items":[]}`, "raw"},
	{"collection nothing", `{"kind":"resourceCollection"}`, "md"},
	{"map", `{"kind":"map","resourceId":"atlas","documentId":"atlasMap","viewport":{"center":[12.5,-3],"zoom":2},"displayHeight":400.5,"fill":true,"title":"World","isTitleHidden":false,"layout":"wrapLeft","width":50}`, "md"},
	{"map empty", `{"kind":"map"}`, "md"},
	{"map odd viewport", `{"kind":"map","viewport":{"center":[1],"zoom":2}}`, "raw"},
	{"infobox", `{"kind":"infobox","layout":"wrapRight","width":40,"rows":[
		{"kind":"image","id":"r1","rank":"a","url":"https://assets.legendkeeper.com/p.png","alt":"Portrait","fill":true,"title":"Portrait","isTitleHidden":true,"isHidden":false},
		{"kind":"keyValue","title":"Stats","display":"inline","rows":[{"key":"Species","content":[{"text":"Magic dog"}]}]},
		{"kind":"textField","title":"Origin","children":[` + P + `]},
		{"kind":"meter","mode":"rating","items":[{"name":"Fame","icon":"star","current":2,"max":5}]},
		{"kind":"resourceLink","isHidden":true,"items":[{"resourceId":"a"}]},
		{"kind":"resourceCollection","filter":{"type":"filter","id":"f","filterType":"tag"}},
		{"kind":"embed","provider":"spotify","url":"https://open.spotify.com/x"},
		{"kind":"map","resourceId":"a","documentId":"b","viewport":{"center":[0,0],"zoom":1}},
		{"kind":"unsupported","id":"u2","title":"Weird"},
		{"kind":"hologram","id":"h2","beam":true},
		{"kind":"textField","children":[],"extra":1}]}`, "md"},
	{"infobox empty", `{"kind":"infobox","rows":[]}`, "md"},

	// Opaque blocks.
	{"unsupported", `{"kind":"unsupported","id":"u1"}`, "unsupported"},
	{"unsupported quoted id", `{"kind":"unsupported","id":"a b"}`, "unsupported"},
	{"unsupported unmodelled", `{"kind":"unsupported","id":"u3","x":1}`, "raw"},
	{"unknown kind", `{"kind":"hologram","id":"h1","nested":{"a":[1,"` + "```" + `"]}}`, "raw"},
	{"known kind bad shape", `{"kind":"paragraph","content":5}`, "raw"},
}

func TestRender_M_OPQ_01_EveryShape(t *testing.T) {
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			page := listOf(t, "["+s.json+"]")
			md := roundTrip(t, page)
			switch s.want {
			case "md":
				if hasRaw(md) && !strings.Contains(s.json, "hologram") && !strings.Contains(s.json, `"extra"`) {
					t.Errorf("rendered as lk-raw, want Markdown:\n%s", md)
				}
			case "raw":
				if !hasRaw(md) {
					t.Errorf("want lk-raw:\n%s", md)
				}
			case "unsupported":
				if !strings.HasPrefix(md, "::unsupported id=") {
					t.Errorf("want ::unsupported:\n%s", md)
				}
			}
			if rendersOpaque(page[0]) != (s.want != "md") {
				t.Errorf("rendersOpaque = %v for %s", s.want == "md", s.want)
			}
		})
	}
}

// Random trees: every shape, nested in every container, round-trips, and
// a block that renders in Markdown form alone still does when nested.
func TestRender_M_OPQ_01_RandomTrees(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	wrap := []func(kids string) string{
		func(k string) string { return `{"kind":"quote","children":[` + k + `]}` },
		func(k string) string { return `{"kind":"callout","tone":"note","children":[` + k + `]}` },
		func(k string) string { return `{"kind":"secret","children":[` + k + `]}` },
		func(k string) string { return `{"kind":"expand","title":"t","children":[` + k + `]}` },
		func(k string) string { return `{"kind":"textField","children":[` + k + `]}` },
		func(k string) string {
			return `{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `,` + k + `]}]}`
		},
		func(k string) string {
			return `{"kind":"orderedList","children":[{"kind":"listItem","children":[` + k + `]}]}`
		},
		func(k string) string {
			return `{"kind":"layout","children":[{"kind":"layoutColumn","children":[` + k + `]},{"kind":"layoutColumn","children":[]}]}`
		},
		func(k string) string {
			return `{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","children":[` + k + `]}]}]}`
		},
		func(k string) string {
			return `{"kind":"infobox","rows":[{"kind":"textField","children":[` + k + `]}]}`
		},
	}
	var tree func(depth int) (string, bool)
	tree = func(depth int) (string, bool) {
		if depth == 0 || r.IntN(3) == 0 {
			s := shapes[r.IntN(len(shapes))]
			for s.want == "unsupported" || strings.Contains(s.json, `"id"`) {
				s = shapes[r.IntN(len(shapes))]
			}
			return s.json, s.want == "md"
		}
		var kids []string
		md := true
		for range 1 + r.IntN(3) {
			k, ok := tree(depth - 1)
			kids = append(kids, k)
			md = md && ok
		}
		return wrap[r.IntN(len(wrap))](strings.Join(kids, ",")), md
	}
	for range 400 {
		s, md := tree(4)
		out := roundTrip(t, listOf(t, "["+s+"]"))
		if md && hasRaw(out) && !strings.Contains(s, "hologram") && !strings.Contains(s, `"extra"`) {
			t.Errorf("nested Markdown-form blocks went lk-raw:\n%s", out)
		}
	}
}

// Every shape also round-trips next to every other: adjacent lists don't
// merge, attribute lines stay with their block.
func TestRender_M_API_01_Neighbours(t *testing.T) {
	var all []string
	for _, s := range shapes {
		all = append(all, s.json)
	}
	roundTrip(t, listOf(t, "["+strings.Join(all, ",")+"]"))
	lists := []string{
		`{"kind":"bulletList","children":[{"kind":"listItem","children":[` + P + `]}]}`,
		`{"kind":"taskList","children":[{"kind":"taskItem","checked":false,"content":[{"text":"t"}]}]}`,
		`{"kind":"orderedList","children":[{"kind":"listItem","children":[` + P + `]}]}`,
		`{"kind":"orderedList","start":2,"children":[{"kind":"listItem","children":[` + P + `]}]}`,
	}
	for _, a := range lists {
		for _, b := range lists {
			for _, c := range lists {
				md := roundTrip(t, listOf(t, "["+a+","+b+","+c+"]"))
				if hasRaw(md) {
					t.Errorf("adjacent lists went lk-raw:\n%s", md)
				}
			}
		}
	}
}
