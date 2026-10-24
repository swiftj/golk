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

// guardReady refuses up front when no guard is configured, for a write
// whose content can only be checked after it is sent (restore_resource).
func (s *server) guardReady() error {
	if s.cfg.GuardOff || s.guard != nil {
		return nil
	}
	return errNoGuard
}

// visibleResource is everything players would see of r if it were
// visible: its name, aliases and tags, the names and blocks of its
// documents that are not hidden, and its sidebar properties (hidden ones
// carry "isHidden":true, which the guard skips). Hidden documents are left
// out: players can't see them whatever the resource's flag. Only page and
// blank documents have blocks the SDK can read; a visible document of any
// other kind (map, board, timeline) holds content the guard can't check,
// so it is an error (errUncheckable): showing it would show that content
// unchecked.
func visibleResource(ctx context.Context, c *lk.Client, r *lk.Resource) (map[string]any, error) {
	var docs []map[string]any
	for _, d := range r.Documents {
		if d.IsHidden {
			continue
		}
		if err := uncheckable(r, &d); err != nil {
			return nil, err
		}
		doc := map[string]any{"name": d.Name}
		if d.Type == lk.DocumentPage || d.Type == lk.DocumentBlank {
			db, err := c.Documents.GetBlocks(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID})
			if err != nil {
				return nil, err
			}
			doc["blocks"] = db.Blocks
		}
		docs = append(docs, doc)
	}
	props, err := c.Properties.List(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name": r.Name, "aliases": r.Aliases, "tags": r.Tags,
		"documents": docs, "properties": props.Items,
	}, nil
}

// uncheckable refuses to show document d of r when the guard can't read
// its content: only page and blank documents have blocks lk can read.
func uncheckable(r *lk.Resource, d *lk.Document) error {
	if d.Type == lk.DocumentPage || d.Type == lk.DocumentBlank {
		return nil
	}
	return fmt.Errorf("document %q (%s) of %q (%s) is a %s document, whose content the spoiler guard can't read, so lk-mcp won't show it to players. "+
		"Hint: review it in LegendKeeper and show it there, or hide that document first (update_document is_hidden=true)", d.Name, d.ID, r.Name, r.ID, d.Type)
}

// CreateResourceIn is create_resource's input.
type CreateResourceIn struct {
	ProjectArg
	DryRunArg
	Name         string   `json:"name" jsonschema:"the resource's name, as players see it in the sidebar"`
	ParentID     string   `json:"parent_id,omitempty" jsonschema:"the parent resource id; omit to create it at the top level"`
	Tags         []string `json:"tags,omitempty" jsonschema:"tags to set"`
	Aliases      []string `json:"aliases,omitempty" jsonschema:"other names that search finds it by"`
	IsHidden     *bool    `json:"is_hidden,omitempty" jsonschema:"true creates it hidden from players (DM-only)"`
	PageMarkdown *string  `json:"page_markdown,omitempty" jsonschema:"the first page's content as LK Markdown (as read_page writes it); omit for an empty page"`
	PageName     string   `json:"page_name,omitempty" jsonschema:"the first page's name; default Main"`
}

func (s *server) createResource(ctx context.Context, c *lk.Client, in CreateResourceIn) (toolResult[ResourceOut], error) {
	var zero toolResult[ResourceOut]
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
	visible := map[string]any{"name": in.Name, "aliases": in.Aliases, "tags": in.Tags}
	nblocks := 0
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
	// A resource created hidden shows players nothing (X-GUARD-04).
	if !hidden {
		if err := s.check(visible); err != nil {
			return zero, err
		}
	}
	out := ResourceOut{Name: in.Name, ParentID: in.ParentID, Hidden: hidden, DryRun: in.DryRun}
	where := "at the top level"
	if in.ParentID != "" {
		where = "under " + in.ParentID
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
}

func (s *server) updateResource(ctx context.Context, c *lk.Client, in UpdateResourceIn) (toolResult[ResourceOut], error) {
	var zero toolResult[ResourceOut]
	if in.ParentID != nil && in.MoveToRoot {
		return zero, errors.New("give parent_id or move_to_root, not both")
	}
	if in.Name == nil && in.Tags == nil && in.Aliases == nil && in.IsHidden == nil && in.ParentID == nil && !in.MoveToRoot {
		return zero, errors.New("nothing to change: give name, tags, aliases, is_hidden, parent_id or move_to_root")
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
	var mv lk.ResourceMove
	move := false
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
		if unhide {
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
	upd := len(did) > 0 && (len(changed) > 0 || in.IsHidden != nil && *in.IsHidden != r.IsHidden)
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
	if err := s.guardReady(); err != nil {
		return zero, err
	}
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
