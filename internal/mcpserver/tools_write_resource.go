package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

// ResourceOut describes a resource after a write (or, with dry_run, as it
// would be).
type ResourceOut struct {
	ResourceID string   `json:"resource_id,omitempty"`
	Name       string   `json:"name"`
	ParentID   string   `json:"parent_id,omitempty"`
	Hidden     bool     `json:"hidden"`
	Documents  []DocRef `json:"documents,omitempty"`
	DryRun     bool     `json:"dry_run,omitempty"`
}

// DocRef names one document in tool output.
type DocRef struct {
	DocumentID string `json:"document_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

func docRefs(ds []lk.Document) []DocRef {
	out := make([]DocRef, 0, len(ds))
	for _, d := range ds {
		out = append(out, DocRef{DocumentID: d.ID, Name: d.Name, Type: string(d.Type)})
	}
	return out
}

func hiddenWord(h bool) string {
	if h {
		return "hidden from players"
	}
	return "visible to players"
}

// visibleResource is everything players would see of r if it were
// visible: its name, aliases and tags, its image and banner URLs, the names and blocks of its
// documents that are not hidden, and its sidebar properties (hidden ones
// carry "isHidden":true, which the guard skips). Hidden documents are left
// out: players can't see them whatever the resource's flag. Each visible
// document's content is read by documentContent; a document of a kind it
// can't read is an error (uncheckable): showing it would show that
// content unchecked.
func visibleResource(ctx context.Context, c *lk.Client, r *lk.Resource) (map[string]any, error) {
	var docs []map[string]any
	for _, d := range r.Documents {
		if d.IsHidden {
			continue
		}
		doc, err := documentContent(ctx, c, r, &d)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	props, err := c.Properties.List(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	v := map[string]any{
		"name": r.Name, "aliases": r.Aliases, "tags": r.Tags, "imageUrl": r.ImageURL,
		"documents": docs, "properties": props.Items,
	}
	if r.Banner != nil {
		v["bannerUrl"] = r.Banner.URL
	}
	return v, nil
}

// hiddenFeaturesConfirmed makes hidden map features and timeline events
// DM-only. It stays false until the owner confirms in LegendKeeper's player
// view that hidden pins and events are invisible (DECISIONS.md D-014,
// X-GUARD-11); until then they are checked like public content.
const hiddenFeaturesConfirmed = false

// documentContent is what players see of document d of r, for the guard:
// its name, and a page's blocks, a map's image and features, or a
// timeline's lanes and events (its sources' too, which it shows), or a
// board's records (with nothing exempt, see strictContent). A kind it
// doesn't know is refused.
func documentContent(ctx context.Context, c *lk.Client, r *lk.Resource, d *lk.Document) (map[string]any, error) {
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	doc := map[string]any{"name": d.Name}
	switch d.Type {
	case lk.DocumentPage, lk.DocumentBlank:
		db, err := c.Documents.GetBlocks(ctx, ref)
		if err != nil {
			return nil, err
		}
		doc["blocks"] = db.Blocks
	case lk.DocumentMap:
		full, err := c.Documents.Get(ctx, ref)
		if err != nil {
			return nil, err
		}
		if full.MapImage != nil {
			doc["mapImage"] = full.MapImage.URL
		}
		fs, err := c.Features.List(ctx, ref, lk.FeatureListParams{})
		if err != nil {
			return nil, err
		}
		doc["features"] = shownFeatures(fs.Items)
	case lk.DocumentTime:
		lanes, err := c.Timelines.Lanes(ctx, ref)
		if err != nil {
			return nil, err
		}
		var events []lk.Event
		for e, err := range c.Timelines.AllEvents(ctx, ref, lk.EventListParams{Limit: 100}) {
			if err != nil {
				return nil, err
			}
			events = append(events, e)
		}
		doc["lanes"], doc["events"] = lanes.Items, shownEvents(events)
		// A custom calendar's names render on the timeline (red-team
		// review): showing the timeline shows them. getResource carries
		// calendarId; an id no calendar has renders as Gregorian.
		if d.CalendarID != "" && !lk.IsBuiltinCalendar(d.CalendarID) {
			for cal, err := range c.Calendars.All(ctx) {
				if err != nil {
					return nil, err
				}
				if cal.ID == d.CalendarID {
					doc["calendar"] = cal.Spec
					break
				}
			}
		}
	case lk.DocumentBoard:
		b, err := c.Boards.GetContent(ctx, ref)
		if err != nil {
			return nil, err
		}
		strict, err := strictContent(map[string]any{"pages": b.Pages, "shapes": b.Shapes, "bindings": b.Bindings, "assets": b.Assets})
		if err != nil {
			return nil, err
		}
		doc["board"] = strict
	default:
		return nil, uncheckable(r, d)
	}
	return doc, nil
}

// shownFeatures and shownEvents are the features and events players can
// see: all of them until hidden ones are confirmed DM-only.
func shownFeatures(fs []lk.Feature) []lk.Feature {
	if !hiddenFeaturesConfirmed {
		return fs
	}
	var out []lk.Feature
	for _, f := range fs {
		if !f.IsHidden {
			out = append(out, f)
		}
	}
	return out
}

func shownEvents(es []lk.Event) []lk.Event {
	if !hiddenFeaturesConfirmed {
		return es
	}
	var out []lk.Event
	for _, e := range es {
		if !e.IsHidden {
			out = append(out, e)
		}
	}
	return out
}

// uncheckable refuses to show document d of r: its kind is one
// documentContent can't read, so the guard can't check it.
func uncheckable(r *lk.Resource, d *lk.Document) error {
	return fmt.Errorf("document %q (%s) of %q (%s) is a %s document, whose content the spoiler guard can't read, so lk-mcp won't show it to players. "+
		"Hint: review it in LegendKeeper and show it there, or hide that document first (update_document is_hidden=true)", d.Name, d.ID, r.Name, r.ID, d.Type)
}

// CreateResourceIn is create_resource's input.
type CreateResourceIn struct {
	ProjectArg
	DryRunArg
	ResourceSpec
}

// ResourceSpec is one resource to create (create_resource, and each item of
// create_resources).
type ResourceSpec struct {
	Name         string   `json:"name" jsonschema:"the resource's name, as players see it in the sidebar"`
	ParentID     string   `json:"parent_id,omitempty" jsonschema:"the parent resource id; omit to create it at the top level"`
	Tags         []string `json:"tags,omitempty" jsonschema:"tags to set"`
	Aliases      []string `json:"aliases,omitempty" jsonschema:"other names that search finds it by"`
	IsHidden     *bool    `json:"is_hidden,omitempty" jsonschema:"true creates it hidden from players (DM-only)"`
	PageMarkdown *string  `json:"page_markdown,omitempty" jsonschema:"the first page's content as LK Markdown (as read_page writes it); omit for an empty page"`
	PageName     string   `json:"page_name,omitempty" jsonschema:"the first page's name; default Main"`
	// Documents creates 1-5 documents of any type instead of one page.
	Documents []NewDocArg `json:"documents,omitempty" jsonschema:"1-5 documents (tabs) to create, in order, instead of page_markdown/page_name"`
	ID        string      `json:"id,omitempty" jsonschema:"a client-chosen cuid2 id (8 or 24 characters); omit to let the server choose"`
	PositionArgs
	IconArgs
	IsLocked *bool      `json:"is_locked,omitempty" jsonschema:"true creates it locked against edits in the app"`
	Banner   *BannerArg `json:"banner,omitempty" jsonschema:"a banner image (an image asset URL of the project)"`
}

// NewDocArg is a document of a new resource.
type NewDocArg struct {
	Name       string  `json:"name,omitempty" jsonschema:"the document's name; default Main"`
	Type       string  `json:"type,omitempty" jsonschema:"page, map, time (timeline), board or blank; default page with markdown, otherwise blank"`
	CalendarID string  `json:"calendar_id,omitempty" jsonschema:"a time document's calendar (list_calendars)"`
	Markdown   *string `json:"markdown,omitempty" jsonschema:"a page document's content as LK Markdown"`
}

// builtResource is a ResourceSpec ready to send: the create body, what
// players would see of it, and its block count.
type builtResource struct {
	rc      lk.ResourceCreate
	visible map[string]any
	hidden  bool
	nblocks int
}

func buildResource(in ResourceSpec) (builtResource, error) {
	var zero builtResource
	if strings.TrimSpace(in.Name) == "" {
		return zero, errors.New("name is required")
	}
	rc := lk.ResourceCreate{Name: in.Name}
	if in.ParentID != "" {
		rc.ParentID = lk.Some(in.ParentID)
	}
	if in.Tags != nil {
		rc.Tags = lk.Some(in.Tags)
	}
	if in.Aliases != nil {
		rc.Aliases = lk.Some(in.Aliases)
	}
	hidden := in.IsHidden != nil && *in.IsHidden
	if in.IsHidden != nil {
		rc.IsHidden = lk.Some(*in.IsHidden)
	}
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.Documents != nil && (in.PageMarkdown != nil || in.PageName != "") {
		return zero, errors.New("give documents, or page_markdown/page_name, not both")
	}
	rc.ID, rc.After, rc.Before = optStr(in.ID), optStr(in.After), optStr(in.Before)
	rc.IconGlyph, rc.IconColor = optPtr(in.IconGlyph), optPtr(in.IconColor)
	if in.IconShape != nil {
		rc.IconShape = lk.Some(lk.IconShape(*in.IconShape))
	}
	if in.IconSize != nil {
		rc.IconSize = lk.Some(lk.IconSize(*in.IconSize))
	}
	rc.IsLocked = optPtr(in.IsLocked)
	visible := map[string]any{"name": in.Name, "aliases": in.Aliases, "tags": in.Tags}
	if in.Banner != nil {
		b := in.Banner.merge(nil)
		rc.Banner = lk.Some(b)
		visible["bannerUrl"] = b.URL
	}
	nblocks := 0
	if in.Documents != nil {
		if len(in.Documents) < 1 || len(in.Documents) > 5 {
			return zero, errors.New("documents takes 1 to 5 documents")
		}
		var docs []lk.ResourceDocument
		var shown []map[string]any
		for i, d := range in.Documents {
			doc := lk.ResourceDocument{Name: optStr(d.Name), CalendarID: optStr(d.CalendarID)}
			if d.Type != "" {
				doc.Type = lk.Some(lk.DocumentType(d.Type))
			}
			v := map[string]any{"name": d.Name}
			if d.Markdown != nil {
				content, _, err := markdown.FromMarkdown(*d.Markdown, markdown.FromOptions{})
				if err != nil {
					return zero, fmt.Errorf("documents[%d].markdown: %w", i, err)
				}
				doc.Blocks = content
				nblocks += len(content)
				v["blocks"] = content
			}
			docs = append(docs, doc)
			shown = append(shown, v)
		}
		rc.Documents = lk.Some(docs)
		visible["documents"] = shown
	}
	if in.PageMarkdown != nil || in.PageName != "" {
		doc := lk.ResourceDocument{Type: lk.Some(lk.DocumentPage)}
		if in.PageName != "" {
			doc.Name = lk.Some(in.PageName)
			visible["page_name"] = in.PageName
		}
		if in.PageMarkdown != nil {
			content, _, err := markdown.FromMarkdown(*in.PageMarkdown, markdown.FromOptions{})
			if err != nil {
				return zero, err
			}
			doc.Blocks, nblocks = content, len(content)
			visible["blocks"] = content
		}
		rc.Documents = lk.Some([]lk.ResourceDocument{doc})
	}
	return builtResource{rc: rc, visible: visible, hidden: hidden, nblocks: nblocks}, nil
}

func (s *server) createResource(ctx context.Context, c *lk.Client, in CreateResourceIn) (toolResult[ResourceOut], error) {
	var zero toolResult[ResourceOut]
	b, err := buildResource(in.ResourceSpec)
	if err != nil {
		return zero, err
	}
	rc, hidden, nblocks := b.rc, b.hidden, b.nblocks
	// A resource created hidden shows players nothing (X-GUARD-04).
	if !hidden {
		if err := s.check(b.visible); err != nil {
			return zero, err
		}
	}
	out := ResourceOut{Name: in.Name, ParentID: in.ParentID, Hidden: hidden, DryRun: in.DryRun}
	where := "at the top level"
	if in.ParentID != "" {
		where = "under " + in.ParentID
	}
	if in.set() {
		where += ", " + strings.TrimPrefix(in.text(), "place it ")
	}
	if in.DryRun {
		return toolResult[ResourceOut]{out: out, text: fmt.Sprintf("Dry run: would create resource %q %s, %s, with a page of %d block(s). Nothing was written.",
			in.Name, where, hiddenWord(hidden), nblocks)}, nil
	}
	r, err := c.Resources.Create(ctx, rc)
	if err != nil {
		return zero, err
	}
	out.ResourceID, out.Name, out.ParentID, out.Hidden, out.Documents = r.ID, r.Name, r.ParentID, r.IsHidden, docRefs(r.Documents)
	return toolResult[ResourceOut]{out: out, text: fmt.Sprintf("Created resource %q (%s) %s, %s; documents: %s.",
		r.Name, r.ID, where, hiddenWord(r.IsHidden), docList(r.Documents))}, nil
}

func docList(ds []lk.Document) string {
	if len(ds) == 0 {
		return "none"
	}
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = fmt.Sprintf("%q (%s, %s)", d.Name, d.ID, d.Type)
	}
	return strings.Join(parts, ", ")
}

// UpdateResourceIn is update_resource's input.
type UpdateResourceIn struct {
	ProjectArg
	DryRunArg
	ID         string    `json:"id" jsonschema:"the resource id"`
	Name       *string   `json:"name,omitempty" jsonschema:"the new name"`
	Tags       *[]string `json:"tags,omitempty" jsonschema:"the new tags; replaces the whole list ([] clears it)"`
	Aliases    *[]string `json:"aliases,omitempty" jsonschema:"the new aliases; replaces the whole list ([] clears it)"`
	IsHidden   *bool     `json:"is_hidden,omitempty" jsonschema:"true hides the resource from players; false shows it (its whole content is checked by the spoiler guard first)"`
	ParentID   *string   `json:"parent_id,omitempty" jsonschema:"move the resource (with its children) under this resource"`
	MoveToRoot bool      `json:"move_to_root,omitempty" jsonschema:"move the resource (with its children) to the top level; not with parent_id"`
	PositionArgs
	IconArgs
	IsLocked    *bool          `json:"is_locked,omitempty" jsonschema:"true locks the resource against edits in the app"`
	Banner      *BannerArg     `json:"banner,omitempty" jsonschema:"the banner image; fields left out keep their value"`
	Permissions map[string]int `json:"permissions,omitempty" jsonschema:"access levels keyed by role (MEMBER, ADMIN, OWNER) or user id: 0 no access, 1 can read, 2 can edit, 3 full access, 4 defer to the role, 5 inherit from the parent; entries left out keep their value"`
}

// PositionArgs place a resource among its siblings (moveResource,
// createResource and the template copies take after or before).
type PositionArgs struct {
	After  string `json:"after,omitempty" jsonschema:"place it right after this sibling resource id (sidebar order); not with before"`
	Before string `json:"before,omitempty" jsonschema:"place it right before this sibling resource id (sidebar order); not with after"`
}

func (p PositionArgs) check() error {
	if p.After != "" && p.Before != "" {
		return errors.New("give after or before, not both")
	}
	return nil
}

func (p PositionArgs) set() bool { return p.After != "" || p.Before != "" }

func (p PositionArgs) text() string {
	if p.After != "" {
		return "place it after " + p.After
	}
	return "place it before " + p.Before
}

// IconArgs are a resource's icon: a glyph, its colour, and the map-pin
// shape and size drawn behind it.
type IconArgs struct {
	IconGlyph *string `json:"icon_glyph,omitempty" jsonschema:"the icon glyph name; empty clears it"`
	IconColor *string `json:"icon_color,omitempty" jsonschema:"the icon colour, e.g. #aa3300"`
	IconShape *string `json:"icon_shape,omitempty" jsonschema:"pin, diamond, flag, marker, shield, circle, banner, arch, or none for the glyph alone"`
	IconSize  *string `json:"icon_size,omitempty" jsonschema:"medium or small"`
}

func (a IconArgs) count() int {
	n := 0
	for _, v := range []*string{a.IconGlyph, a.IconColor, a.IconShape, a.IconSize} {
		if v != nil {
			n++
		}
	}
	return n
}

// BannerArg is a resource's banner. URL must be an image asset URL of the
// project (upload_image).
type BannerArg struct {
	Enabled   *bool    `json:"enabled,omitempty" jsonschema:"show or hide the banner"`
	URL       *string  `json:"url,omitempty" jsonschema:"the image asset URL"`
	YPosition *float64 `json:"y_position,omitempty" jsonschema:"the vertical focus, 0 (top) to 1 (bottom)"`
}

// merge lays b over cur (nil for none).
func (b *BannerArg) merge(cur *lk.Banner) lk.Banner {
	out := lk.Banner{Enabled: true, YPosition: 0.5}
	if cur != nil {
		out = *cur
	}
	if b.Enabled != nil {
		out.Enabled = *b.Enabled
	}
	if b.URL != nil {
		out.URL = *b.URL
	}
	if b.YPosition != nil {
		out.YPosition = *b.YPosition
	}
	return out
}

// raisesMemberAccess reports whether perms gives the MEMBER role (players)
// any access other than none: like unhiding, that can show content, so the
// guard checks it (D-021).
// optPtr is *v as an Optional: unset when v is nil.
func optPtr[T any](v *T) lk.Optional[T] {
	if v == nil {
		return lk.Optional[T]{}
	}
	return lk.Some(*v)
}

// optStr is v as an Optional: unset when empty.
func optStr(v string) lk.Optional[string] {
	if v == "" {
		return lk.Optional[string]{}
	}
	return lk.Some(v)
}

func raisesMemberAccess(perms map[string]int) bool {
	v, ok := perms["MEMBER"]
	return ok && v != int(lk.PermissionNoAccess)
}

func (s *server) updateResource(ctx context.Context, c *lk.Client, in UpdateResourceIn) (toolResult[ResourceOut], error) {
	var zero toolResult[ResourceOut]
	if in.ParentID != nil && in.MoveToRoot {
		return zero, errors.New("give parent_id or move_to_root, not both")
	}
	if err := in.check(); err != nil {
		return zero, err
	}
	for k, v := range in.Permissions {
		if v < 0 || v > 5 {
			return zero, fmt.Errorf("permissions[%s] = %d: a level is 0 to 5", k, v)
		}
	}
	if in.Name == nil && in.Tags == nil && in.Aliases == nil && in.IsHidden == nil && in.ParentID == nil && !in.MoveToRoot &&
		!in.set() && in.count() == 0 && in.IsLocked == nil && in.Banner == nil && in.Permissions == nil {
		return zero, errors.New("nothing to change: give a field to set (name, tags, aliases, is_hidden, parent_id, move_to_root, after, before, icon_*, is_locked, banner or permissions)")
	}
	r, err := c.Resources.Get(ctx, in.ID)
	if err != nil {
		return zero, err
	}
	out := ResourceOut{ResourceID: r.ID, Name: r.Name, ParentID: r.ParentID, Hidden: r.IsHidden, DryRun: in.DryRun}
	var u lk.ResourceUpdate
	var did []string
	changed := map[string]any{}
	after := *r
	if in.Name != nil && *in.Name != r.Name {
		u.Name, out.Name, after.Name = lk.Some(*in.Name), *in.Name, *in.Name
		changed["name"] = *in.Name
		did = append(did, fmt.Sprintf("rename %q to %q", r.Name, *in.Name))
	}
	if in.Tags != nil && !slices.Equal(*in.Tags, r.Tags) {
		u.Tags, after.Tags = lk.Some(*in.Tags), *in.Tags
		changed["tags"] = *in.Tags
		did = append(did, fmt.Sprintf("set tags to [%s]", strings.Join(*in.Tags, ", ")))
	}
	if in.Aliases != nil && !slices.Equal(*in.Aliases, r.Aliases) {
		u.Aliases, after.Aliases = lk.Some(*in.Aliases), *in.Aliases
		changed["aliases"] = *in.Aliases
		did = append(did, fmt.Sprintf("set aliases to [%s]", strings.Join(*in.Aliases, ", ")))
	}
	unhide := false
	if in.IsHidden != nil && *in.IsHidden != r.IsHidden {
		u.IsHidden, out.Hidden = lk.Some(*in.IsHidden), *in.IsHidden
		unhide = !*in.IsHidden
		did = append(did, map[bool]string{true: "hide it from players", false: "show it to players"}[*in.IsHidden])
	}
	setStr := func(dst *lk.Optional[string], v *string, cur, what string) {
		if v != nil && *v != cur {
			*dst = lk.Some(*v)
			changed[what] = *v
			did = append(did, fmt.Sprintf("set %s to %q", what, *v))
		}
	}
	setStr(&u.IconGlyph, in.IconGlyph, r.IconGlyph, "iconGlyph")
	setStr(&u.IconColor, in.IconColor, r.IconColor, "iconColor")
	if in.IconShape != nil && *in.IconShape != string(r.IconShape) {
		u.IconShape = lk.Some(lk.IconShape(*in.IconShape))
		did = append(did, fmt.Sprintf("set the icon shape to %s", *in.IconShape))
	}
	if in.IconSize != nil && *in.IconSize != string(r.IconSize) {
		u.IconSize = lk.Some(lk.IconSize(*in.IconSize))
		did = append(did, fmt.Sprintf("set the icon size to %s", *in.IconSize))
	}
	if in.IsLocked != nil && *in.IsLocked != r.IsLocked {
		u.IsLocked = lk.Some(*in.IsLocked)
		did = append(did, map[bool]string{true: "lock it", false: "unlock it"}[*in.IsLocked])
	}
	if in.Banner != nil {
		b := in.Banner.merge(r.Banner)
		u.Banner = lk.Some(b)
		changed["bannerUrl"] = b.URL
		did = append(did, "set its banner")
	}
	if in.Permissions != nil {
		p := make(map[string]lk.PermissionLevel, len(in.Permissions))
		for k, v := range in.Permissions {
			p[k] = lk.PermissionLevel(v)
		}
		u.Permissions = lk.Some(p)
		did = append(did, "set its permissions")
	}
	var mv lk.ResourceMove
	move := false
	if in.set() {
		mv.After, mv.Before = optStr(in.After), optStr(in.Before)
		move = true
		did = append(did, in.text())
	}
	switch {
	case in.MoveToRoot && r.ParentID != "":
		mv.ParentID, out.ParentID, move = lk.Null[string](), "", true
		did = append(did, "move it to the top level")
	case in.ParentID != nil && *in.ParentID != r.ParentID:
		if *in.ParentID == "" {
			return zero, errors.New("parent_id is empty: use move_to_root to move to the top level")
		}
		mv.ParentID, out.ParentID, move = lk.Value(*in.ParentID), *in.ParentID, true
		did = append(did, "move it under "+*in.ParentID)
	}
	if len(did) == 0 {
		return toolResult[ResourceOut]{out: out, text: fmt.Sprintf("Resource %q (%s) already matches; nothing to change.", r.Name, r.ID)}, nil
	}
	// The guard checks what players would see after the change: nothing
	// while the resource stays hidden (X-GUARD-04); everything it shows
	// when it is being unhidden (X-GUARD-05); otherwise the new name,
	// tags and aliases. Moving changes nobody's view: hidden is not
	// inherited (D-012).
	if !out.Hidden && !s.cfg.GuardOff {
		visible := changed
		if unhide || raisesMemberAccess(in.Permissions) {
			if visible, err = visibleResource(ctx, c, &after); err != nil {
				return zero, err
			}
		}
		if len(visible) > 0 {
			if err := s.check(visible); err != nil {
				return zero, err
			}
		}
	}
	summary := strings.Join(did, " and ")
	if in.DryRun {
		return toolResult[ResourceOut]{out: out, text: fmt.Sprintf("Dry run: would %s (resource %q, %s). Nothing was written.", summary, r.Name, r.ID)}, nil
	}
	cur := r
	upd := len(changed) > 0 || u.IsHidden.IsSet() || u.IconShape.IsSet() || u.IconSize.IsSet() ||
		u.IsLocked.IsSet() || u.Banner.IsSet() || u.Permissions.IsSet()
	if upd {
		if cur, err = c.Resources.Update(ctx, r.ID, u); err != nil {
			return zero, err
		}
	}
	if move {
		if cur, err = c.Resources.Move(ctx, r.ID, mv); err != nil {
			if upd {
				return zero, fmt.Errorf("the other changes were saved, but the move failed: %w", err)
			}
			return zero, err
		}
	}
	out.Name, out.ParentID, out.Hidden = cur.Name, cur.ParentID, cur.IsHidden
	return toolResult[ResourceOut]{out: out, text: fmt.Sprintf("Did %s: resource %q (%s), %s.", summary, cur.Name, cur.ID, hiddenWord(cur.IsHidden))}, nil
}

// DeleteResourceIn is delete_resource's input.
type DeleteResourceIn struct {
	ProjectArg
	DryRunArg
	ID      string `json:"id" jsonschema:"the resource id"`
	Cascade bool   `json:"cascade,omitempty" jsonschema:"also delete its child resources and their descendants; required when it has children"`
}

// DeleteResourceOut is delete_resource's output.
type DeleteResourceOut struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Documents  int    `json:"documents"`
	Children   int    `json:"children"`
	// MoreChildren is set when there are more children than Children.
	MoreChildren bool `json:"more_children,omitempty"`
	Cascade      bool `json:"cascade,omitempty"`
	DryRun       bool `json:"dry_run,omitempty"`
}

func (s *server) deleteResource(ctx context.Context, c *lk.Client, in DeleteResourceIn) (toolResult[DeleteResourceOut], error) {
	var zero toolResult[DeleteResourceOut]
	// No guard: deleting takes content away from players, it can't show
	// them anything.
	r, err := c.Resources.Get(ctx, in.ID)
	if err != nil {
		return zero, err
	}
	kids, err := c.Resources.List(ctx, lk.ResourceListParams{ParentID: r.ID, Limit: 100})
	if err != nil {
		return zero, err
	}
	out := DeleteResourceOut{ResourceID: r.ID, Name: r.Name, Documents: len(r.Documents), Children: len(kids.Items),
		MoreChildren: kids.HasMore, Cascade: in.Cascade, DryRun: in.DryRun}
	children := fmt.Sprintf("%d child resource(s)", out.Children)
	if out.MoreChildren {
		children = "at least " + children
	}
	if out.Children > 0 && !in.Cascade {
		return zero, fmt.Errorf("resource %q (%s) has %s; nothing was deleted. Hint: pass cascade to delete them (and their descendants) too, or move them first", r.Name, r.ID, children)
	}
	what := fmt.Sprintf("resource %q (%s) with %d document(s) and %s", r.Name, r.ID, out.Documents, children)
	if out.Children > 0 {
		what += " and their descendants"
	}
	if in.DryRun {
		return toolResult[DeleteResourceOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := c.Resources.Delete(ctx, r.ID, lk.ResourceDeleteParams{Cascade: in.Cascade}); err != nil {
		return zero, err
	}
	return toolResult[DeleteResourceOut]{out: out, text: fmt.Sprintf("Deleted %s. restore_resource with id %s brings it back.", what, r.ID)}, nil
}

// RestoreResourceIn is restore_resource's input.
type RestoreResourceIn struct {
	ProjectArg
	DryRunArg
	ID string `json:"id" jsonschema:"the id of the deleted resource"`
}

// RestoreResourceOut is restore_resource's output.
type RestoreResourceOut struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name,omitempty"`
	Hidden     bool   `json:"hidden"`
	// Checked is the number of restored-tree resources the guard scanned.
	Checked int  `json:"checked,omitempty"`
	DryRun  bool `json:"dry_run,omitempty"`
}

