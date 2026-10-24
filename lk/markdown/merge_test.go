package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
)

// fakeHooks stands in for the codec: a node's key is its JSON with every id
// and rank removed (an unsupported node shows only its kind and envelope),
// and unsupported and raw nodes are opaque.
var fakeHooks = hooks{key: fakeKey, opaque: fakeOpaque}

func fakeKey(n blocks.Node) string {
	data, err := json.Marshal(n)
	if err != nil {
		return "\x00invalid"
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return "\x00invalid"
	}
	out, err := json.Marshal(stripHidden(v, true))
	if err != nil {
		return "\x00invalid"
	}
	return string(out)
}

func fakeOpaque(n blocks.Node) bool {
	switch n.(type) {
	case *blocks.UnsupportedBlock, *blocks.RawBlock, *blocks.UnsupportedProperty, *blocks.RawProperty:
		return true
	}
	return false
}

// stripHidden removes "id" and "rank" at every level. With unsupported,
// an unsupported object keeps only its kind and envelope.
func stripHidden(v any, unsupported bool) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, e := range x {
			if k == "id" || k == "rank" {
				continue
			}
			if unsupported && x["kind"] == "unsupported" && k != "kind" && k != "title" && k != "isTitleHidden" && k != "isHidden" {
				continue
			}
			m[k] = stripHidden(e, unsupported)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripHidden(e, unsupported)
		}
		return out
	}
	return v
}

func encode(t testing.TB, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decodeList(t testing.TB, s string) blocks.List {
	t.Helper()
	var l blocks.List
	if err := json.Unmarshal([]byte(s), &l); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return l
}

// withoutIDs is l as Markdown without {#id} parses it: no ids or ranks.
func withoutIDs(t testing.TB, l blocks.List) blocks.List {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(encode(t, l)), &v); err != nil {
		t.Fatal(err)
	}
	return decodeList(t, encode(t, stripHidden(v, false)))
}

func p(id, text string) *blocks.ParagraphBlock { return blocks.WithID(id, blocks.P(text)) }

func row(id, rank, key, value string) blocks.KVRow {
	r := blocks.KV(key, value)
	r.ID, r.Rank = id, rank
	return r
}

type reconcileCase struct {
	name         string
	base, parsed blocks.List
	allow        bool
	want         blocks.List // expected result, compared as JSON
	report       *Report
	err          func(error) bool // the error expected, if any
}

func isIDError(err error) bool {
	var e *IDError
	return errors.As(err, &e)
}

func isRemoveError(ids ...string) func(error) bool {
	return func(err error) bool {
		var e *RemoveError
		if !errors.As(err, &e) || !errors.Is(err, ErrRemovesOpaque) {
			return false
		}
		var got []string
		for _, r := range e.Removed {
			if !r.Opaque {
				return false
			}
			got = append(got, r.ID)
		}
		return reflect.DeepEqual(got, ids)
	}
}

func runReconcile(t *testing.T, c reconcileCase) {
	t.Helper()
	before := encode(t, c.base)
	got, rep, err := reconcile(c.parsed, c.base, fakeHooks, c.allow)
	if after := encode(t, c.base); after != before {
		t.Fatalf("base was modified\nbefore %s\nafter  %s", before, after)
	}
	if c.err != nil {
		if err == nil || !c.err(err) {
			t.Fatalf("err = %v, want the expected error", err)
		}
		if got != nil || rep != nil {
			t.Fatalf("returned blocks or report with an error")
		}
		return
	}
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if c.want != nil {
		if g, w := encode(t, got), encode(t, c.want); g != w {
			t.Errorf("result\ngot  %s\nwant %s", g, w)
		}
	}
	if c.report != nil && !reflect.DeepEqual(rep, c.report) {
		t.Errorf("report\ngot  %+v\nwant %+v", *rep, *c.report)
	}
}

