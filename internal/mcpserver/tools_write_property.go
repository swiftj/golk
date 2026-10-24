package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

// RowIn is one key-value row of set_property.
type RowIn struct {
	Key   string `json:"key" jsonschema:"the row's key"`
	Value string `json:"value" jsonschema:"the row's value as inline LK Markdown (one paragraph: **bold**, links, mentions); empty for none"`
}

// SetPropertyIn is set_property's input.
type SetPropertyIn struct {
	ProjectArg
	DryRunArg
	ResourceID    string   `json:"resource_id" jsonschema:"the resource whose sidebar property to set"`
	Title         string   `json:"title" jsonschema:"the property's title; an existing property with exactly this title (case-sensitive) is updated, otherwise one is created"`
	Markdown      *string  `json:"markdown,omitempty" jsonschema:"a text field's body as LK Markdown; not with rows"`
	Rows          *[]RowIn `json:"rows,omitempty" jsonschema:"key-value rows, replacing all rows; not with markdown"`
	IsHidden      *bool    `json:"is_hidden,omitempty" jsonschema:"true hides the property from players; false shows it"`
	IsTitleHidden *bool    `json:"is_title_hidden,omitempty" jsonschema:"true hides the title above the property"`
}

// PropertyOut describes a sidebar property after a write.
type PropertyOut struct {
	ResourceID string `json:"resource_id"`
	PropertyID string `json:"property_id,omitempty"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Hidden     bool   `json:"hidden"`
	Created    bool   `json:"created,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func propTitle(p blocks.Property) (string, bool) {
	env := p.PropertyEnvelope()
	if env == nil {
		return "", false
	}
	return env.Title.Get()
}

// byTitle returns the properties titled exactly title.
func byTitle(ps []blocks.Property, title string) []blocks.Property {
	var out []blocks.Property
	for _, p := range ps {
		if t, ok := propTitle(p); ok && t == title {
			out = append(out, p)
		}
	}
	return out
}

func propIDs(ps []blocks.Property) string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.PropertyEnvelope().ID
	}
	return strings.Join(ids, ", ")
}

// inlineRuns parses one row value: inline LK Markdown making at most one
// paragraph.
func inlineRuns(key, v string) (blocks.Runs, error) {
	if strings.TrimSpace(v) == "" {
		return blocks.Runs{}, nil
	}
	l, _, err := markdown.FromMarkdown(v, markdown.FromOptions{})
	if err != nil {
		return nil, fmt.Errorf("row %q: %w", key, err)
	}
	if len(l) == 1 {
		if p, ok := l[0].(*blocks.ParagraphBlock); ok {
			return p.Content, nil
		}
	}
	return nil, fmt.Errorf("row %q: the value must be one paragraph of inline Markdown, not %d block(s)", key, len(l))
}

func kvRows(rows []RowIn) (blocks.KVRows, error) {
	out := blocks.KVRows{}
	for _, r := range rows {
		runs, err := inlineRuns(r.Key, r.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, blocks.KVRow{Key: r.Key, Content: runs})
	}
	return out, nil
}

// asJSONMap is p as a generic JSON object, for building the property as it
// will be after a patch.
func asJSONMap(p any) (map[string]any, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(raw, &m)
}