// restoreResource restores a deleted resource. LegendKeeper answers 404
// for a deleted resource, so its content can't be checked before the
// restore. Instead the guard must be configured (or explicitly off) before
// anything is sent, and after the restore every visible resource of the
// restored tree is hidden first, then checked, and shown again only when
// the guard passes it. Restored content is visible to players only between
// the restore and the hide of each resource (one request for the restored
// resource itself), never while the guard reads it.
func (s *server) restoreResource(ctx context.Context, c *lk.Client, in RestoreResourceIn) (toolResult[RestoreResourceOut], error) {
	var zero toolResult[RestoreResourceOut]
	out := RestoreResourceOut{ResourceID: in.ID, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[RestoreResourceOut]{out: out, text: fmt.Sprintf("Dry run: would restore resource %s and the descendants deleted with it. "+
			"A deleted resource can't be read, so after the restore every visible resource of the restored tree is hidden at once, checked by the spoiler guard, and shown again only if it passes. Nothing was written.", in.ID)}, nil
	}
	r, err := c.Resources.Restore(ctx, in.ID)
	if err != nil {
		return zero, err
	}
	out.Name, out.Hidden = r.Name, r.IsHidden
	if s.cfg.GuardOff {
		return toolResult[RestoreResourceOut]{out: out, text: fmt.Sprintf("Restored resource %q (%s), %s.", r.Name, r.ID, hiddenWord(r.IsHidden))}, nil
	}
	visible, err := s.hideRestored(ctx, c, r)
	if err != nil {
		return zero, err
	}
	var held []string
	for _, v := range visible {
		out.Checked++
		reason := s.recheck(ctx, c, v.id)
		if reason == nil {
			if _, uerr := c.Resources.Update(ctx, v.id, lk.ResourceUpdate{IsHidden: lk.Some(false)}); uerr == nil {
				continue
			} else {
				reason = fmt.Errorf("showing it again failed: %w", explain(uerr))
			}
		}
		if v.id == r.ID {
			out.Hidden = true
		}
		held = append(held, fmt.Sprintf("%q (%s): %v", v.name, v.id, reason))
	}
	if len(held) > 0 {
		return zero, fmt.Errorf("restored resource %q (%s), but these stay hidden from players:\n- %s\nHint: fix the content (or move it into :::secret blocks), then show it with update_resource is_hidden=false",
			r.Name, r.ID, strings.Join(held, "\n- "))
	}
	return toolResult[RestoreResourceOut]{out: out, text: fmt.Sprintf("Restored resource %q (%s), %s; the spoiler guard checked %d visible resource(s) of the restored tree, hidden while it did.",
		r.Name, r.ID, hiddenWord(out.Hidden), out.Checked)}, nil
}

