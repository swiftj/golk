package blocks

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/swiftj/golk/lk/internal/opt"
)

type golden struct {
	Text   string          `json:"text"`
	Blocks json.RawMessage `json:"blocks"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	data, err := os.ReadFile("testdata/plaintext_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// sameJSON reports whether a and b are the same JSON value.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("%v: %s", err, a)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return reflect.DeepEqual(va, vb)
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func specSchemas(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("../../docs/legendkeeper/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	return spec.Components.Schemas
}

func TestBlock_C_BLOCK_01_EveryKindDecodesAndRoundTrips(t *testing.T) {
	var union struct {
		Discriminator struct {
			Mapping map[string]string `json:"mapping"`
		} `json:"discriminator"`
	}
	if err := json.Unmarshal(specSchemas(t)["Block"], &union); err != nil {
		t.Fatal(err)
	}
	var specKinds []string
	for k := range union.Discriminator.Mapping {
		specKinds = append(specKinds, k)
		if _, ok := blockKinds[Kind(k)]; !ok {
			t.Errorf("spec kind %q has no Go type", k)
		}
	}
	if len(specKinds) != 28 || len(blockKinds) != len(specKinds) {
		t.Errorf("spec has %d kinds, the package %d", len(specKinds), len(blockKinds))
	}

	// The server's own page round-trips to the same JSON value.
	g := loadGolden(t)
	var l List
	if err := json.Unmarshal(g.Blocks, &l); err != nil {
		t.Fatal(err)
	}
	for _, b := range l {
		if _, raw := b.(*RawBlock); raw {
			t.Errorf("known kind %s decoded as Raw", b.Kind())
		}
	}
	if out := mustEncode(t, l); !sameJSON(t, out, g.Blocks) {
		t.Errorf("round trip changed the page:\n got %s\nwant %s", out, g.Blocks)
	}
}

func TestBlock_C_BLOCK_02_UnknownKindKept(t *testing.T) {
	in := `[{"kind":"hologram","id":"h1","beam":{"color":"#00ff00"}},{"kind":"rule"}]`
	var l List
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	raw, ok := l[0].(*RawBlock)
	if !ok || raw.Kind() != "hologram" || raw.NodeID() != "h1" {
		t.Fatalf("got %#v", l[0])
	}
	if out := mustEncode(t, l); string(out) != `[{"kind":"hologram","id":"h1","beam":{"color":"#00ff00"}},{"kind":"rule"}]` {
		t.Errorf("re-encoded %s", out)
	}
	// Unknown kinds inside a container and unknown property kinds too.
	in = `[{"kind":"quote","children":[{"kind":"hologram"}]},{"kind":"infobox","rows":[{"kind":"chart","id":"c1","bars":[1,2]}]}]`
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	if _, ok := l[1].(*InfoboxBlock).Rows[0].(*RawProperty); !ok {
		t.Errorf("unknown property kind: %#v", l[1].(*InfoboxBlock).Rows[0])
	}
	if out := mustEncode(t, l); !sameJSON(t, out, []byte(in)) {
		t.Errorf("re-encoded %s", out)
	}
	// A known kind in a shape the package does not expect is kept raw rather
	// than failing the page.
	in = `[{"kind":"heading","level":"one","content":[]},{"kind":"keyValue","rows":{"k":"v"}},{"kind":"rule"}]`
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, ok := l[i].(*RawBlock); !ok {
			t.Errorf("block %d = %#v, want RawBlock", i, l[i])
		}
	}
	if out := mustEncode(t, l); string(out) != in {
		t.Errorf("re-encoded %s", out)
	}
	if p, err := UnmarshalProperty([]byte(`{"kind":"image","url":7}`)); err != nil || p.Kind() != KindImage || p.PropertyEnvelope() != nil {
		t.Errorf("bad image property = %#v, %v", p, err)
	}
	// Only JSON that is not a block object is an error.
	for _, bad := range []string{`[1]`, `[{"id":"x"}]`, `[{"kind":3}]`, `{}`} {
		if err := json.Unmarshal([]byte(bad), &l); err == nil {
			t.Errorf("%s decoded", bad)
		}
	}
}

func TestBlock_C_CONTENT_04_UnsupportedEchoedByID(t *testing.T) {
	in := `[{"kind":"unsupported","id":"u1"},{"kind":"infobox","rows":[{"kind":"unsupported","id":"u2","rank":"a","title":"Old"}]}]`
	var l List
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	if u, ok := l[0].(*UnsupportedBlock); !ok || u.ID != "u1" {
		t.Fatalf("got %#v", l[0])
	}
	if u, ok := l[1].(*InfoboxBlock).Rows[0].(*UnsupportedProperty); !ok || u.ID != "u2" {
		t.Fatalf("got %#v", l[1].(*InfoboxBlock).Rows[0])
	}
	if out := mustEncode(t, l); string(out) != `[{"kind":"unsupported","id":"u1"},{"kind":"infobox","rows":[{"kind":"unsupported","id":"u2","rank":"a","title":"Old"}]}]` {
		t.Errorf("re-encoded %s", out)
	}
}

func TestBlock_C_BLOCK_03_OnlySetFieldsSent(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    Block
		want string
	}{
		{"empty paragraph", &ParagraphBlock{}, `{"kind":"paragraph","content":[]}`},
		{"styled paragraph", &ParagraphBlock{Align: opt.Some(AlignCenter), Content: Runs{&Text{Text: "x"}}},
			`{"kind":"paragraph","align":"center","content":[{"text":"x"}]}`},
		{"required bool false is sent", &TaskItemBlock{Content: Runs{}}, `{"kind":"taskItem","checked":false,"content":[]}`},
		{"optional false is sent when set", &ImageBlock{URL: "u", IsTitleHidden: opt.Some(false)},
			`{"kind":"image","url":"u","isTitleHidden":false}`},
		{"empty items omitted when nil", &ResourceCollectionBlock{}, `{"kind":"resourceCollection"}`},
		{"empty items sent when set", &ResourceCollectionBlock{Items: ResourceItems{}}, `{"kind":"resourceCollection","items":[]}`},
		{"required items sent when nil", &ResourceLinkBlock{}, `{"kind":"resourceLink","items":[]}`},
		{"marks only when true", &ParagraphBlock{Content: Runs{&Text{Text: "a", Bold: true}, &Break{}, &Mention{ResourceID: "r1", Color: opt.Some("#112233")}}},
			`{"kind":"paragraph","content":[{"text":"a","bold":true},{"break":true},{"mention":{"resourceId":"r1"},"color":"#112233"}]}`},
		{"filter type filled", &ResourceCollectionBlock{Filter: &Filter{ID: "f", FilterType: "tag", Operator: opt.Some("has_any"), Tags: []string{"Kingdom"}}},
			`{"kind":"resourceCollection","filter":{"type":"filter","id":"f","filterType":"tag","operator":"has_any","tags":["Kingdom"]}}`},
		{"filter null value kept", &ResourceCollectionBlock{Filter: &Filter{ID: "f", FilterType: "field", Field: opt.Some("parentId"), Operator: opt.Some("equals"), Value: json.RawMessage("null")}},
			`{"kind":"resourceCollection","filter":{"type":"filter","id":"f","filterType":"field","field":"parentId","operator":"equals","value":null}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(mustEncode(t, tc.b)); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestBlock_C_BLOCK_04_UnmodelledFieldsKept(t *testing.T) {
	in := `{"kind":"paragraph","id":"p1","content":[{"text":"a","font":"serif"},{"mention":{"resourceId":"r","avatar":"x"},"glow":true}],"indent":2}`
	b, err := Unmarshal([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	p := b.(*ParagraphBlock)
	p.Content = append(p.Content, &Text{Text: "b"}) // a caller edit
	want := `{"kind":"paragraph","id":"p1","content":[{"text":"a","font":"serif"},{"mention":{"resourceId":"r","avatar":"x"},"glow":true},{"text":"b"}],"indent":2}`
	if got := string(mustEncode(t, p)); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	// Envelope fields of a property and fields inside rows are kept as well.
	in = `{"kind":"keyValue","id":"k","rank":"a","pinned":true,"rows":[{"key":"k","text":"v","emoji":"x"}]}`
	pr, err := UnmarshalProperty([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := mustEncode(t, pr); !sameJSON(t, got, []byte(in)) {
		t.Errorf("property re-encoded %s", got)
	}
}

func TestBlock_C_BLOCK_05_UnknownEnumValuesKept(t *testing.T) {
	in := `{"kind":"callout","tone":"sparkle","children":[{"kind":"paragraph","align":"justify","content":[]}]}`
	b, err := Unmarshal([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if c := b.(*CalloutBlock); c.Tone != "sparkle" {
		t.Errorf("tone = %q", c.Tone)
	}
	if got := mustEncode(t, b); !bytes.Equal(got, []byte(in)) {
		t.Errorf("re-encoded %s", got)
	}
}

// specFields lists a schema's property names, following allOf, and for a
// oneOf without a discriminator the union of its variants' properties.
func specFields(t *testing.T, schemas map[string]json.RawMessage, raw json.RawMessage) []string {
	t.Helper()
	var s struct {
		Ref        string                     `json:"$ref"`
		Properties map[string]json.RawMessage `json:"properties"`
		OneOf      []json.RawMessage          `json:"oneOf"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s.Ref != "" {
		return specFields(t, schemas, schemas[s.Ref[len("#/components/schemas/"):]])
	}
	var out []string
	for k := range s.Properties {
		out = append(out, k)
	}
	for _, v := range s.OneOf {
		for _, f := range specFields(t, schemas, v) {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	slices.Sort(out)
	return out
}

// goFields lists the JSON names v's type declares, plus "kind" for blocks
// and properties.
func goFields(v any, kind bool) []string {
	var out []string
	for k := range fieldsOf(reflect.TypeOf(v).Elem()) {
		out = append(out, k)
	}
	if kind {
		out = append(out, "kind")
	}
	slices.Sort(out)
	return out
}

func TestBlock_C_BLOCK_03_FieldsMatchSpec(t *testing.T) {
	schemas := specSchemas(t)
	check := func(schema string, v any, kind bool) {
		t.Helper()
		raw, ok := schemas[schema]
		if !ok {
			t.Errorf("no spec schema %s", schema)
			return
		}
		if want, got := specFields(t, schemas, raw), goFields(v, kind); !slices.Equal(want, got) {
			t.Errorf("%s: spec fields %v, Go fields %v", schema, want, got)
		}
	}
	for k, newBlock := range blockKinds {
		name := string(k)
		check(string(name[0]-32)+name[1:]+"Block", newBlock(), true)
	}
	for k, newProp := range propertyKinds {
		name := string(k)
		check(string(name[0]-32)+name[1:]+"Property", newProp(), true)
	}
	check("FilterPredicate", &Filter{}, false)
	check("MapViewport", &Viewport{}, false)

	// Inline object schemas: the keyValue rows, meter items and collection items.
	var kv, meter, coll struct {
		Properties map[string]struct {
			Items json.RawMessage `json:"items"`
		} `json:"properties"`
	}
	for _, s := range []struct {
		name string
		into any
	}{{"KeyValueBlock", &kv}, {"MeterBlock", &meter}, {"ResourceCollectionBlock", &coll}} {
		if err := json.Unmarshal(schemas[s.name], s.into); err != nil {
			t.Fatal(err)
		}
	}
	inline := func(name string, raw json.RawMessage, v any) {
		t.Helper()
		if want, got := specFields(t, schemas, raw), goFields(v, false); !slices.Equal(want, got) {
			t.Errorf("%s: spec fields %v, Go fields %v", name, want, got)
		}
	}
	inline("KeyValueBlock.rows[]", kv.Properties["rows"].Items, &KVRow{})
	inline("MeterBlock.items[]", meter.Properties["items"].Items, &MeterItem{})
	inline("ResourceCollectionBlock.items[]", coll.Properties["items"].Items, &ResourceItem{})

	// The three run shapes.
	var run struct {
		AnyOf []json.RawMessage `json:"anyOf"`
	}
	if err := json.Unmarshal(schemas["Run"], &run); err != nil || len(run.AnyOf) != 3 {
		t.Fatalf("Run: %v, %d variants", err, len(run.AnyOf))
	}
	var mention struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(run.AnyOf[1], &mention); err != nil {
		t.Fatal(err)
	}
	inline("Run (text)", run.AnyOf[0], &Text{})
	inline("Run (mention)", run.AnyOf[1], &mentionWire{})
	inline("Run (mention.mention)", mention.Properties["mention"], &mentionRef{})
	if want := specFields(t, schemas, run.AnyOf[2]); !slices.Equal(want, []string{"break"}) {
		t.Errorf("Run (break): spec fields %v", want)
	}
}