func (s *server) setProperty(ctx context.Context, c *lk.Client, in SetPropertyIn) (toolResult[PropertyOut], error) {
	var zero toolResult[PropertyOut]
	if in.Title == "" {
		return zero, errors.New("title is required")
	}
	if in.Markdown != nil && in.Rows != nil {
		return zero, errors.New("give markdown (a text field) or rows (key-value), not both")
	}
	var children blocks.List
	if in.Markdown != nil {
		var err error
		if children, _, err = markdown.FromMarkdown(*in.Markdown, markdown.FromOptions{}); err != nil {
			return zero, err
		}
		if children == nil {
			children = blocks.List{}
		}
	}
	var rows blocks.KVRows
	if in.Rows != nil {
		var err error
		if rows, err = kvRows(*in.Rows); err != nil {
			return zero, err
		}
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	list, err := c.Properties.List(ctx, r.ID)
	if err != nil {
		return zero, err
	}
	same := byTitle(list.Items, in.Title)
	if len(same) > 1 {
		return zero, fmt.Errorf("resource %q (%s) has %d properties titled %q (%s); nothing was written. Hint: rename one, or use delete_property with property_id",
			r.Name, r.ID, len(same), in.Title, propIDs(same))
	}
	out := PropertyOut{ResourceID: r.ID, Title: in.Title, DryRun: in.DryRun}
	var visible any
	var did string
	var create blocks.Property
	var patch lk.PropertyPatch
	if len(same) == 0 {
		env := blocks.Envelope{Title: lk.Some(in.Title)}
		if in.IsHidden != nil {
			env.IsHidden = lk.Some(*in.IsHidden)
		}
		if in.IsTitleHidden != nil {
			env.IsTitleHidden = lk.Some(*in.IsTitleHidden)
		}
		switch {
		case in.Markdown != nil:
			create = &blocks.TextFieldProperty{Envelope: env, Children: children}
		case in.Rows != nil:
			create = &blocks.KeyValueProperty{Envelope: env, Rows: rows}
		default:
			return zero, fmt.Errorf("resource %q has no property titled %q; to create it give markdown (a text field) or rows (key-value)", r.Name, in.Title)
		}
		out.Kind, out.Hidden, out.Created = string(create.Kind()), in.IsHidden != nil && *in.IsHidden, true
		visible, did = create, fmt.Sprintf("create %s property %q", create.Kind(), in.Title)
	} else {
		p := same[0]
		env := p.PropertyEnvelope()
		out.PropertyID, out.Kind = env.ID, string(p.Kind())
		hidden, _ := env.IsHidden.Get()
		titleHidden, _ := env.IsTitleHidden.Get()
		out.Hidden = hidden
		switch {
		case in.Markdown != nil && p.Kind() != blocks.KindTextField:
			return zero, fmt.Errorf("property %q (%s) is a %s property, not a text field; markdown can't change its kind. Hint: give rows for a keyValue property, or delete_property and set it again", in.Title, env.ID, p.Kind())
		case in.Rows != nil && p.Kind() != blocks.KindKeyValue:
			return zero, fmt.Errorf("property %q (%s) is a %s property, not key-value; rows can't change its kind. Hint: give markdown for a textField property, or delete_property and set it again", in.Title, env.ID, p.Kind())
		}
		after, err := asJSONMap(p)
		if err != nil {
			return zero, err
		}
		var changes []string
		if in.Markdown != nil {
			patch.Children, after["children"] = lk.Some(children), children
			changes = append(changes, "set its text")
		}
		if in.Rows != nil {
			patch.Rows, after["rows"] = lk.Some(rows), rows
			changes = append(changes, fmt.Sprintf("set its %d row(s)", len(rows)))
		}
		if in.IsHidden != nil && *in.IsHidden != hidden {
			patch.IsHidden, after["isHidden"], out.Hidden = lk.Some(*in.IsHidden), *in.IsHidden, *in.IsHidden
			changes = append(changes, map[bool]string{true: "hide it from players", false: "show it to players"}[*in.IsHidden])
		}
		if in.IsTitleHidden != nil && *in.IsTitleHidden != titleHidden {
			patch.IsTitleHidden, after["isTitleHidden"] = lk.Some(*in.IsTitleHidden), *in.IsTitleHidden
			changes = append(changes, map[bool]string{true: "hide its title", false: "show its title"}[*in.IsTitleHidden])
		}
		if len(changes) == 0 {
			return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Property %q (%s) already matches; nothing to change.", in.Title, env.ID)}, nil
		}
		visible, did = after, fmt.Sprintf("update %s property %q (%s): %s", p.Kind(), in.Title, env.ID, strings.Join(changes, ", "))
	}
	// The property as it will be is checked; its own "isHidden":true
	// exempts it, so showing a hidden property checks its whole content
	// (X-GUARD-05). Nothing on a hidden resource is checked (X-GUARD-04).
	if !r.IsHidden {
		if err := s.check(map[string]any{"property": visible}); err != nil {
			return zero, err
		}
	}
	if in.DryRun {
		return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Dry run: would %s on %q (%s). Nothing was written.", did, r.Name, r.ID)}, nil
	}
	var res blocks.Property
	if create != nil {
		res, err = c.Properties.Create(ctx, r.ID, create, lk.Place{})
	} else {
		res, err = c.Properties.Update(ctx, r.ID, out.PropertyID, patch)
	}
	if err != nil {
		return zero, err
	}
	if env := res.PropertyEnvelope(); env != nil {
		out.PropertyID = env.ID
		out.Hidden, _ = env.IsHidden.Get()
	}
	verb := "Updated"
	if create != nil {
		verb = "Created"
	}
	return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("%s %s property %q (%s) on %q (%s), %s.", verb, out.Kind, in.Title, out.PropertyID, r.Name, r.ID, hiddenWord(out.Hidden))}, nil
}

// DeletePropertyIn is delete_property's input.
type DeletePropertyIn struct {
	ProjectArg
	DryRunArg
	ResourceID string `json:"resource_id" jsonschema:"the resource whose sidebar property to delete"`
	Title      string `json:"title,omitempty" jsonschema:"the property's exact title (case-sensitive); not with property_id"`
	PropertyID string `json:"property_id,omitempty" jsonschema:"the property's id; not with title"`
}

func (s *server) deleteProperty(ctx context.Context, c *lk.Client, in DeletePropertyIn) (toolResult[PropertyOut], error) {
	var zero toolResult[PropertyOut]
	if (in.Title == "") == (in.PropertyID == "") {
		return zero, errors.New("give title or property_id (one of them)")
	}
	// No guard: removing a property can't show players anything new.
	list, err := c.Properties.List(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	var p blocks.Property
	if in.PropertyID != "" {
		for _, q := range list.Items {
			if env := q.PropertyEnvelope(); env != nil && env.ID == in.PropertyID {
				p = q
			}
		}
		if p == nil {
			return zero, fmt.Errorf("resource %s has no property %s", in.ResourceID, in.PropertyID)
		}
	} else {
		same := byTitle(list.Items, in.Title)
		switch len(same) {
		case 0:
			return zero, fmt.Errorf("resource %s has no property titled %q", in.ResourceID, in.Title)
		case 1:
			p = same[0]
		default:
			return zero, fmt.Errorf("resource %s has %d properties titled %q (%s); nothing was deleted. Hint: pass property_id", in.ResourceID, len(same), in.Title, propIDs(same))
		}
	}
	env := p.PropertyEnvelope()
	title, _ := env.Title.Get()
	hidden, _ := env.IsHidden.Get()
	out := PropertyOut{ResourceID: in.ResourceID, PropertyID: env.ID, Title: title, Kind: string(p.Kind()), Hidden: hidden, DryRun: in.DryRun}
	what := fmt.Sprintf("%s property %q (%s) of resource %s", p.Kind(), title, env.ID, in.ResourceID)
	if in.DryRun {
		return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := c.Properties.Delete(ctx, in.ResourceID, env.ID); err != nil {
		return zero, err
	}
	return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Deleted %s.", what)}, nil
}
