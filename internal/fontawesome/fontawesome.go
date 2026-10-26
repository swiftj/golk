// Package fontawesome is a directory of the Font Awesome 6 Free icons that
// LegendKeeper takes as icon glyphs: every non-brand icon, with its label,
// categories, search terms and the older names it replaced (D-022).
//
// The data is icons.tsv, generated from Font Awesome's own metadata by
// internal/cmd/faicons (make icons). Font Awesome's metadata is MIT
// licensed; see NOTICE.md.
package fontawesome

import (
	_ "embed"
	"slices"
	"strings"
	"sync"
)

// Version is the Font Awesome Free release icons.tsv was built from.
const Version = "6.7.2"

//go:embed icons.tsv
var tsv string

// Icon is one Font Awesome icon.
type Icon struct {
	Name       string   // the name LegendKeeper takes, e.g. "house"
	Label      string   // e.g. "House"
	Categories []string // category ids, e.g. "buildings"
	Terms      []string // search terms, e.g. "home", "residence"
	Aliases    []string // older names it replaced, e.g. "home"
}

// Category is one of Font Awesome's icon categories.
type Category struct {
	ID, Label string
	Count     int
}

type directory struct {
	icons      []Icon
	byName     map[string]int
	byAlias    map[string]int
	categories []Category
}

var load = sync.OnceValue(func() *directory {
	d := &directory{byName: map[string]int{}, byAlias: map[string]int{}}
	count := map[string]int{}
	for line := range strings.Lines(tsv) {
		f := strings.Split(strings.TrimRight(line, "\n"), "\t")
		switch {
		case strings.HasPrefix(line, "#"):
		case f[0] == "@" && len(f) == 3:
			d.categories = append(d.categories, Category{ID: f[1], Label: f[2]})
		case len(f) == 5:
			ic := Icon{Name: f[0], Label: f[1], Categories: strings.Fields(f[2]), Terms: strings.Fields(f[3]), Aliases: strings.Fields(f[4])}
			d.byName[ic.Name] = len(d.icons)
			for _, a := range ic.Aliases {
				d.byAlias[a] = len(d.icons)
			}
			for _, c := range ic.Categories {
				count[c]++
			}
			d.icons = append(d.icons, ic)
		}
	}
	for i := range d.categories {
		d.categories[i].Count = count[d.categories[i].ID]
	}
	return d
})

// All returns every icon, sorted by name.
func All() []Icon { return slices.Clone(load().icons) }

// Categories returns Font Awesome's categories, sorted by id.
func Categories() []Category { return slices.Clone(load().categories) }

// Lookup returns the icon named name.
func Lookup(name string) (Icon, bool) {
	d := load()
	if i, ok := d.byName[name]; ok {
		return d.icons[i], true
	}
	return Icon{}, false
}

// Canonical returns the current name for name: name itself when it is an
// icon, the icon it was renamed to when it is an older name, or false.
func Canonical(name string) (string, bool) {
	d := load()
	if _, ok := d.byName[name]; ok {
		return name, true
	}
	if i, ok := d.byAlias[name]; ok {
		return d.icons[i].Name, true
	}
	return "", false
}

// Search returns up to limit icons matching every word of query, best
// first, optionally only those in category (an id). An empty query lists
// the category. Matching is on the name, the label, the search terms, the
// older names and the categories.
func Search(query, category string, limit int) []Icon {
	d := load()
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return r == ' ' || r == '-' || r == ',' })
	type hit struct {
		i, score int
	}
	var hits []hit
	for i, ic := range d.icons {
		if category != "" && !slices.Contains(ic.Categories, category) {
			continue
		}
		total := 0
		for _, w := range words {
			s := score(ic, w)
			if s == 0 {
				total = 0
				break
			}
			total += s
		}
		if len(words) > 0 && total == 0 {
			continue
		}
		if ic.Name == strings.Join(words, "-") {
			total += 100
		}
		hits = append(hits, hit{i, total})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Icon, len(hits))
	for k, h := range hits {
		out[k] = d.icons[h.i]
	}
	return out
}

// score is how well one query word matches ic, 0 for not at all.
func score(ic Icon, w string) int {
	best := 0
	up := func(s int) { best = max(best, s) }
	for _, part := range strings.Split(ic.Name, "-") {
		if part == w {
			up(10)
		} else if strings.HasPrefix(part, w) {
			up(5)
		}
	}
	for _, part := range strings.Fields(strings.ToLower(ic.Label)) {
		if part == w {
			up(8)
		}
	}
	for _, t := range ic.Terms {
		if t == w || slices.Contains(strings.Split(t, "-"), w) {
			up(6)
		} else if strings.HasPrefix(t, w) {
			up(3)
		}
	}
	for _, a := range ic.Aliases {
		if slices.Contains(strings.Split(a, "-"), w) {
			up(4)
		}
	}
	if slices.Contains(ic.Categories, w) {
		up(2)
	}
	return best
}
