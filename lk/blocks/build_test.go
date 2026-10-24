package blocks

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/swiftj/golk/lk/internal/opt"
)

func encodes(t *testing.T, v any, want string) {
	t.Helper()
	if got := string(mustEncode(t, v)); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestRun_C_RUN_01_C_RUN_02_DecodeByKey(t *testing.T) {
	in := `[{"text":"a","bold":true,"link":"https://x"},{"mention":{"resourceId":"r1","documentId":"d1","text":"L","name":"N"},"highlight":"#000000"},{"break":true},{"sparkle":1},{"break":false}]`
	var rs Runs
	if err := json.Unmarshal([]byte(in), &rs); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 5 {
		t.Fatalf("%d runs", len(rs))
	}
	if txt, ok := rs[0].(*Text); !ok || !txt.Bold || txt.Link.Or("") != "https://x" {
		t.Errorf("text run = %#v", rs[0])
	}
	m, ok := rs[1].(*Mention)
	if !ok || m.ResourceID != "r1" || m.DocumentID.Or("") != "d1" || m.Label.Or("") != "L" || m.Name.Or("") != "N" || m.Highlight.Or("") != "#000000" {
		t.Errorf("mention run = %#v", rs[1])
	}
	if _, ok := rs[2].(*Break); !ok {
		t.Errorf("break run = %#v", rs[2])
	}
	for _, i := range []int{3, 4} {
		if _, ok := rs[i].(*RawRun); !ok {
			t.Errorf("run %d = %#v, want RawRun", i, rs[i])
		}
	}
	// Marks are sent only when true; unknown shapes come back unchanged.
	if out := mustEncode(t, rs); !sameJSON(t, out, []byte(in)) {
		t.Errorf("re-encoded %s", out)
	}
	encodes(t, &Text{Text: "x", Bold: false, Code: false}, `{"text":"x"}`)
}

func TestRun_C_RUN_03_MarkHelpers(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  Runs
		want string
	}{
		{"nesting", Bold(Italic("Mathematical!")), `[{"text":"Mathematical!","bold":true,"italic":true}]`},
		{"mixed text", R("What time is it? ", Bold("Adventure time"), "!"),
			`[{"text":"What time is it? "},{"text":"Adventure time","bold":true},{"text":"!"}]`},
		{"mono clears marks", Mono(Bold(Link("https://x", Color("#112233", "c")))), `[{"text":"c","code":true}]`},
		{"later marks skip mono", Bold(Color("#112233", Mono("c"))), `[{"text":"c","code":true}]`},
		{"sub clears sup", Sub(Sup("2")), `[{"text":"2","sub":true}]`},
		{"sup clears sub", Sup(Sub("2")), `[{"text":"2","sup":true}]`},
		{"color colors a mention", Color("#EC4899", MentionOf("pb", "Princess Bubblegum")),
			`[{"mention":{"resourceId":"pb","text":"Princess Bubblegum"},"color":"#EC4899"}]`},
		{"highlight", Highlight("#F59E0B", "made of candy", BR()), `[{"text":"made of candy","highlight":"#F59E0B"},{"break":true}]`},
		{"bold skips mentions and breaks", Bold(MentionOf("r", ""), BR()), `[{"mention":{"resourceId":"r"}},{"break":true}]`},
		{"link", Link("lk://x", "go"), `[{"text":"go","link":"lk://x"}]`},
		{"underline strike", Underline(Strike("u")), `[{"text":"u","underline":true,"strike":true}]`},
	} {
		t.Run(tc.name, func(t *testing.T) { encodes(t, tc.got, tc.want) })
	}
	// Helpers copy: the argument is not changed.
	plain := R("p")
	_ = Bold(plain)
	encodes(t, plain, `[{"text":"p"}]`)
}

func TestRun_C_RUN_04_MentionText(t *testing.T) {
	label := &Mention{ResourceID: "r", Label: opt.Some("Label"), Name: opt.Some("Name")}
	name := &Mention{ResourceID: "r", Name: opt.Some("Name")}
	if got := RunsText(Runs{label, &Text{Text: " / "}, name, &Break{}, &Mention{ResourceID: "r"}}); got != "Label / Name\n" {
		t.Errorf("text = %q", got)
	}
	// A mention read with its name is echoed with it (D-009, A-24).
	encodes(t, name, `{"mention":{"resourceId":"r","name":"Name"}}`)
}