func TestReconcile_M_ID_01(t *testing.T) {
	page := func() blocks.List {
		return decodeList(t, `[
			{"kind":"heading","id":"h1","level":1,"content":[{"text":"Ooo"}]},
			{"kind":"paragraph","id":"p1","align":"center","content":[{"text":"Candy "},{"text":"Kingdom","bold":true}],"futureField":{"x":1}},
			{"kind":"callout","id":"c1","tone":"info","children":[{"kind":"paragraph","id":"p2","content":[{"text":"Beware"}]}]},
			{"kind":"keyValue","id":"kv1","rows":[{"id":"r1","rank":"a0","key":"Species","content":[{"text":"Dog"}]}]},
			{"kind":"infobox","id":"ib1","rows":[{"kind":"keyValue","id":"pr1","rank":"a0","title":"Stats","rows":[{"id":"r2","rank":"a0","key":"HP","text":"3"}]},{"kind":"unsupported","id":"u2","rank":"a1","blob":true}]},
			{"kind":"unsupported","id":"u1","data":[1,2,3]},
			{"kind":"wizardry","id":"w1","spell":"fireball"}
		]`)
	}
	base := page()
	parsed := withoutIDs(t, page())
	// Markdown keeps the ids of opaque content.
	parsed[4].(*blocks.InfoboxBlock).Rows[1] = &blocks.UnsupportedProperty{Envelope: blocks.Envelope{ID: "u2"}}
	parsed[5] = &blocks.UnsupportedBlock{ID: "u1"}
	parsed[6] = &blocks.RawBlock{KindName: "wizardry", JSON: json.RawMessage(`{"kind":"wizardry","id":"w1","spell":"fireball"}`)}

	t.Run("M-ID-01 unchanged page is byte-identical", func(t *testing.T) {
		got, rep, err := reconcile(parsed, base, fakeHooks, false)
		if err != nil {
			t.Fatal(err)
		}
		if g, w := encode(t, got), encode(t, base); g != w {
			t.Fatalf("got  %s\nwant %s", g, w)
		}
		want := &Report{Kept: []string{"h1", "p1", "c1", "kv1", "ib1", "u1", "w1"}}
		if !reflect.DeepEqual(rep, want) {
			t.Fatalf("report %+v", *rep)
		}
		// Deep copy: changing the result leaves base alone.
		got[1].(*blocks.ParagraphBlock).Content = blocks.R("changed")
		got[2].(*blocks.CalloutBlock).Children[0].(*blocks.ParagraphBlock).Content = blocks.R("changed")
		got[3].(*blocks.KeyValueBlock).Rows[0].Key = "changed"
		if g, w := encode(t, base), encode(t, page()); g != w {
			t.Fatalf("result aliases base\nbase %s", g)
		}
	})
	t.Run("M-ID-01 explicit ids on every node", func(t *testing.T) {
		got, rep, err := reconcile(page(), base, fakeHooks, false)
		if err != nil {
			t.Fatal(err)
		}
		if g, w := encode(t, got), encode(t, base); g != w {
			t.Fatalf("got  %s\nwant %s", g, w)
		}
		if len(rep.Kept) != 7 || rep.Changed != nil || rep.Added != nil || rep.Removed != nil {
			t.Fatalf("report %+v", *rep)
		}
	})
}

func TestReconcile_M_ID_02(t *testing.T) {
	base := func() blocks.List {
		return blocks.List{
			p("p1", "a"),
			blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "b"), p("p3", "c"))),
			p("p4", "d"),
		}
	}
	runReconcile(t, reconcileCase{
		name:   "M-ID-02 edit one nested paragraph",
		base:   base(),
		parsed: blocks.List{blocks.P("a"), blocks.Callout(blocks.ToneInfo, blocks.P("B"), blocks.P("c")), blocks.P("d")},
		want:   blocks.List{p("p1", "a"), blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "B"), p("p3", "c"))), p("p4", "d")},
		report: &Report{Kept: []string{"p1", "p3", "p4"}, Changed: []string{"c1", "p2"}},
	})
}

