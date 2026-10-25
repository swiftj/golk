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
	PropertyTargetArg
	Title            string   `json:"title,omitempty" jsonschema:"the property's title; an existing property with exactly this title (case-sensitive) is updated, otherwise one is created. Optional when property_markdown or property_id names it"`
	PropertyID       string   `json:"property_id,omitempty" jsonschema:"update the property with this id instead of matching by title"`
	Markdown         *string  `json:"markdown,omitempty" jsonschema:"a text field's body as LK Markdown; not with rows or property_markdown"`
	Rows             *[]RowIn `json:"rows,omitempty" jsonschema:"key-value rows, replacing all rows; not with markdown or property_markdown"`
	PropertyMarkdown *string  `json:"property_markdown,omitempty" jsonschema:"the whole property, of any kind, as one LK Markdown property directive as get_resource shows it, e.g. ':::meter mode=bar title=Vitals ... :::', ':::links title=Allies ... :::', an image line, '::embed ...' or '::map ...'. Replaces the property's content; its kind can't change"`
	IsHidden         *bool    `json:"is_hidden,omitempty" jsonschema:"true hides the property from players; false shows it"`
	IsTitleHidden    *bool    `json:"is_title_hidden,omitempty" jsonschema:"true hides the title above the property"`
	Rank             *string  `json:"rank,omitempty" jsonschema:"an existing property's place: a rank string; properties sort by rank as plain strings"`
	After            string   `json:"after,omitempty" jsonschema:"place a new property right after this property id; not with before"`
	Before           string   `json:"before,omitempty" jsonschema:"place a new property right before this property id; not with after"`
}

// PropertyTargetArg picks where properties live: a resource's sidebar, or
// with document_id and block_id an infobox block on a page.
type PropertyTargetArg struct {
	ResourceID string `json:"resource_id" jsonschema:"the resource whose sidebar (or page) holds the property"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"with block_id: the page holding an infobox block, to set a property of that infobox instead of the sidebar"`
	BlockID    string `json:"block_id,omitempty" jsonschema:"with document_id: the infobox block (read_page with ids shows block ids)"`
}

// propTarget is a resolved PropertyTargetArg: the sidebar, or an infobox
// whose writes carry the page version read with the list (If-Match).
type propTarget struct {
	c       *lk.Client
	arg     PropertyTargetArg
	version string
}

func (a PropertyTargetArg) infobox() bool { return a.BlockID != "" }

func (a PropertyTargetArg) check() error {
	if a.ResourceID == "" {
		return errors.New("resource_id is required")
	}
	if (a.DocumentID == "") != (a.BlockID == "") {
		return errors.New("give document_id and block_id together (an infobox), or neither (the sidebar)")
	}
	return nil
}

func (a PropertyTargetArg) where() string {
	if a.infobox() {
		return fmt.Sprintf("infobox %s on page %s of resource %s", a.BlockID, a.DocumentID, a.ResourceID)
	}
	return "the sidebar of resource " + a.ResourceID
}

func (t *propTarget) doc() lk.DocRef {
	return lk.DocRef{ResourceID: t.arg.ResourceID, DocumentID: t.arg.DocumentID}
}

func (t *propTarget) ifMatch() []lk.CallOption {
	if t.version == "" {
		return nil
	}
	return []lk.CallOption{lk.IfMatch(t.version)}
}

