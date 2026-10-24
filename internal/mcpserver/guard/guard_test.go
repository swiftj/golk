package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swiftj/golk/lk/blocks"
)

// Every term in these tests is made up.

func writeTerms(t testing.TB, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustCheck(t *testing.T, g *Guard, v any) []Violation {
	t.Helper()
	vs, err := g.Check(v)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return vs
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestGuard_X_GUARD_01_TermsFile(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
		wantErr string
	}{
		{"comments and blanks", "# DM only\n\nZorblax\n   # indented comment\n  the Crimson Accord  \r\n\t\n", []string{"Zorblax", "the Crimson Accord"}, ""},
		{"empty file", "", nil, ""},
		{"only comments", "# nothing\n#\n", nil, ""},
		{"bom", "\uFEFF# c\nMirevale\n", []string{"Mirevale"}, ""},
		{"hash inside term kept", "Mire#vale\n", []string{"Mire#vale"}, ""},
		{"no trailing newline", "Zorblax", []string{"Zorblax"}, ""},
		{"invalid utf8", "Zorblax\nMire\xffvale\n", nil, "line 2 is not valid UTF-8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTerms([]byte(c.content))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				if strings.Contains(err.Error(), "Mire") {
					t.Fatalf("error leaks a term: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("terms = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGuard_X_GUARD_01_EmptyListAllowsEverything(t *testing.T) {
	g := Open(writeTerms(t, "# none yet\n"))
	if vs := mustCheck(t, g, map[string]any{"name": "Zorblax"}); len(vs) != 0 {
		t.Fatalf("violations = %v", vs)
	}
}

func TestGuard_X_GUARD_01_FileErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("Zorblax\n\xc3\x28\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path string
	}{
		{"missing", filepath.Join(dir, "absent.txt")},
		{"directory", dir},
		{"invalid utf8", bad},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := Open(c.path)
			vs, err := g.Check(map[string]any{"name": "hello"})
			if err == nil {
				t.Fatalf("Check succeeded with %v", vs)
			}
			if !strings.Contains(err.Error(), c.path) {
				t.Errorf("error %q lacks the path", err)
			}
			if strings.Contains(err.Error(), "Zorblax") {
				t.Errorf("error %q leaks a term", err)
			}
		})
	}
}

func TestGuard_X_GUARD_01_ReloadOnChange(t *testing.T) {
	p := writeTerms(t, "Zorblax\n")
	g := Open(p)
	page := map[string]any{"name": "Zorblax of Mirevale"}
	if got := mustCheck(t, g, page); len(got) != 1 || got[0].Term != "Zorblax" {
		t.Fatalf("first check = %v", got)
	}
	// Same size and different content, with mtime moved forward: only the
	// mtime tells the change apart.
	if err := os.WriteFile(p, []byte("Mirevale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if got := mustCheck(t, g, page); len(got) != 1 || got[0].Term != "Mirevale" {
		t.Fatalf("after rewrite = %v", got)
	}
	// Size change alone (same mtime) also reloads.
	if err := os.WriteFile(p, []byte("Mirevale\nZorblax\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if n, err := g.Terms(); err != nil || n != 2 {
		t.Fatalf("Terms = %d, %v", n, err)
	}
	// The file going away makes Check fail, even though terms were loaded.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Check(page); err == nil {
		t.Fatal("Check succeeded after the terms file was removed")
	}
	// And it recovers when the file comes back.
	if err := os.WriteFile(p, []byte("Zorblax\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mustCheck(t, g, page); len(got) != 1 {
		t.Fatalf("after restore = %v", got)
	}
}

func TestGuard_X_GUARD_01_Concurrent(t *testing.T) {
	p := writeTerms(t, "Zorblax\n")
	g := Open(p)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				if i == 0 && j%10 == 0 {
					_ = os.WriteFile(p, []byte(strings.Repeat("Zorblax\n", 1+j%3)), 0o600)
				}
				vs, err := g.Check(map[string]any{"name": "Zorblax"})
				if err == nil && len(vs) != 1 {
					t.Errorf("violations = %v", vs)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestGuard_X_GUARD_02_Matching(t *testing.T) {
	cases := []struct {
		term, text string
		want       bool
	}{
		{"Zorblax", "Zorblax", true},
		{"Zorblax", "zORBLAX rises", true},
		{"Ærendil", "the tomb of ærendil", true},
		{"ærendil", "ÆRENDIL", true},
		{"Σοφία", "ΣΟΦΊΑ", true},
		{"kaz", "\u212Aaz", true}, // Kelvin sign folds with k
		{"Zorblax", "Zorblaxian", false},
		{"Zorblax", "AZorblax", false},
		{"Zorblax", "Zorblax2", false},
		{"Zorblax", "Zorblax's tower", true},
		{"Zorblax", "Zorblax\u2019s tower", true},
		{"Zorblax", "(Zorblax)", true},
		{"Zorblax", "Zorblax.", true},
		{"Zorblax", "Zorblax_v2", true}, // '_' is a boundary
		{"Zorblax", "x-Zorblax-y", true},
		{"Zorblax", "Zor\u00ADblax", true}, // soft hyphen is invisible
		{"Zorblax", "Zor\u200Bblax", true}, // zero-width space too
		{"Zorblax", "Zor blax", false},
		{"Zorblax", "", false},
		{"the Crimson Accord", "sign the Crimson Accord now", true},
		{"the Crimson Accord", "the\nCrimson\tAccord", true},
		{"the Crimson Accord", "the   Crimson \u00A0\n Accord", true},
		{"the  Crimson   Accord", "the Crimson Accord", true}, // term spacing normalized
		{"the Crimson Accord", "theCrimson Accord", false},
		{"the Crimson Accord", "the Crimson Accordance", false},
		{"the Crimson Accord", "bathe Crimson Accord", false},
		{"the Crimson Accord", "the Crimson, Accord", false},
		{"#seal", "break the #seal", true}, // non-word edge needs no boundary
		{"#seal", "x#seal", true},
		{"O'Mire", "o'mire said", true},
		{"42", "vault 42", true},
		{"42", "vault 420", false},
	}
	for _, c := range cases {
		t.Run(c.term+"|"+c.text, func(t *testing.T) {
			if got := Matches(c.term, c.text); got != c.want {
				t.Fatalf("Matches(%q, %q) = %v, want %v", c.term, c.text, got, c.want)
			}
		})
	}
}

func TestGuard_X_GUARD_02_ReportsOncePerPathInOrder(t *testing.T) {
	g := Open(writeTerms(t, "Mirevale\nZorblax\nzorblax\n"))
	got := mustCheck(t, g, raw(`{"a":"Zorblax and Zorblax in Mirevale","b":["mirevale","none"]}`))
	want := []Violation{
		{Term: "Mirevale", Path: "$.a"},
		{Term: "Zorblax", Path: "$.a"}, // duplicate term (by folding) reported once, as first written
		{Term: "Mirevale", Path: "$.b[0]"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestGuard_X_GUARD_03_Visible(t *testing.T) {
	g := Open(writeTerms(t, "Zorblax\nthe Crimson Accord\n"))
	cases := []struct {
		name string
		v    any
		want []Violation
	}{
		{"name", map[string]any{"name": "Zorblax"}, []Violation{{"Zorblax", "$.name"}}},
		{"tags", map[string]any{"tags": []string{"npc", "zorblax"}}, []Violation{{"Zorblax", "$.tags[1]"}}},
		{"keys are not checked", raw(`{"Zorblax":"fine"}`), nil},
		{"weird key path", raw(`{"weird key":"Zorblax","a.b":["Zorblax"]}`), []Violation{{"Zorblax", `$["weird key"]`}, {"Zorblax", `$["a.b"][0]`}}},
		{"numbers bools null ignored", raw(`{"a":1,"b":true,"c":null,"d":1e400}`), nil},
		{"url", blocks.List{blocks.Image("https://example.test/zorblax.png", "a map")}, []Violation{{"Zorblax", "$[0].url"}}},
		{"alt text", blocks.List{blocks.Image("https://example.test/x.png", "portrait of Zorblax")}, []Violation{{"Zorblax", "$[0].alt"}}},
		{"top-level secret", blocks.List{blocks.P("hello"), blocks.Secret(blocks.P("Zorblax"))}, nil},
		{"nested secrets", blocks.List{blocks.Secret(blocks.Quote(blocks.Secret(blocks.P("Zorblax"))))}, nil},
		{
			"secret inside a list inside a quote",
			blocks.List{blocks.Quote(blocks.UL(blocks.LI(blocks.P("open"), blocks.Secret(blocks.P("the Crimson Accord")))))},
			nil,
		},
		{
			"public sibling of a secret in a quote",
			blocks.List{blocks.Quote(blocks.P("Zorblax"), blocks.Secret(blocks.P("Zorblax")))},
			[]Violation{{"Zorblax", "$[0].children[0].content[0].text"}},
		},
		{"mention label", raw(`[{"kind":"paragraph","content":[{"mention":{"resourceId":"abc12345","label":"Zorblax"}}]}]`), []Violation{{"Zorblax", "$[0].content[0].mention.label"}}},
		{"mention name", raw(`[{"kind":"paragraph","content":[{"mention":{"resourceId":"abc12345","name":"the Crimson Accord"}}]}]`), []Violation{{"the Crimson Accord", "$[0].content[0].mention.name"}}},
		{
			"hidden infobox property",
			raw(`[{"kind":"infobox","properties":[{"kind":"textField","isHidden":true,"title":"Zorblax","content":[{"text":"Zorblax"}]},{"kind":"textField","title":"Zorblax"}]}]`),
			[]Violation{{"Zorblax", "$[0].properties[1].title"}},
		},
		{"isHidden false is visible", raw(`{"isHidden":false,"t":"Zorblax"}`), []Violation{{"Zorblax", "$.t"}}},
		{"isHidden string is visible", raw(`{"isHidden":"true","t":"Zorblax"}`), []Violation{{"Zorblax", "$.t"}}},
		{"kind case-sensitive", raw(`{"kind":"Secret","t":"Zorblax"}`), []Violation{{"Zorblax", "$.t"}}},
		{"key case-sensitive", raw(`{"Kind":"secret","IsHidden":true,"t":"Zorblax"}`), []Violation{{"Zorblax", "$.t"}}},
		{"hidden key after content", raw(`{"t":"Zorblax","isHidden":true}`), nil},
		{"secret kind after content", raw(`{"children":[{"text":"Zorblax"}],"kind":"secret"}`), nil},
		{"html escaping", map[string]any{"t": "<Zorblax>&"}, []Violation{{"Zorblax", "$.t"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustCheck(t, g, c.v)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v\nwant %v", got, c.want)
			}
		})
	}
}

func TestGuard_X_GUARD_03_ServerShape(t *testing.T) {
	// What tools_write.go passes: a map with name and blocks.
	g := Open(writeTerms(t, "Zorblax\n"))
	got := mustCheck(t, g, map[string]any{
		"name":   "Zorblax's Lair",
		"blocks": blocks.List{blocks.H(2, "Lair"), blocks.P("Ruled by ", blocks.Bold("Zorblax")), blocks.Secret(blocks.P("Zorblax"))},
	})
	want := []Violation{{"Zorblax", "$.blocks[1].content[1].text"}, {"Zorblax", "$.name"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestGuard_X_GUARD_03_RawBlocks(t *testing.T) {
	g := Open(writeTerms(t, "Zorblax\n"))
	var l blocks.List
	in := `[{"kind":"futureThing","id":"aaaaaaaa","payload":{"deep":["Zorblax"]}},` +
		`{"kind":"secret","id":"bbbbbbbb","children":[{"kind":"futureThing","note":"Zorblax"}]}]`
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	if _, ok := l[0].(*blocks.RawBlock); !ok {
		t.Fatalf("block 0 is %T, want *blocks.RawBlock", l[0])
	}
	got := mustCheck(t, g, l)
	want := []Violation{{"Zorblax", "$[0].payload.deep[0]"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}

	// A raw block that is itself a secret is exempt.
	rawSecret := &blocks.RawBlock{KindName: "secret", JSON: raw(`{"kind":"secret","extra":{"x":"Zorblax"}}`)}
	if got := mustCheck(t, g, blocks.List{rawSecret}); len(got) != 0 {
		t.Fatalf("raw secret flagged: %v", got)
	}
}

func TestGuard_X_GUARD_03_DuplicateKeys(t *testing.T) {
	g := Open(writeTerms(t, "Zorblax\n"))
	cases := []struct {
		name string
		json string
		want []Violation
	}{
		// Exempt only when every occurrence agrees: safe under first-wins
		// and last-wins parsers alike.
		{"hidden then shown", `{"isHidden":true,"isHidden":false,"t":"Zorblax"}`, []Violation{{"Zorblax", "$.t"}}},
		{"shown then hidden", `{"isHidden":false,"isHidden":true,"t":"Zorblax"}`, []Violation{{"Zorblax", "$.t"}}},
		{"hidden twice", `{"isHidden":true,"t":"Zorblax","isHidden":true}`, nil},
		{"secret then paragraph", `{"kind":"secret","kind":"paragraph","t":"Zorblax"}`, []Violation{{"Zorblax", "$.t"}}},
		{"paragraph then secret", `{"kind":"paragraph","kind":"secret","t":"Zorblax"}`, []Violation{{"Zorblax", "$.t"}}},
		{"secret kind, mixed hidden", `{"kind":"secret","isHidden":false,"t":"Zorblax"}`, nil},
		// Every occurrence of a duplicated string is checked.
		{"shadowed text", `{"t":"Zorblax","t":"fine"}`, []Violation{{"Zorblax", "$.t"}}},
		{"shadowing text", `{"t":"fine","t":"Zorblax"}`, []Violation{{"Zorblax", "$.t"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Through a RawBlock, the way such JSON reaches the guard.
			l := blocks.List{&blocks.RawBlock{KindName: "x", JSON: raw(c.json)}}
			got := mustCheck(t, g, l)
			for i := range c.want {
				c.want[i].Path = "$[0]" + strings.TrimPrefix(c.want[i].Path, "$")
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v\nwant %v", got, c.want)
			}
		})
	}
}

func TestGuard_X_GUARD_03_EncodeError(t *testing.T) {
	g := Open(writeTerms(t, "Zorblax\n"))
	if _, err := g.Check(map[string]any{"f": func() {}}); err == nil {
		t.Fatal("Check of an unencodable value succeeded")
	}
}