func TestReconcile(t *testing.T) {
	abc := func() blocks.List { return blocks.List{p("p1", "a"), p("p2", "b"), p("p3", "c")} }
	unsupported := func(id string) *blocks.UnsupportedBlock {
		return decodeList(t, `[{"kind":"unsupported","id":"`+id+`","hidden":"stuff"}]`)[0].(*blocks.UnsupportedBlock)
	}
	infobox := func() *blocks.InfoboxBlock {
		return decodeList(t, `[{"kind":"infobox","id":"ib1","rows":[
			{"kind":"keyValue","id":"pr1","rank":"a0","title":"Stats","rows":[{"id":"r1","rank":"a0","key":"HP","text":"3"}]},
			{"kind":"unsupported","id":"u2","rank":"a1","blob":true}]}]`)[0].(*blocks.InfoboxBlock)
	}
	kvRow := func(title string, rows ...blocks.KVRow) blocks.Property {
		return blocks.AsProperty(blocks.KeyValue(rows...), blocks.Envelope{Title: opt.Some(title)})
	}
	cases := []reconcileCase{
		{
			name:   "M-ID-02 insert in the middle",
			base:   abc(),
			parsed: blocks.List{blocks.P("a"), blocks.P("x"), blocks.P("b"), blocks.P("c")},
			want:   blocks.List{p("p1", "a"), blocks.P("x"), p("p2", "b"), p("p3", "c")},
			report: &Report{Kept: []string{"p1", "p2", "p3"}, Added: []string{"blocks[1]"}},
		},
		{
			name:   "M-ID-02 delete in the middle",
			base:   abc(),
			parsed: blocks.List{blocks.P("a"), blocks.P("c")},
			want:   blocks.List{p("p1", "a"), p("p3", "c")},
			report: &Report{Kept: []string{"p1", "p3"}, Removed: []RemovedBlock{{ID: "p2", Kind: blocks.KindParagraph}}},
		},
		{
			name:   "M-ID-02 reorder keeps every id",
			base:   abc(),
			parsed: blocks.List{blocks.P("c"), blocks.P("a"), blocks.P("b")},
			want:   blocks.List{p("p3", "c"), p("p1", "a"), p("p2", "b")},
			report: &Report{Kept: []string{"p3", "p1", "p2"}},
		},
		{
			name:   "M-ID-02 edit and insert in one hunk pairs by kind",
			base:   abc(),
			parsed: blocks.List{blocks.H(2, "new"), blocks.P("A"), blocks.P("B"), blocks.P("c")},
			want:   blocks.List{blocks.H(2, "new"), p("p1", "A"), p("p2", "B"), p("p3", "c")},
			report: &Report{Kept: []string{"p3"}, Changed: []string{"p1", "p2"}, Added: []string{"blocks[0]"}},
		},
		{
			name:   "M-ID-03 kind change at the same position moves no id",
			base:   blocks.List{p("p1", "a")},
			parsed: blocks.List{blocks.H(1, "a")},
			want:   blocks.List{blocks.H(1, "a")},
			report: &Report{Added: []string{"blocks[0]"}, Removed: []RemovedBlock{{ID: "p1", Kind: blocks.KindParagraph}}},
		},
		{
			name:   "M-ID-03 duplicate equal paragraphs use a base block once",
			base:   blocks.List{p("p1", "x")},
			parsed: blocks.List{blocks.P("x"), blocks.P("x")},
			want:   blocks.List{p("p1", "x"), blocks.P("x")},
			report: &Report{Kept: []string{"p1"}, Added: []string{"blocks[1]"}},
		},
		{
			name: "M-ID-03 explicit id moves a block out of a container",
			base: blocks.List{p("p1", "a"), blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "b")))},
			parsed: blocks.List{
				p("p2", "b"), blocks.P("a"), blocks.Callout(blocks.ToneInfo, blocks.P("new")),
			},
			want: blocks.List{
				p("p2", "b"), p("p1", "a"), blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, blocks.P("new"))),
			},
			report: &Report{Kept: []string{"p2", "p1"}, Changed: []string{"c1"}, Added: []string{"blocks[2].children[0]"}},
		},
		{
			name:   "M-ID-03 explicit id wins over matching",
			base:   blocks.List{blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "b"))), p("p3", "b")},
			parsed: blocks.List{blocks.Callout(blocks.ToneInfo, p("p3", "b"))},
			want:   blocks.List{blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p3", "b")))},
			report: &Report{Kept: []string{"p3"}, Changed: []string{"c1"}, Removed: []RemovedBlock{{ID: "p2", Kind: blocks.KindParagraph}}},
		},
		{
			name:   "M-ID-03 explicit id with new content is changed in place",
			base:   abc(),
			parsed: blocks.List{p("p3", "C"), blocks.P("a"), blocks.P("b")},
			want:   blocks.List{p("p3", "C"), p("p1", "a"), p("p2", "b")},
			report: &Report{Kept: []string{"p1", "p2"}, Changed: []string{"p3"}},
		},
		{
			name:   "M-ID-03 explicit id not in base is kept as given",
			base:   abc(),
			parsed: blocks.List{blocks.P("a"), blocks.P("b"), blocks.P("c"), p("zz", "d")},
			want:   blocks.List{p("p1", "a"), p("p2", "b"), p("p3", "c"), p("zz", "d")},
			report: &Report{Kept: []string{"p1", "p2", "p3"}, Added: []string{"blocks[3]"}},
		},
		{
			name:   "M-ID-03 explicit id of another kind is an error",
			base:   abc(),
			parsed: blocks.List{blocks.WithID("p1", blocks.H(1, "a"))},
			err:    isIDError,
		},
		{
			name:   "M-ID-03 a block id on an infobox row is an error",
			base:   blocks.List{blocks.WithID("kv1", blocks.KeyValue())},
			parsed: blocks.List{blocks.Infobox(blocks.AsProperty(blocks.KeyValue(), blocks.Envelope{ID: "kv1"}))},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 the same id twice is an error",
			base:   abc(),
			parsed: blocks.List{p("p1", "a"), blocks.Callout(blocks.ToneInfo, p("p1", "a"))},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 a new id twice is an error",
			parsed: blocks.List{p("n1", "a"), p("n1", "b")},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 unsupported id not in base is an error",
			base:   blocks.List{unsupported("u1")},
			parsed: blocks.List{&blocks.UnsupportedBlock{ID: "u9"}},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 unsupported without base is an error",
			parsed: blocks.List{blocks.P("a"), &blocks.UnsupportedBlock{ID: "u1"}},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 unsupported row not in base is an error",
			base:   blocks.List{infobox()},
			parsed: blocks.List{blocks.Infobox(&blocks.UnsupportedProperty{Envelope: blocks.Envelope{ID: "u9"}})},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-06 unsupported without an id is an error",
			base:   blocks.List{unsupported("u1")},
			parsed: blocks.List{&blocks.UnsupportedBlock{}},
			err:    isIDError,
		},
		{
			name:   "M-SAFE-04 unsupported echoed by id keeps its content",
			base:   blocks.List{p("p1", "a"), unsupported("u1")},
			parsed: blocks.List{blocks.P("A"), &blocks.UnsupportedBlock{ID: "u1"}},
			want:   blocks.List{p("p1", "A"), unsupported("u1")},
			report: &Report{Kept: []string{"u1"}, Changed: []string{"p1"}},
		},
		{
			name:   "M-SAFE-04 removing a top-level unsupported is an error",
			base:   blocks.List{p("p1", "a"), unsupported("u1")},
			parsed: blocks.List{blocks.P("a")},
			err:    isRemoveError("u1"),
		},
		{
			name:   "M-SAFE-04 AllowRemoveOpaque permits it",
			base:   blocks.List{p("p1", "a"), unsupported("u1")},
			parsed: blocks.List{blocks.P("a")},
			allow:  true,
			want:   blocks.List{p("p1", "a")},
			report: &Report{Kept: []string{"p1"}, Removed: []RemovedBlock{{ID: "u1", Kind: blocks.KindUnsupported, Opaque: true}}},
		},
		{
			name:   "M-SAFE-04 removing an unsupported inside an edited callout is an error",
			base:   blocks.List{blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "b"), unsupported("u1")))},
			parsed: blocks.List{blocks.Callout(blocks.ToneInfo, blocks.P("B"))},
			err:    isRemoveError("u1"),
		},
		{
			name:   "M-SAFE-04 nested removal allowed is reported",
			base:   blocks.List{blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "b"), unsupported("u1")))},
			parsed: blocks.List{blocks.Callout(blocks.ToneInfo, blocks.P("B"))},
			allow:  true,
			want:   blocks.List{blocks.WithID("c1", blocks.Callout(blocks.ToneInfo, p("p2", "B")))},
			report: &Report{Changed: []string{"c1", "p2"}, Removed: []RemovedBlock{{ID: "u1", Kind: blocks.KindUnsupported, Opaque: true}}},
		},
		{
			name:   "M-SAFE-04 removing a container that holds an unsupported is an error",
			base:   blocks.List{p("p1", "a"), blocks.WithID("q1", blocks.Quote(p("p2", "b"), unsupported("u1")))},
			parsed: blocks.List{blocks.P("a")},
			err:    isRemoveError("u1"),
		},
		{
			name:   "M-SAFE-04 removing an unsupported infobox row is an error",
			base:   blocks.List{infobox()},
			parsed: blocks.List{blocks.Infobox(kvRow("Stats", blocks.KV("HP", "4")))},
			err:    isRemoveError("u2"),
		},
		{
			name:   "M-SAFE-04 removing a whole infobox with an unsupported row is an error",
			base:   blocks.List{p("p1", "a"), infobox()},
			parsed: blocks.List{blocks.P("a")},
			err:    isRemoveError("u2"),
		},
		{
			name:   "M-SAFE-04 whole infobox removal allowed is reported as opaque",
			base:   blocks.List{p("p1", "a"), infobox()},
			parsed: blocks.List{blocks.P("a")},
			allow:  true,
			report: &Report{Kept: []string{"p1"}, Removed: []RemovedBlock{{ID: "ib1", Kind: blocks.KindInfobox, Opaque: true}}},
		},
		{
			name:   "M-SAFE-04 removing a raw block is an error",
			base:   decodeList(t, `[{"kind":"paragraph","id":"p1","content":[]},{"kind":"wizardry","id":"w1"}]`),
			parsed: blocks.List{&blocks.ParagraphBlock{}},
			err:    isRemoveError("w1"),
		},
		{
			name:   "M-SAFE-04 removed secret is reported",
			base:   blocks.List{p("p1", "a"), blocks.WithID("s1", blocks.Secret(p("p2", "The Lich lives")))},
			parsed: blocks.List{blocks.P("a")},
			want:   blocks.List{p("p1", "a")},
			report: &Report{Kept: []string{"p1"}, Removed: []RemovedBlock{{ID: "s1", Kind: blocks.KindSecret, Secret: true}}},
		},
		{
			name:   "M-SAFE-04 removed container holding a secret is reported",
			base:   blocks.List{blocks.WithID("x1", blocks.Expand("More", blocks.WithID("s1", blocks.Secret(p("p2", "hidden")))))},
			parsed: blocks.List{},
			report: &Report{Removed: []RemovedBlock{{ID: "x1", Kind: blocks.KindExpand, Secret: true}}},
		},
		{
			name: "M-ID-02 key-value rows keep ids and ranks by key",
			base: blocks.List{blocks.WithID("kv1", blocks.KeyValue(
				row("r1", "a0", "Species", "Dog"), row("r2", "a1", "Plays", "Games")))},
			parsed: blocks.List{blocks.KeyValue(blocks.KV("Plays", "Video games"), blocks.KV("Species", "Dog"), blocks.KV("New", "x"))},
			want: blocks.List{blocks.WithID("kv1", blocks.KeyValue(
				row("r2", "a1", "Plays", "Video games"), row("r1", "a0", "Species", "Dog"), blocks.KV("New", "x")))},
			report: &Report{Changed: []string{"kv1"}},
		},
		{
			name: "M-ID-02 resource and meter items keep their ids",
			base: decodeList(t, `[
				{"kind":"resourceLink","id":"l1","items":[{"id":"i1","rank":"a0","resourceId":"finn"},{"id":"i2","rank":"a1","resourceId":"jake","documentId":"d9"}]},
				{"kind":"meter","id":"m1","mode":"bar","items":[{"id":"hp","name":"HP","icon":"heart","current":3,"max":5}]}]`),
			parsed: decodeList(t, `[
				{"kind":"resourceLink","items":[{"resourceId":"jake","documentId":"d9"},{"resourceId":"finn"},{"resourceId":"jake"}]},
				{"kind":"meter","mode":"bar","items":[{"name":"MP","icon":"star","current":1,"max":1},{"name":"HP","icon":"heart","current":2,"max":5}]}]`),
			want: decodeList(t, `[
				{"kind":"resourceLink","id":"l1","items":[{"id":"i2","rank":"a1","resourceId":"jake","documentId":"d9"},{"id":"i1","rank":"a0","resourceId":"finn"},{"resourceId":"jake"}]},
				{"kind":"meter","id":"m1","mode":"bar","items":[{"name":"MP","icon":"star","current":1,"max":1},{"id":"hp","name":"HP","icon":"heart","current":2,"max":5}]}]`),
			report: &Report{Changed: []string{"l1", "m1"}},
		},
		{
			name:   "M-ID-02 edited infobox row keeps id, rank and row ids",
			base:   blocks.List{infobox()},
			parsed: blocks.List{blocks.Infobox(kvRow("Stats", blocks.KV("HP", "4")), &blocks.UnsupportedProperty{Envelope: blocks.Envelope{ID: "u2"}})},
			want: decodeList(t, `[{"kind":"infobox","id":"ib1","rows":[
				{"kind":"keyValue","id":"pr1","rank":"a0","title":"Stats","rows":[{"id":"r1","rank":"a0","key":"HP","content":[{"text":"4"}]}]},
				{"kind":"unsupported","id":"u2","rank":"a1","blob":true}]}]`),
			report: &Report{Changed: []string{"ib1", "pr1"}, Kept: []string{"u2"}},
		},
		{
			name:   "M-ID-02 retitled unsupported row keeps its content",
			base:   blocks.List{infobox()},
			parsed: blocks.List{blocks.Infobox(kvRow("Stats", blocks.KVRow{Key: "HP", Text: opt.Some("3")}), &blocks.UnsupportedProperty{Envelope: blocks.Envelope{ID: "u2", Title: opt.Some("Secret map")}})},
			want: decodeList(t, `[{"kind":"infobox","id":"ib1","rows":[
				{"kind":"keyValue","id":"pr1","rank":"a0","title":"Stats","rows":[{"id":"r1","rank":"a0","key":"HP","text":"3"}]},
				{"kind":"unsupported","id":"u2","rank":"a1","title":"Secret map","blob":true}]}]`),
			report: &Report{Kept: []string{"pr1"}, Changed: []string{"ib1", "u2"}},
		},
		{
			name:   "M-ID-03 without base everything is added",
			parsed: blocks.List{blocks.P("a"), blocks.Callout(blocks.ToneInfo, blocks.P("b"))},
			want:   blocks.List{blocks.P("a"), blocks.Callout(blocks.ToneInfo, blocks.P("b"))},
			report: &Report{Added: []string{"blocks[0]", "blocks[1]"}},
		},
		{
			name:   "M-ID-03 a base block is never reused by a nested match",
			base:   blocks.List{p("p1", "x")},
			parsed: blocks.List{blocks.Quote(blocks.P("x")), blocks.P("x")},
			want:   blocks.List{blocks.Quote(blocks.P("x")), p("p1", "x")},
			report: &Report{Kept: []string{"p1"}, Added: []string{"blocks[0]"}},
		},
		{
			name:   "a nil parsed block is an error",
			parsed: blocks.List{blocks.P("a"), nil},
			err:    func(err error) bool { var e *SyntaxError; return errors.As(err, &e) },
		},
		{
			name:   "a nil parsed child is an error",
			parsed: blocks.List{&blocks.QuoteBlock{Children: blocks.List{(*blocks.ParagraphBlock)(nil)}}},
			err:    func(err error) bool { var e *SyntaxError; return errors.As(err, &e) },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runReconcile(t, c) })
	}
}

