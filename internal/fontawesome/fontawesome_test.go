package fontawesome

import (
	"slices"
	"strings"
	"testing"
)

// X-ICON-04: the directory holds the Font Awesome 6 Free non-brand icons,
// resolves older names, and finds icons by name, label, term and category.
func TestDirectory_X_ICON_04(t *testing.T) {
	if !strings.HasPrefix(tsv, "# Font Awesome Free "+Version+" ") {
		t.Errorf("icons.tsv header doesn't name version %s", Version)
	}
	if n := len(All()); n < 1300 {
		t.Errorf("%d icons", n)
	}
	if _, ok := Lookup("crown"); !ok {
		t.Error("no crown")
	}
	for _, brand := range []string{"github", "fort-awesome", "wolf-pack-battalion"} {
		if _, ok := Lookup(brand); ok {
			t.Errorf("brand icon %s is listed", brand)
		}
	}
	for from, want := range map[string]string{"house": "house", "home": "house", "shield-alt": "shield-halved", "magic": "wand-magic"} {
		if got, ok := Canonical(from); !ok || got != want {
			t.Errorf("Canonical(%q) = %q, %v", from, got, ok)
		}
	}
	if _, ok := Canonical("castle"); ok {
		t.Error("castle is a Pro icon, not in the Free directory")
	}

	names := func(ics []Icon) []string {
		var out []string
		for _, ic := range ics {
			out = append(out, ic.Name)
		}
		return out
	}
	for q, want := range map[string]string{"crown": "crown", "king": "crown", "home": "house", "beer": "beer-mug-empty", "dragon": "dragon", "chess rook": "chess-rook"} {
		if got := names(Search(q, "", 10)); !slices.Contains(got, want) {
			t.Errorf("Search(%q) = %v, lacks %s", q, got, want)
		}
	}
	if got := Search("crown", "", 5); got[0].Name != "crown" {
		t.Errorf("Search(crown)[0] = %s", got[0].Name)
	}
	if got := Search("zzzqqq", "", 5); len(got) != 0 {
		t.Errorf("Search(nonsense) = %v", names(got))
	}
	animals := Search("", "animals", 0)
	if len(animals) < 20 || !slices.Contains(names(animals), "dragon") {
		t.Errorf("animals = %v", names(animals))
	}
	if got := names(Search("horse", "animals", 0)); !slices.Contains(got, "horse") {
		t.Errorf("horse in animals = %v", got)
	}
	cats := Categories()
	i := slices.IndexFunc(cats, func(c Category) bool { return c.ID == "animals" })
	if i < 0 || cats[i].Label != "Animals" || cats[i].Count != len(animals) {
		t.Errorf("animals category %+v, %d icons", cats, len(animals))
	}
}