// hideRestored hides the restored resource r and its live descendants that
// are visible, and returns them. Hidden ones are left alone, but not their
// children (hidden is not inherited). On an error the resources found so
// far are hidden, and the error says the rest may be visible.
func (s *server) hideRestored(ctx context.Context, c *lk.Client, r *lk.Resource) ([]flaggedResource, error) {
	var visible []flaggedResource
	hide := func(id, name string) error {
		if _, err := c.Resources.Update(ctx, id, lk.ResourceUpdate{IsHidden: lk.Some(true)}); err != nil {
			return fmt.Errorf("restored resource %q (%s), but hiding %q (%s) for the spoiler guard check FAILED, so it may be visible to players now: %w. Hint: hide it with update_resource is_hidden=true at once",
				r.Name, r.ID, name, id, explain(err))
		}
		visible = append(visible, flaggedResource{id: id, name: name})
		return nil
	}
	if !r.IsHidden {
		if err := hide(r.ID, r.Name); err != nil {
			return nil, err
		}
	}
	queue := []string{r.ID}
	seen := map[string]bool{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for kid, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: cur, Limit: 100}) {
			if err != nil {
				return nil, fmt.Errorf("restored resource %q (%s), but listing its descendants failed, so some may be visible to players and unchecked: %w. Hint: list them and hide anything that must not be shown",
					r.Name, r.ID, explain(err))
			}
			if !kid.IsHidden {
				if err := hide(kid.ID, kid.Name); err != nil {
					return nil, err
				}
			}
			queue = append(queue, kid.ID)
		}
	}
	return visible, nil
}