func TestReconcileNilBase(t *testing.T) {
	// blocks.List can't encode a nil element, so this case skips the
	// table's base-unchanged check.
	base := blocks.List{nil, p("p1", "a"), (*blocks.ParagraphBlock)(nil), blocks.Quote(nil)}
	got, rep, err := reconcile(blocks.List{blocks.P("a")}, base, fakeHooks, false)
	if err != nil {
		t.Fatal(err)
	}
	if g := encode(t, got); g != encode(t, blocks.List{p("p1", "a")}) {
		t.Fatalf("got %s", g)
	}
	if !reflect.DeepEqual(rep.Kept, []string{"p1"}) {
		t.Fatalf("report %+v", *rep)
	}
}

func TestReconcileGarbageRaw(t *testing.T) {
	bad := &blocks.RawBlock{KindName: "x", JSON: json.RawMessage(`{garbage`)}
	for _, allow := range []bool{false, true} {
		// Must not panic; any outcome but a panic is acceptable.
		_, _, _ = reconcile(blocks.List{blocks.P("a"), bad}, blocks.List{bad, p("p1", "a")}, fakeHooks, allow)
		_, _, _ = reconcile(blocks.List{blocks.P("a")}, blocks.List{bad}, fakeHooks, allow)
	}
	if _, _, err := reconcile(blocks.List{blocks.P("a")}, blocks.List{bad}, fakeHooks, false); !errors.Is(err, ErrRemovesOpaque) {
		t.Fatalf("err = %v, want ErrRemovesOpaque", err)
	}
	if _, _, err := reconcile(nil, nil, hooks{}, false); err == nil {
		t.Fatal("missing hooks must be an error")
	}
}