func TestBuild_C_BUILD_01_GuideBuilders(t *testing.T) {
	col := Column(50, "Candy Castle")
	for _, tc := range []struct {
		name string
		b    any
		want string
	}{
		{"p", P("Lives in Lumpy Space."), `{"kind":"paragraph","content":[{"text":"Lives in Lumpy Space."}]}`},
		{"h", H(1, "The Tree Fort"), `{"kind":"heading","level":1,"content":[{"text":"The Tree Fort"}]}`},
		{"quote", Quote("Science is the best kind of magic."), `{"kind":"quote","children":[{"kind":"paragraph","content":[{"text":"Science is the best kind of magic."}]}]}`},
		{"code", Code("x := 1", "go"), `{"kind":"code","language":"go","text":"x := 1"}`},
		{"code no language", Code("x", ""), `{"kind":"code","text":"x"}`},
		{"rule", Rule(), `{"kind":"rule"}`},
		{"ul", UL("Treasure room", []any{"Escort ", MentionOf("pb", ""), " home"}),
			`{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"Treasure room"}]}]},{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"Escort "},{"mention":{"resourceId":"pb"}},{"text":" home"}]}]}]}`},
		{"ol with start", func() any { l := OL("a"); l.Start = opt.Some(3); return l }(),
			`{"kind":"orderedList","start":3,"children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"a"}]}]}]}`},
		{"li with nested list", LI("Outer", UL("Inner")),
			`{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"Outer"}]},{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"Inner"}]}]}]}]}`},
		{"tasks", Tasks(Task(true, "Done")), `{"kind":"taskList","children":[{"kind":"taskItem","checked":true,"content":[{"text":"Done"}]}]}`},
		{"table", Table(TR(TH("H"), TD("C"))),
			`{"kind":"table","children":[{"kind":"tableRow","children":[{"kind":"tableCell","header":true,"children":[{"kind":"paragraph","content":[{"text":"H"}]}]},{"kind":"tableCell","children":[{"kind":"paragraph","content":[{"text":"C"}]}]}]}]}`},
		{"callout", Callout(ToneWarning, "Beware"), `{"kind":"callout","tone":"warning","children":[{"kind":"paragraph","content":[{"text":"Beware"}]}]}`},
		{"expand", Expand("More", "Hidden"), `{"kind":"expand","title":"More","children":[{"kind":"paragraph","content":[{"text":"Hidden"}]}]}`},
		{"layout", Layout(col, Column(0, "Candy Village")),
			`{"kind":"layout","children":[{"kind":"layoutColumn","width":50,"children":[{"kind":"paragraph","content":[{"text":"Candy Castle"}]}]},{"kind":"layoutColumn","children":[{"kind":"paragraph","content":[{"text":"Candy Village"}]}]}]}`},
		{"image", Image("https://assets.example/x.png", "The Tree Fort"), `{"kind":"image","url":"https://assets.example/x.png","alt":"The Tree Fort"}`},
		{"embed", Embed(EmbedYouTube, "https://youtube.example/v"), `{"kind":"embed","provider":"youtube","url":"https://youtube.example/v"}`},
		{"keyValue", KeyValue(KV("Lives in", "The Tree Fort")), `{"kind":"keyValue","rows":[{"key":"Lives in","content":[{"text":"The Tree Fort"}]}]}`},
		{"infobox", Infobox(AsProperty(KeyValue(KV("Species", "Magic dog")), Envelope{Title: opt.Some("Stats")})),
			`{"kind":"infobox","rows":[{"kind":"keyValue","title":"Stats","rows":[{"key":"Species","content":[{"text":"Magic dog"}]}]}]}`},
		{"meter", Meter(MeterBar, MeterItem{Name: "HP", Icon: "heart", Current: 3, Max: 5}),
			`{"kind":"meter","mode":"bar","items":[{"name":"HP","icon":"heart","current":3,"max":5}]}`},
		{"links", Links("finn", ResourceItem{ResourceID: "jake", DocumentID: opt.Some("d")}),
			`{"kind":"resourceLink","items":[{"resourceId":"finn"},{"resourceId":"jake","documentId":"d"}]}`},
		{"collection", Collection("finn"), `{"kind":"resourceCollection","items":[{"resourceId":"finn"}]}`},
		{"collection by filter", CollectionWhere(Filter{ID: "f1", FilterType: "field", Field: opt.Some("parentId"), Operator: opt.Some("equals"), Value: json.RawMessage(`"$self"`)}),
			`{"kind":"resourceCollection","filter":{"type":"filter","id":"f1","filterType":"field","field":"parentId","operator":"equals","value":"$self"}}`},
		{"secret", Secret("The DM's plan"), `{"kind":"secret","children":[{"kind":"paragraph","content":[{"text":"The DM's plan"}]}]}`},
		{"textField", TextField("Origin story"), `{"kind":"textField","children":[{"kind":"paragraph","content":[{"text":"Origin story"}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) { encodes(t, tc.b, tc.want) })
	}
}

func TestBuild_C_BUILD_02_RequiredFirstChild(t *testing.T) {
	// The builders of kinds that require children take the first one as a
	// separate parameter, so Quote() and UL() do not compile.
	for _, tc := range []struct {
		name      string
		got, want any
	}{
		{"Quote", Quote, (func(any, ...any) *QuoteBlock)(nil)},
		{"UL", UL, (func(any, ...any) *BulletListBlock)(nil)},
		{"OL", OL, (func(any, ...any) *OrderedListBlock)(nil)},
		{"LI", LI, (func(any, ...any) *ListItemBlock)(nil)},
		{"Tasks", Tasks, (func(Block, ...Block) *TaskListBlock)(nil)},
		{"Table", Table, (func(*TableRowBlock, ...*TableRowBlock) *TableBlock)(nil)},
		{"Layout", Layout, (func(*LayoutColumnBlock, *LayoutColumnBlock, ...*LayoutColumnBlock) *LayoutBlock)(nil)},
	} {
		if g, w := reflect.TypeOf(tc.got), reflect.TypeOf(tc.want); g != w {
			t.Errorf("%s is %v, want %v", tc.name, g, w)
		}
	}
}

func TestBuild_C_BUILD_03_Shorthand(t *testing.T) {
	// A string child is a paragraph; runs are a paragraph; lists flatten.
	got := Secret("a", Bold("b"), List{Rule()}, []any{"c"}, nil)
	encodes(t, got, `{"kind":"secret","children":[{"kind":"paragraph","content":[{"text":"a"}]},{"kind":"paragraph","content":[{"text":"b","bold":true}]},{"kind":"rule"},{"kind":"paragraph","content":[{"text":"c"}]}]}`)
	// A list item given a paragraph or a block wraps it rather than nesting
	// a second paragraph.
	encodes(t, UL(P("x"), &ListItemBlock{Children: List{P("y")}}),
		`{"kind":"bulletList","children":[{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"x"}]}]},{"kind":"listItem","children":[{"kind":"paragraph","content":[{"text":"y"}]}]}]}`)
	// Other values are their fmt.Sprint text.
	encodes(t, P(42), `{"kind":"paragraph","content":[{"text":"42"}]}`)
}

func TestBuild_C_BUILD_04_StyledWithIDAsProperty(t *testing.T) {
	style := Style{Align: opt.Some(AlignCenter), Background: opt.Some("#14B8A6"), Color: opt.Some("#A855F7")}
	encodes(t, Styled(H(1, "Candy Kingdom"), style), `{"kind":"heading","level":1,"align":"center","background":"#14B8A6","content":[{"text":"Candy Kingdom"}]}`)
	encodes(t, Styled(Quote("q"), style), `{"kind":"quote","color":"#A855F7","children":[{"kind":"paragraph","content":[{"text":"q"}]}]}`)
	encodes(t, Styled(Column(50, "c"), style), `{"kind":"layoutColumn","width":50,"background":"#14B8A6","children":[{"kind":"paragraph","content":[{"text":"c"}]}]}`)
	// A style field the block lacks is ignored, and unset fields are kept.
	p := P("x")
	p.Align = opt.Some(AlignRight)
	encodes(t, Styled(p, Style{Background: opt.Some("#000000")}), `{"kind":"paragraph","align":"right","background":"#000000","content":[{"text":"x"}]}`)

	encodes(t, WithID("intro", P("Hi")), `{"kind":"paragraph","id":"intro","content":[{"text":"Hi"}]}`)
	raw := &RawBlock{KindName: "x", JSON: json.RawMessage(`{"kind":"x"}`)}
	if WithID("y", raw).NodeID() != "" {
		t.Error("WithID changed a raw block")
	}

	// The block's title moves to the envelope unless the envelope sets one;
	// layout and width are dropped.
	img := Image("https://assets.example/p.png", "")
	img.Fill = opt.Some(true)
	img.Title = opt.Some("Block title")
	img.Layout = opt.Some(LayoutWrapRight)
	encodes(t, AsProperty(img, Envelope{Title: opt.Some("Portrait"), IsTitleHidden: opt.Some(true)}),
		`{"kind":"image","title":"Portrait","isTitleHidden":true,"url":"https://assets.example/p.png","fill":true}`)
	encodes(t, AsProperty(img, Envelope{IsHidden: opt.Some(true)}),
		`{"kind":"image","title":"Block title","isHidden":true,"url":"https://assets.example/p.png","fill":true}`)
	for _, b := range []PropertyCapable{TextField("t"), KeyValue(), Image("u", ""), Meter(MeterBar), Collection(), Links(), Embed(EmbedSpotify, "u"), Map("r", "d")} {
		if p := AsProperty(b, Envelope{}); p.Kind() != b.Kind() || reflect.ValueOf(p).IsNil() {
			t.Errorf("AsProperty(%s) = %#v", b.Kind(), p)
		}
	}
}

func TestBuild_C_BUILD_05_MapBlocks(t *testing.T) {
	encodes(t, Map("atlas", "atlasMap"), `{"kind":"map","resourceId":"atlas","documentId":"atlasMap"}`)
	encodes(t, EmptyMap(), `{"kind":"map"}`)
}

func TestBuild_C_BUILD_06_NoClientValidation(t *testing.T) {
	// The server rejects icon/color with a non-custom tone; the builder
	// sends what it is given.
	c := Callout(ToneInfo, "x")
	c.Icon, c.Color = opt.Some("dragon"), opt.Some("red")
	encodes(t, c, `{"kind":"callout","tone":"info","icon":"dragon","color":"red","children":[{"kind":"paragraph","content":[{"text":"x"}]}]}`)
	encodes(t, H(9, "deep"), `{"kind":"heading","level":9,"content":[{"text":"deep"}]}`)
}