// recheck runs the guard over resource id as players would see it once
// shown again. It returns why the resource must stay hidden, or nil.
func (s *server) recheck(ctx context.Context, c *lk.Client, id string) error {
	r, err := c.Resources.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("could not read it to check: %w", explain(err))
	}
	v, err := visibleResource(ctx, c, r)
	if err != nil {
		return fmt.Errorf("could not read its content to check: %w", explain(err))
	}
	return s.check(v)
}

type flaggedResource struct {
	id, name string
}

// maxBatchCreate is the most resources one createManyResources call takes.
const maxBatchCreate = 100

// CreateResourcesIn is create_resources' input.
type CreateResourcesIn struct {
	ProjectArg
	DryRunArg
	Resources []ResourceSpec `json:"resources" jsonschema:"1-100 resources to create in order, each with the fields create_resource takes; the batch counts as one write"`
}

// CreateResourcesOut is create_resources' output.
type CreateResourcesOut struct {
	Resources []ResourceOut `json:"resources"`
	DryRun    bool          `json:"dry_run,omitempty"`
}

func (s *server) createResources(ctx context.Context, c *lk.Client, in CreateResourcesIn) (toolResult[CreateResourcesOut], error) {
	var zero toolResult[CreateResourcesOut]
	if len(in.Resources) == 0 || len(in.Resources) > maxBatchCreate {
		return zero, fmt.Errorf("resources takes 1 to %d resources", maxBatchCreate)
	}
	rcs := make([]lk.ResourceCreate, len(in.Resources))
	out := CreateResourcesOut{Resources: []ResourceOut{}, DryRun: in.DryRun}
	for i, spec := range in.Resources {
		b, err := buildResource(spec)
		if err != nil {
			return zero, fmt.Errorf("resources[%d]: %w", i, err)
		}
		// Each resource is checked by its own hidden flag (X-GUARD-04).
		if !b.hidden {
			if err := s.check(map[string]any{"resources": map[string]any{fmt.Sprint(i): b.visible}}); err != nil {
				return zero, err
			}
		}
		rcs[i] = b.rc
		out.Resources = append(out.Resources, ResourceOut{Name: spec.Name, ParentID: spec.ParentID, Hidden: b.hidden})
	}
	if in.DryRun {
		return toolResult[CreateResourcesOut]{out: out, text: fmt.Sprintf("Dry run: would create %d resource(s) in one batch: %s. Nothing was written.",
			len(rcs), specNames(in.Resources))}, nil
	}
	rs, err := c.Resources.CreateMany(ctx, rcs)
	if err != nil {
		return zero, err
	}
	out.Resources = out.Resources[:0]
	var lines []string
	for _, r := range rs {
		out.Resources = append(out.Resources, ResourceOut{ResourceID: r.ID, Name: r.Name, ParentID: r.ParentID, Hidden: r.IsHidden, Documents: docRefs(r.Documents)})
		lines = append(lines, fmt.Sprintf("- %q (%s), %s", r.Name, r.ID, hiddenWord(r.IsHidden)))
	}
	return toolResult[CreateResourcesOut]{out: out, text: fmt.Sprintf("Created %d resource(s):\n%s", len(rs), strings.Join(lines, "\n"))}, nil
}

func specNames(specs []ResourceSpec) string {
	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = fmt.Sprintf("%q", sp.Name)
	}
	return strings.Join(names, ", ")
}