func TestReconcileLarge(t *testing.T) {
	const n = 20000
	build := func(prefix string, edit int) blocks.List {
		l := make(blocks.List, n)
		for i := range l {
			text := fmt.Sprintf("%s paragraph %d", prefix, i)
			if i == edit {
				text = "edited"
			}
			l[i] = blocks.P(text)
		}
		return l
	}
	withIDs := func(l blocks.List) blocks.List {
		for i, b := range l {
			blocks.WithID(fmt.Sprintf("p%d", i), b)
		}
		return l
	}
	t.Run("M-ID-02 one edit among 20k paragraphs", func(t *testing.T) {
		start := time.Now()
		_, rep, err := reconcile(build("x", n/2), withIDs(build("x", -1)), fakeHooks, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Kept) != n-1 || !reflect.DeepEqual(rep.Changed, []string{fmt.Sprintf("p%d", n/2)}) || rep.Added != nil || rep.Removed != nil {
			t.Fatalf("kept %d changed %v added %v removed %v", len(rep.Kept), rep.Changed, rep.Added, rep.Removed)
		}
		if d := time.Since(start); d > slowdown*5*time.Second {
			t.Fatalf("took %v", d)
		}
	})
	t.Run("M-ID-03 20k paragraphs all different stay bounded", func(t *testing.T) {
		start := time.Now()
		_, rep, err := reconcile(build("new", -1), withIDs(build("old", -1)), fakeHooks, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Changed) != n {
			t.Fatalf("changed %d", len(rep.Changed))
		}
		if d := time.Since(start); d > slowdown*10*time.Second {
			t.Fatalf("took %v", d)
		}
	})
	t.Run("M-ID-02 interleaved inserts past the diff bound keep every id", func(t *testing.T) {
		base := withIDs(build("x", -1))
		var parsed blocks.List
		for i := range n {
			parsed = append(parsed, blocks.P(fmt.Sprintf("x paragraph %d", i)), blocks.P(fmt.Sprintf("inserted %d", i)))
		}
		start := time.Now()
		_, rep, err := reconcile(parsed, base, fakeHooks, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Kept) != n || len(rep.Added) != n || rep.Changed != nil {
			t.Fatalf("kept %d added %d changed %d", len(rep.Kept), len(rep.Added), len(rep.Changed))
		}
		if d := time.Since(start); d > slowdown*10*time.Second {
			t.Fatalf("took %v", d)
		}
	})
}

