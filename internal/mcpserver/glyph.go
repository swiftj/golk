package mcpserver

import (
	"slices"
	"strings"

	"github.com/swiftj/golk/internal/fontawesome"
)

// Icon glyphs (D-022). LegendKeeper takes a Font Awesome 6 icon name,
// bare and lowercase ("crown"); it rejects prefixes ("fa-crown"), capitals,
// brand icons and Font Awesome 5 names ("home"), all with the same
// VALIDATION_ERROR and no field. Agents often write those forms, so glyphs
// are normalized before they're sent, and find_icon searches the Font
// Awesome 6 Free directory (internal/fontawesome).

// glyphDesc is the schema description of every icon_glyph parameter.
const glyphDesc = "a Font Awesome 6 icon name, lowercase with no fa- prefix, e.g. crown, chess-rook, dragon, skull, house; look names up with find_icon; empty clears it"

// faStyleWords are the style tokens of a Font Awesome class list, dropped
// from a glyph ("fa-solid fa-crown", "fas fa-crown").
var faStyleWords = []string{"fa", "fas", "far", "fal", "fat", "fad", "fab", "fass", "fa-solid", "fa-regular", "fa-light",
	"fa-thin", "fa-duotone", "fa-brands", "fa-sharp"}

// glyphSuggestions are Font Awesome 6 Free glyphs for a world wiki. The
// wiki skill lists the same ones (X-ICON-02), and every one is in the
// directory (X-ICON-03).
var glyphSuggestions = []struct{ kind, names string }{
	{"People", "user person person-dress user-tie user-secret user-ninja user-shield hat-wizard people-group users crown"},
	{"Places", "chess-rook dungeon tower-observation city tree-city mountain-city house house-chimney tent campground church place-of-worship landmark monument archway bridge shop building-columns school"},
	{"Land and sea", "mountain mountain-sun volcano tree water globe map map-location-dot location-dot anchor ship sailboat compass"},
	{"Factions and faiths", "shield shield-halved flag ankh cross yin-yang star-of-david dharmachakra om peace masks-theater"},
	{"Creatures", "dragon horse horse-head cat dog spider crow dove fish frog paw ghost skull skull-crossbones bone"},
	{"Items", "hammer wand-sparkles wand-magic-sparkles bomb ring gem coins sack-dollar key scroll book book-open book-skull flask feather dice-d20"},
	{"Events and ideas", "fire bolt explosion snowflake sun moon star hourglass calendar-days eye heart heart-crack trophy music utensils beer-mug-empty hand-fist scale-balanced gavel"},
}

// normalizeGlyph rewrites *p into the form LegendKeeper takes: lowercase,
// without Font Awesome class prefixes, words joined by hyphens, and an
// older Font Awesome name renamed to its current one. A name not in the
// directory is kept (LegendKeeper also takes Pro icons). An empty glyph
// (which clears it) is kept.
func normalizeGlyph(p *string) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower(strings.ReplaceAll(*p, "_", " "))) {
		if !slices.Contains(faStyleWords, w) {
			words = append(words, w)
		}
	}
	g := strings.TrimPrefix(strings.Join(words, "-"), "fa-")
	if name, ok := fontawesome.Canonical(g); ok {
		g = name
	}
	*p = g
}

// glyphHint is the hint for a rejected glyph.
const glyphHint = "the icon glyph must be a Font Awesome 6 icon name, lowercase with no fa- prefix (crown, chess-rook, dragon, skull, house). Brand icons are rejected. Search for a name with find_icon, e.g. find_icon query \"castle\""
