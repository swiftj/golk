package mcpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/fontawesome"
	"github.com/swiftj/golk/internal/testutil"
)

// X-ICON-01: glyphs are rewritten into the form LegendKeeper takes.
func TestGlyphs_X_ICON_01_Normalize(t *testing.T) {
	for in, want := range map[string]string{
		"crown":             "crown",
		"  Crown ":          "crown",
		"fa-crown":          "crown",
		"fas fa-crown":      "crown",
		"fa-solid fa-crown": "crown",
		"fa-solid fa-home":  "house",
		"home":              "house",
		"shield-alt":        "shield-halved",
		"Chess Rook":        "chess-rook",
		"hat_wizard":        "hat-wizard",
		"castle":            "castle",
		"":                  "",
	} {
		got := in
		normalizeGlyph(&got)
		if got != want {
			t.Errorf("normalizeGlyph(%q) = %q, want %q", in, got, want)
		}
	}
	normalizeGlyph(nil)
}

// X-ICON-03: every suggested glyph is in the Font Awesome 6 directory.
func TestGlyphs_X_ICON_03_SuggestionsExist(t *testing.T) {
	for _, s := range glyphSuggestions {
		for _, n := range strings.Fields(s.names) {
			if _, ok := fontawesome.Lookup(n); !ok {
				t.Errorf("suggested glyph %s (%s) isn't a Font Awesome %s Free icon", n, s.kind, fontawesome.Version)
			}
		}
	}
}

// X-ICON-05: find_icon searches the directory, filters by category, and
// with no arguments lists the categories.
func TestFindIcon_X_ICON_05(t *testing.T) {
	h := newHarness(t)
	got := text(h.call("find_icon", map[string]any{"query": "king"}))
	if !strings.Contains(got, "\n- crown: Crown (") || !strings.Contains(got, "use the name as icon_glyph") {
		t.Errorf("king:\n%s", got)
	}
	got = text(h.call("find_icon", map[string]any{"query": "horse", "category": "animals", "limit": 3}))
	if !strings.Contains(got, "- horse: Horse") || strings.Count(got, "\n- ") > 3 {
		t.Errorf("horse in animals:\n%s", got)
	}
	got = text(h.call("find_icon", map[string]any{}))
	if !strings.Contains(got, "- animals: Animals, ") {
		t.Errorf("categories:\n%s", got)
	}
	if got = text(h.call("find_icon", map[string]any{"query": "zzzqqq"})); !strings.Contains(got, "No Font Awesome") {
		t.Errorf("nonsense:\n%s", got)
	}
	wtMustFail(t, h, "find_icon", map[string]any{"category": "nope"}, "no Font Awesome category")
	wtMustFail(t, h, "find_icon", map[string]any{"query": "x", "limit": 101}, "limit is 1 to 100")
	if len(h.lk.Requests()) != 0 {
		t.Errorf("find_icon called the API: %v", h.lk.Requests())
	}
}

// X-ICON-01: the tools send the normalized glyph, and a rejected icon gets
// a hint naming the accepted form.
func TestGlyphs_X_ICON_01_Tools(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtRes))
	wtMustOK(t, h, "update_resource", map[string]any{"id": "r1", "icon_glyph": "fa-solid fa-Home"})
	wtReq(t, h.writes()[0], http.MethodPatch, wtPath+"/r1", "", `{"iconGlyph":"house"}`)

	h.lk.Enqueue(wtOK(wtRes))
	wtMustOK(t, h, "create_resource", map[string]any{"name": "Ooo", "icon_glyph": "Shield-Alt"})
	wtReq(t, h.writes()[1], http.MethodPost, wtPath, "", `{"name":"Ooo","iconGlyph":"shield-halved"}`)

	h.lk.Enqueue(wtOK(wtRes), testutil.JSON(http.StatusBadRequest,
		`{"message":"Invalid icon name. Must be a valid FontAwesome icon.","code":"VALIDATION_ERROR","requestId":"rq1"}`))
	got := wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "icon_glyph": "wolf-pack-battalion"}, "Invalid icon name")
	if !strings.Contains(got, "Hint: the icon glyph must be a Font Awesome 6 icon name") {
		t.Errorf("no glyph hint:\n%s", got)
	}
}

// X-ICON-02: every icon_glyph parameter has the shared description, and the
// wiki skill lists exactly the suggested glyphs.
func TestGlyphs_X_ICON_02_Docs(t *testing.T) {
	h := newHarness(t)
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	var walk func(tool string, v any)
	walk = func(tool string, v any) {
		switch v := v.(type) {
		case map[string]any:
			if p, ok := v["properties"].(map[string]any); ok {
				if g, ok := p["icon_glyph"].(map[string]any); ok {
					seen = append(seen, tool)
					if g["description"] != glyphDesc {
						t.Errorf("%s icon_glyph description %q", tool, g["description"])
					}
				}
			}
			for _, c := range v {
				walk(tool, c)
			}
		case []any:
			for _, c := range v {
				walk(tool, c)
			}
		}
	}
	for _, tool := range res.Tools {
		var sc any
		if err := json.Unmarshal(mustJSON(tool.InputSchema), &sc); err != nil {
			t.Fatal(err)
		}
		walk(tool.Name, sc)
	}
	for _, want := range []string{"create_resource", "create_resources", "update_resource", "set_map_feature", "set_timeline_event"} {
		if !strings.Contains(strings.Join(seen, " "), want) {
			t.Errorf("%s has no icon_glyph (have %v)", want, seen)
		}
	}

	skill, err := os.ReadFile(filepath.Join(pluginDir, "skills", "legendkeeper-wiki", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range glyphSuggestions {
		if line := "  - **" + s.kind + ":** " + s.names + "\n"; !strings.Contains(string(skill), line) {
			t.Errorf("the wiki skill lacks the line %q", line)
		}
	}
}
