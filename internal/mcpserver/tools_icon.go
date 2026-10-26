package mcpserver

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/fontawesome"
	"github.com/swiftj/golk/lk"
)

// find_icon searches the Font Awesome 6 Free directory built into lk-mcp
// (D-022), so an agent picks an icon_glyph LegendKeeper takes instead of
// guessing. It makes no API call.

// FindIconIn is find_icon's input.
type FindIconIn struct {
	Query    string `json:"query,omitempty" jsonschema:"what the icon should show, e.g. king, tavern, ship, magic; omit to browse a category, or omit both to list the categories"`
	Category string `json:"category,omitempty" jsonschema:"only icons in this Font Awesome category id, e.g. animals, buildings, maps"`
	Limit    int    `json:"limit,omitempty" jsonschema:"at most this many icons, 1-100; default 25"`
}

// IconOut is one icon find_icon found.
type IconOut struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Categories []string `json:"categories,omitempty"`
}

// IconCategoryOut is one Font Awesome category.
type IconCategoryOut struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// FindIconOut is find_icon's output.
type FindIconOut struct {
	FontAwesome string            `json:"font_awesome"`
	Icons       []IconOut         `json:"icons,omitempty"`
	Categories  []IconCategoryOut `json:"categories,omitempty"`
}

func (s *server) addIconTools(ms *mcp.Server) {
	addTool(s, ms, &mcp.Tool{
		Name: "find_icon",
		Description: "Find Font Awesome 6 icon names to use as icon_glyph (resources, map pins, timeline events). " +
			"Searches names, labels and search terms of every free icon, best match first; with category, only that category; " +
			"with neither, lists the categories. Use a returned name exactly.",
		Annotations: readOnly("Find icon"),
	}, func(FindIconIn) string { return "" }, s.findIcon)
}

func (s *server) findIcon(_ context.Context, _ *lk.Client, in FindIconIn) (toolResult[FindIconOut], error) {
	var zero toolResult[FindIconOut]
	out := FindIconOut{FontAwesome: fontawesome.Version}
	var b strings.Builder
	cats := fontawesome.Categories()
	if in.Limit < 0 || in.Limit > 100 {
		return zero, fmt.Errorf("limit is 1 to 100, not %d", in.Limit)
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	if in.Category != "" && !slices.ContainsFunc(cats, func(c fontawesome.Category) bool { return c.ID == in.Category }) {
		return zero, fmt.Errorf("no Font Awesome category %q; call find_icon with no query or category to list them", in.Category)
	}
	if strings.TrimSpace(in.Query) == "" && in.Category == "" {
		fmt.Fprintf(&b, "Font Awesome %s categories (pass an id as category):\n", fontawesome.Version)
		for _, c := range cats {
			out.Categories = append(out.Categories, IconCategoryOut{ID: c.ID, Label: c.Label, Count: c.Count})
			fmt.Fprintf(&b, "- %s: %s, %d icons\n", c.ID, c.Label, c.Count)
		}
		return toolResult[FindIconOut]{out: out, text: b.String()}, nil
	}
	icons := fontawesome.Search(in.Query, in.Category, in.Limit)
	if len(icons) == 0 {
		fmt.Fprintf(&b, "No Font Awesome %s icon matches %q. Try a simpler word or a synonym, or browse a category (find_icon with no query lists them).", fontawesome.Version, in.Query)
		return toolResult[FindIconOut]{out: out, text: b.String()}, nil
	}
	what := fmt.Sprintf("%q", in.Query)
	if in.Query == "" {
		what = "category " + in.Category
	} else if in.Category != "" {
		what += " in category " + in.Category
	}
	fmt.Fprintf(&b, "%d Font Awesome %s icon(s) for %s; use the name as icon_glyph:\n", len(icons), fontawesome.Version, what)
	for _, ic := range icons {
		out.Icons = append(out.Icons, IconOut{Name: ic.Name, Label: ic.Label, Categories: ic.Categories})
		fmt.Fprintf(&b, "- %s: %s", ic.Name, ic.Label)
		if terms := ic.Terms[:min(len(ic.Terms), 6)]; len(terms) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(terms, ", "))
		}
		b.WriteString("\n")
	}
	return toolResult[FindIconOut]{out: out, text: b.String()}, nil
}
