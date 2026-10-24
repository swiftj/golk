package guard

import (
	"os"
	"testing"
)

// An editor that rewrites the terms file in place truncates it first. A
// Check that reads it then must not run with no terms: a file that held
// terms and now reads empty is refused, and the guard recovers once the
// terms are back.
func TestRedteam_X_GUARD_01_EmptiedFileRefuses(t *testing.T) {
	p := writeTerms(t, "Zorblax\n")
	g := Open(p)
	page := map[string]any{"name": "Zorblax"}
	if got := mustCheck(t, g, page); len(got) != 1 {
		t.Fatalf("first = %v", got)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if vs, err := g.Check(page); err == nil {
		t.Fatalf("an emptied terms file was accepted: %v", vs)
	}
	if err := os.WriteFile(p, []byte("Zorblax\n# again\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mustCheck(t, g, page); len(got) != 1 {
		t.Fatalf("after rewrite = %v", got)
	}
}

// Positional exemptions: "kind":"secret" and a hidden property kind
// exempt only blocks and properties, not text runs.
func TestRedteam_X_GUARD_03_ExemptionsArePositional(t *testing.T) {
	g := Open(writeTerms(t, "Zorblax\n"))
	run := func(extra map[string]any) map[string]any {
		r := map[string]any{"text": "Zorblax"}
		for k, v := range extra {
			r[k] = v
		}
		return map[string]any{"blocks": []any{map[string]any{"kind": "paragraph", "content": []any{r}}}}
	}
	for name, v := range map[string]any{
		"secret run":          run(map[string]any{"kind": "secret"}),
		"hidden property run": run(map[string]any{"kind": "textField", "isHidden": true}),
		"secret under content": map[string]any{"blocks": []any{map[string]any{"kind": "paragraph",
			"content": []any{map[string]any{"kind": "secret", "children": []any{map[string]any{"text": "Zorblax"}}}}}}},
	} {
		if got := mustCheck(t, g, v); len(got) != 1 {
			t.Errorf("%s: violations %v", name, got)
		}
	}
	for name, v := range map[string]any{
		"secret block": map[string]any{"blocks": []any{map[string]any{"kind": "secret",
			"children": []any{map[string]any{"kind": "paragraph", "content": []any{map[string]any{"text": "Zorblax"}}}}}}},
		"hidden property":    map[string]any{"property": map[string]any{"kind": "textField", "isHidden": true, "title": "Zorblax"}},
		"hidden sidebar row": map[string]any{"properties": []any{map[string]any{"kind": "keyValue", "isHidden": true, "title": "Zorblax"}}},
		"hidden infobox row": map[string]any{"blocks": []any{map[string]any{"kind": "infobox",
			"rows": []any{map[string]any{"kind": "textField", "isHidden": true, "title": "Zorblax"}}}}},
	} {
		if got := mustCheck(t, g, v); len(got) != 0 {
			t.Errorf("%s: violations %v", name, got)
		}
	}
}