func (t *propTarget) list(ctx context.Context) ([]blocks.Property, error) {
	if t.arg.infobox() {
		l, err := t.c.Blocks.ListProperties(ctx, t.doc(), t.arg.BlockID)
		if err != nil {
			return nil, err
		}
		t.version = l.Version
		return l.Items, nil
	}
	l, err := t.c.Properties.List(ctx, t.arg.ResourceID)
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

func (t *propTarget) create(ctx context.Context, p blocks.Property, place lk.Place) (blocks.Property, error) {
	if t.arg.infobox() {
		v, err := t.c.Blocks.CreateProperty(ctx, t.doc(), t.arg.BlockID, p, place, t.ifMatch()...)
		if err != nil {
			return nil, err
		}
		return v.Value, nil
	}
	return t.c.Properties.Create(ctx, t.arg.ResourceID, p, place)
}

func (t *propTarget) update(ctx context.Context, id string, patch lk.PropertyPatch) (blocks.Property, error) {
	if t.arg.infobox() {
		v, err := t.c.Blocks.UpdateProperty(ctx, t.doc(), t.arg.BlockID, id, patch, t.ifMatch()...)
		if err != nil {
			return nil, err
		}
		return v.Value, nil
	}
	return t.c.Properties.Update(ctx, t.arg.ResourceID, id, patch)
}

func (t *propTarget) delete(ctx context.Context, id string) error {
	if t.arg.infobox() {
		return t.c.Blocks.DeleteProperty(ctx, t.doc(), t.arg.BlockID, id, t.ifMatch()...)
	}
	return t.c.Properties.Delete(ctx, t.arg.ResourceID, id)
}

// visibleTo reports whether players can see the target's content at all:
// not when its resource or (for an infobox) its document is hidden.
func (t *propTarget) visible(r *lk.Resource) (bool, error) {
	if r.IsHidden {
		return false, nil
	}
	if t.arg.infobox() {
		d, err := findDoc(r, t.arg.DocumentID)
		if err != nil {
			return false, err
		}
		return !d.IsHidden, nil
	}
	return true, nil
}

// parseProperty parses one property directive (as get_resource renders
// properties inside an :::infobox).
func parseProperty(md string) (blocks.Property, error) {
	l, _, err := markdown.FromMarkdown(":::infobox\n"+strings.TrimRight(md, "\n")+"\n:::\n", markdown.FromOptions{})
	if err != nil {
		return nil, fmt.Errorf("property_markdown: %w", err)
	}
	if len(l) == 1 {
		if ib, ok := l[0].(*blocks.InfoboxBlock); ok && len(ib.Rows) == 1 {
			return ib.Rows[0], nil
		}
	}
	return nil, errors.New("property_markdown must hold exactly one property directive (e.g. ':::keyvalue title=Stats ... :::'), as get_resource shows them")
}

// contentPatch is the patch that gives an existing property p's content
// (not its envelope).
func contentPatch(p blocks.Property) (lk.PropertyPatch, error) {
	var u lk.PropertyPatch
	switch x := p.(type) {
	case *blocks.TextFieldProperty:
		u.Children = lk.Some(orEmpty(x.Children))
	case *blocks.KeyValueProperty:
		u.Rows, u.Display = lk.Some(x.Rows), x.Display
	case *blocks.ImageProperty:
		u.URL, u.Alt, u.Fill = lk.Some(x.URL), x.Alt, x.Fill
	case *blocks.MeterProperty:
		u.Mode, u.MeterItems = lk.Some(x.Mode), lk.Some(x.Items)
		u.ShowText, u.ShowIcon, u.ShowMax = x.ShowText, x.ShowIcon, x.ShowMax
	case *blocks.ResourceCollectionProperty:
		if x.Items != nil {
			u.ResourceItems = lk.Some(x.Items)
		}
		if x.Filter != nil {
			u.Filter = lk.Some(*x.Filter)
		}
	case *blocks.ResourceLinkProperty:
		u.ResourceItems = lk.Some(x.Items)
	case *blocks.EmbedProperty:
		u.Provider, u.URL = lk.Some(x.Provider), lk.Some(x.URL)
	case *blocks.MapProperty:
		u.ResourceID, u.DocumentID, u.DisplayHeight, u.Fill = x.ResourceID, x.DocumentID, x.DisplayHeight, x.Fill
		if x.Viewport != nil {
			u.Viewport = lk.Some(*x.Viewport)
		}
	default:
		return u, fmt.Errorf("a %s property can't be set with property_markdown", p.Kind())
	}
	return u, nil
}

func orEmpty(l blocks.List) blocks.List {
	if l == nil {
		return blocks.List{}
	}
	return l
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
	if err := in.check(); err != nil {
		return zero, err
	}
	given := 0
	for _, set := range []bool{in.Markdown != nil, in.Rows != nil, in.PropertyMarkdown != nil} {
		if set {
			given++
		}
	}
	if given > 1 {
		return zero, errors.New("give one of markdown (a text field), rows (key-value) or property_markdown (any kind)")
	}
	if in.After != "" && in.Before != "" {
		return zero, errors.New("give after or before, not both")
	}
	// The new content, as a whole property (parsed) or as parts.
	var parsed blocks.Property
	if in.PropertyMarkdown != nil {
		var err error
		if parsed, err = parseProperty(*in.PropertyMarkdown); err != nil {
			return zero, err
		}
	}
	title := in.Title
	if title == "" && parsed != nil {
		title, _ = propTitle(parsed)
	}
	if title == "" && in.PropertyID == "" {
		return zero, errors.New("title is required (or property_id, or a title in property_markdown)")
	}
	var children blocks.List
	if in.Markdown != nil {
		var err error
		if children, _, err = markdown.FromMarkdown(*in.Markdown, markdown.FromOptions{}); err != nil {
			return zero, err
		}
		children = orEmpty(children)
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
	t := &propTarget{c: c, arg: in.PropertyTargetArg}
	shown, err := t.visible(r)
	if err != nil {
		return zero, err
	}
	list, err := t.list(ctx)
	if err != nil {
		return zero, err
	}
	var same []blocks.Property
	if in.PropertyID != "" {
		for _, q := range list {
			if env := q.PropertyEnvelope(); env != nil && env.ID == in.PropertyID {
				same = append(same, q)
			}
		}
		if len(same) == 0 {
			return zero, fmt.Errorf("%s has no property %s", in.where(), in.PropertyID)
		}
	} else {
		same = byTitle(list, title)
	}
	if len(same) > 1 {
		return zero, fmt.Errorf("%s has %d properties titled %q (%s); nothing was written. Hint: pass property_id",
			in.where(), len(same), title, propIDs(same))
	}
	out := PropertyOut{ResourceID: r.ID, Title: title, DryRun: in.DryRun}
	var visible any
	var did string
	var create blocks.Property
	var patch lk.PropertyPatch
	if len(same) == 0 {
		switch {
		case parsed != nil:
			create = parsed
			env := create.PropertyEnvelope()
			if env == nil {
				return zero, errors.New("property_markdown holds a raw property, which can't be created")
			}
			env.ID, env.Rank = "", ""
			if in.Title != "" {
				env.Title = lk.Some(in.Title)
			}
		case in.Markdown != nil:
			create = &blocks.TextFieldProperty{Envelope: blocks.Envelope{Title: lk.Some(title)}, Children: children}
		case in.Rows != nil:
			create = &blocks.KeyValueProperty{Envelope: blocks.Envelope{Title: lk.Some(title)}, Rows: rows}
		default:
			return zero, fmt.Errorf("%s has no property titled %q; to create it give markdown (a text field), rows (key-value) or property_markdown (any kind)", in.where(), title)
		}
		env := create.PropertyEnvelope()
		if in.IsHidden != nil {
			env.IsHidden = lk.Some(*in.IsHidden)
		}
		if in.IsTitleHidden != nil {
			env.IsTitleHidden = lk.Some(*in.IsTitleHidden)
		}
		hidden, _ := env.IsHidden.Get()
		out.Kind, out.Hidden, out.Created = string(create.Kind()), hidden, true
		visible, did = create, fmt.Sprintf("create %s property %q", create.Kind(), title)
		if in.Rank != nil {
			return zero, errors.New("rank is for an existing property; place a new one with after or before")
		}
	} else {
		p := same[0]
		env := p.PropertyEnvelope()
		if env == nil {
			return zero, fmt.Errorf("property %s is a raw property this tool can't change", propIDs(same))
		}
		curTitle, _ := env.Title.Get()
		out.PropertyID, out.Kind, out.Title = env.ID, string(p.Kind()), curTitle
		hidden, _ := env.IsHidden.Get()
		titleHidden, _ := env.IsTitleHidden.Get()
		out.Hidden = hidden
		switch {
		case in.Markdown != nil && p.Kind() != blocks.KindTextField:
			return zero, fmt.Errorf("property %q (%s) is a %s property, not a text field; markdown can't change its kind. Hint: give property_markdown with its kind, or delete_property and set it again", curTitle, env.ID, p.Kind())
		case in.Rows != nil && p.Kind() != blocks.KindKeyValue:
			return zero, fmt.Errorf("property %q (%s) is a %s property, not key-value; rows can't change its kind. Hint: give property_markdown with its kind, or delete_property and set it again", curTitle, env.ID, p.Kind())
		case parsed != nil && parsed.Kind() != p.Kind():
			return zero, fmt.Errorf("property %q (%s) is a %s property, but property_markdown holds a %s property; a property's kind can't change. Hint: delete_property and set it again", curTitle, env.ID, p.Kind(), parsed.Kind())
		case in.After != "" || in.Before != "":
			return zero, errors.New("after and before place a new property; move an existing one with rank")
		}
		after, err := asJSONMap(p)
		if err != nil {
			return zero, err
		}
		var changes []string
		if parsed != nil {
			if patch, err = contentPatch(parsed); err != nil {
				return zero, err
			}
			whole, err := asJSONMap(parsed)
			if err != nil {
				return zero, err
			}
			for k, v := range whole {
				if k != "id" && k != "rank" && k != "kind" && k != "title" && k != "isHidden" && k != "isTitleHidden" {
					after[k] = v
				}
			}
			pe := parsed.PropertyEnvelope()
			if t, ok := pe.Title.Get(); ok && in.Title == "" && in.PropertyID != "" && t != curTitle {
				patch.Title, after["title"] = lk.Some(t), t
				changes = append(changes, fmt.Sprintf("rename it %q", t))
			}
			if in.IsHidden == nil {
				if h, ok := pe.IsHidden.Get(); ok && h != hidden {
					in.IsHidden = &h
				}
			}
			if in.IsTitleHidden == nil {
				if h, ok := pe.IsTitleHidden.Get(); ok && h != titleHidden {
					in.IsTitleHidden = &h
				}
			}
			changes = append(changes, "set its content")
		}
		if in.Title != "" && in.PropertyID != "" && in.Title != curTitle {
			patch.Title, after["title"], out.Title = lk.Some(in.Title), in.Title, in.Title
			changes = append(changes, fmt.Sprintf("rename it %q", in.Title))
		}
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
		if in.Rank != nil && *in.Rank != env.Rank {
			patch.Rank = lk.Some(*in.Rank)
			changes = append(changes, fmt.Sprintf("set its rank to %q", *in.Rank))
		}
		if len(changes) == 0 {
			return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Property %q (%s) already matches; nothing to change.", curTitle, env.ID)}, nil
		}
		visible, did = after, fmt.Sprintf("update %s property %q (%s): %s", p.Kind(), curTitle, env.ID, strings.Join(changes, ", "))
	}
	// The property as it will be is checked; its own "isHidden":true
	// exempts it, so showing a hidden property checks its whole content
	// (X-GUARD-05). Nothing on a hidden resource or page is checked
	// (X-GUARD-04).
	if shown {
		if err := s.check(map[string]any{"property": visible}); err != nil {
			return zero, err
		}
	}
	if in.DryRun {
		return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Dry run: would %s in %s. Nothing was written.", did, in.where())}, nil
	}
	var res blocks.Property
	if create != nil {
		res, err = t.create(ctx, create, lk.Place{After: in.After, Before: in.Before})
	} else {
		res, err = t.update(ctx, out.PropertyID, patch)
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
	return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("%s %s property %q (%s) in %s, %s.", verb, out.Kind, out.Title, out.PropertyID, in.where(), hiddenWord(out.Hidden))}, nil
}

// DeletePropertyIn is delete_property's input.
type DeletePropertyIn struct {
	ProjectArg
	DryRunArg
	PropertyTargetArg
	Title      string `json:"title,omitempty" jsonschema:"the property's exact title (case-sensitive); not with property_id"`
	PropertyID string `json:"property_id,omitempty" jsonschema:"the property's id; not with title"`
}

func (s *server) deleteProperty(ctx context.Context, c *lk.Client, in DeletePropertyIn) (toolResult[PropertyOut], error) {
	var zero toolResult[PropertyOut]
	if (in.Title == "") == (in.PropertyID == "") {
		return zero, errors.New("give title or property_id (one of them)")
	}
	if err := in.check(); err != nil {
		return zero, err
	}
	// No guard: removing a property can't show players anything new.
	t := &propTarget{c: c, arg: in.PropertyTargetArg}
	items, err := t.list(ctx)
	if err != nil {
		return zero, err
	}
	var p blocks.Property
	if in.PropertyID != "" {
		for _, q := range items {
			if env := q.PropertyEnvelope(); env != nil && env.ID == in.PropertyID {
				p = q
			}
		}
		if p == nil {
			return zero, fmt.Errorf("%s has no property %s", in.where(), in.PropertyID)
		}
	} else {
		same := byTitle(items, in.Title)
		switch len(same) {
		case 0:
			return zero, fmt.Errorf("%s has no property titled %q", in.where(), in.Title)
		case 1:
			p = same[0]
		default:
			return zero, fmt.Errorf("%s has %d properties titled %q (%s); nothing was deleted. Hint: pass property_id", in.where(), len(same), in.Title, propIDs(same))
		}
	}
	env := p.PropertyEnvelope()
	title, _ := env.Title.Get()
	hidden, _ := env.IsHidden.Get()
	out := PropertyOut{ResourceID: in.ResourceID, PropertyID: env.ID, Title: title, Kind: string(p.Kind()), Hidden: hidden, DryRun: in.DryRun}
	what := fmt.Sprintf("%s property %q (%s) in %s", p.Kind(), title, env.ID, in.where())
	if in.DryRun {
		return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := t.delete(ctx, env.ID); err != nil {
		return zero, err
	}
	return toolResult[PropertyOut]{out: out, text: fmt.Sprintf("Deleted %s.", what)}, nil
}