// FuzzReconcile checks, for any parsed and base pages, that reconcile never
// panics; that a result never holds an id twice (when base doesn't); that
// every unsupported block of Base survives unless the call failed or
// AllowRemoveOpaque was set (M-SAFE-04); and that Base parsed back, with or
// without ids, reconciles to Base byte for byte (M-ID-01).
func FuzzReconcile(f *testing.F) {
	seeds := []string{
		`[]`,
		`[{"kind":"paragraph","id":"p1","content":[{"text":"a"}]},{"kind":"unsupported","id":"u1"}]`,
		`[{"kind":"callout","id":"c1","tone":"info","children":[{"kind":"paragraph","id":"p2","content":[{"text":"b"}]},{"kind":"unsupported","id":"u1"}]}]`,
		`[{"kind":"infobox","id":"ib1","rows":[{"kind":"keyValue","id":"pr1","rank":"a0","rows":[{"id":"r1","key":"HP","text":"3"}]},{"kind":"unsupported","id":"u2"}]}]`,
		`[{"kind":"secret","id":"s1","children":[{"kind":"paragraph","content":[]}]},{"kind":"wizardry","id":"w1"}]`,
		`[{"kind":"paragraph","content":[{"text":"a"}]},{"kind":"paragraph","id":"p1","content":[{"text":"a"}]}]`,
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add([]byte(a), []byte(b), false)
		}
	}
	f.Fuzz(func(t *testing.T, parsedJSON, baseJSON []byte, allow bool) {
		var parsed, base blocks.List
		if json.Unmarshal(parsedJSON, &parsed) != nil || json.Unmarshal(baseJSON, &base) != nil {
			return
		}
		baseBytes, err := json.Marshal(base)
		if err != nil {
			return
		}
		baseUnique := uniqueIDs(base)
		got, rep, err := reconcile(parsed, base, fakeHooks, allow)
		if again, _ := json.Marshal(base); !bytes.Equal(again, baseBytes) {
			t.Fatalf("base was modified")
		}
		if err != nil {
			if got != nil || rep != nil {
				t.Fatal("blocks or report returned with an error")
			}
		} else {
			checkResult(t, got, base, baseUnique, allow)
		}

		// Base read back (ids and all) and Base without ids.
		var same blocks.List
		if json.Unmarshal(baseBytes, &same) != nil {
			return
		}
		for _, in := range []blocks.List{same, withoutIDs(t, same)} {
			got, _, err := reconcile(in, base, fakeHooks, false)
			if err != nil {
				continue
			}
			if g, _ := json.Marshal(got); !bytes.Equal(g, baseBytes) {
				t.Fatalf("M-ID-01: unchanged page differs\ngot  %s\nbase %s", g, baseBytes)
			}
		}
	})
}

