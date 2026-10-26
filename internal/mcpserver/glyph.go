package mcpserver

import (
	"slices"
	"strings"
)

// Icon glyphs (D-022). LegendKeeper takes a Font Awesome 6 icon name,
// bare and lowercase ("crown"), including Pro icons ("castle"); it rejects
// prefixes ("fa-crown"), capitals, brand icons and Font Awesome 5 names
// ("home"), all with the same VALIDATION_ERROR and no field. Agents often
// write those forms, so glyphs are normalized before they're sent.

// glyphDesc is the schema description of every icon_glyph parameter.
const glyphDesc = "a Font Awesome 6 icon name, lowercase with no fa- prefix, e.g. crown, castle, dragon, skull, house; Pro icons work, brand icons and Font Awesome 5 names don't; empty clears it"

// faStyleWords are the style tokens of a Font Awesome class list, dropped
// from a glyph ("fa-solid fa-crown", "fas fa-crown").
var faStyleWords = []string{"fa", "fas", "far", "fal", "fat", "fad", "fab", "fass", "fa-solid", "fa-regular", "fa-light",
	"fa-thin", "fa-duotone", "fa-brands", "fa-sharp"}

// glyphAliases maps Font Awesome 5 names, which LegendKeeper rejects, to
// their Font Awesome 6 names. Every target is checked against the sandbox
// (TestIntegration_Glyphs_X_ICON_03).
var glyphAliases = map[string]string{
	"balance-scale":        "scale-balanced",
	"beer":                 "beer-mug-empty",
	"book-dead":            "book-skull",
	"calendar-alt":         "calendar-days",
	"coffee":               "mug-saucer",
	"exclamation-triangle": "triangle-exclamation",
	"feather-alt":          "feather-pointed",
	"fire-alt":             "fire-flame-curved",
	"fist-raised":          "hand-fist",
	"glass-cheers":         "champagne-glasses",
	"glass-martini":        "martini-glass-empty",
	"globe-americas":       "earth-americas",
	"globe-europe":         "earth-europe",
	"home":                 "house",
	"hospital-alt":         "hospital",
	"info-circle":          "circle-info",
	"magic":                "wand-magic",
	"map-marked":           "map-location",
	"map-marked-alt":       "map-location-dot",
	"map-marker":           "location-pin",
	"map-marker-alt":       "location-dot",
	"map-signs":            "signs-post",
	"question-circle":      "circle-question",
	"shield-alt":           "shield-halved",
	"shopping-cart":        "cart-shopping",
	"store-alt":            "shop",
	"theater-masks":        "masks-theater",
	"times":                "xmark",
	"tint":                 "droplet",
	"university":           "building-columns",
	"user-alt":             "user-large",
	"user-friends":         "user-group",
	"users-cog":            "users-gear",
	"wine-glass-alt":       "wine-glass-empty",
}

// glyphSuggestions are glyphs for a world wiki, each checked against the
// sandbox (TestIntegration_Glyphs_X_ICON_03). The wiki skill lists the same ones
// (TestGlyphs_X_ICON_02).
var glyphSuggestions = []struct{ kind, names string }{
	{"People", "user person person-dress user-crown user-tie user-secret user-ninja user-shield hat-wizard people-group users crown"},
	{"Places", "castle fort chess-rook dungeon tower-observation city house house-chimney cabin tent campground church place-of-worship landmark monument archway bridge shop building-columns school"},
	{"Land and sea", "mountain mountain-sun volcano tree trees island-tropical water globe map map-location-dot location-dot anchor ship sailboat compass"},
	{"Factions and faiths", "shield shield-halved flag ankh cross yin-yang star-of-david dharmachakra om peace masks-theater"},
	{"Creatures", "dragon horse unicorn spider crow dove fish frog paw ghost skull skull-crossbones"},
	{"Items", "sword swords dagger axe axe-battle bow-arrow hammer ring gem coins sack-dollar key scroll book book-open book-skull flask wand-magic-sparkles staff"},
	{"Events and ideas", "fire bolt snowflake sun moon star hourglass calendar-days eye heart heart-crack trophy music utensils beer-mug-empty hand-fist scale-balanced gavel"},
}

// normalizeGlyph rewrites *p into the form LegendKeeper takes: lowercase,
// without Font Awesome class prefixes, words joined by hyphens, and Font
// Awesome 5 names renamed. An empty glyph (which clears it) is kept.
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
	if to, ok := glyphAliases[g]; ok {
		g = to
	}
	*p = g
}

// glyphHint is the hint for a rejected glyph.
const glyphHint = "the icon glyph must be a Font Awesome 6 icon name, lowercase with no fa- prefix (crown, castle, dragon, skull, house, shield-halved). Font Awesome 5 names such as home or shield-alt and brand icons are rejected; pick a different name"

// glyphsToCheck is every suggested glyph and rename target, once each.
func glyphsToCheck() []string {
	var out []string
	for _, s := range glyphSuggestions {
		out = append(out, strings.Fields(s.names)...)
	}
	for _, to := range glyphAliases {
		out = append(out, to)
	}
	slices.Sort(out)
	return slices.Compact(out)
}