func uniqueIDs(l blocks.List) bool {
	seen := map[string]bool{}
	ok := true
	blocks.Walk(l, func(_ string, n blocks.Node) bool {
		if id := n.NodeID(); id != "" {
			if seen[id] {
				ok = false
			}
			seen[id] = true
		}
		return ok
	})
	return ok
}

func checkResult(t *testing.T, got, base blocks.List, baseUnique, allow bool) {
	t.Helper()
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("result does not encode: %v", err)
	}
	if baseUnique && !uniqueIDs(got) {
		t.Fatalf("result holds an id twice: %s", encode(t, got))
	}
	if allow {
		return
	}
	kept := map[string]bool{}
	blocks.Walk(got, func(_ string, n blocks.Node) bool {
		if isUnsupported(n) {
			kept[n.NodeID()] = true
		}
		return true
	})
	blocks.Walk(base, func(path string, n blocks.Node) bool {
		if isUnsupported(n) && !kept[n.NodeID()] {
			t.Fatalf("M-SAFE-04: base unsupported %q at %s was dropped", n.NodeID(), path)
		}
		return true
	})
}

func TestRemoveErrorMessage(t *testing.T) {
	e := &RemoveError{Removed: []RemovedBlock{{ID: "u1", Kind: blocks.KindUnsupported, Opaque: true}, {ID: "w1", Kind: "wizardry", Opaque: true}}}
	if msg := e.Error(); !strings.Contains(msg, "unsupported u1, wizardry w1") || !strings.Contains(msg, "AllowRemoveOpaque") {
		t.Fatalf("message %q", msg)
	}
	if !errors.Is(e, ErrRemovesOpaque) {
		t.Fatal("RemoveError must match ErrRemovesOpaque")
	}
}
