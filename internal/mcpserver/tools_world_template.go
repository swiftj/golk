package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addTemplateTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name: "list_templates",
		Description: "List the project's resource templates one page at a time, at any depth: id, name, parent, hidden flag and documents. " +
			"A top-level template has parent templates. When next_cursor is returned, call again with it for the next page.",
		Annotations: readOnly("List templates"),
	}, func(in ListTemplatesIn) string { return in.Project }, s.listTemplates)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "save_as_template",
		Description: "Copy a resource (with include_children, its whole subtree) into a new top-level template. The resource doesn't change. " +
			"Templates count as player-visible: every copied resource that isn't hidden, and the template's name, is checked by the spoiler guard first. " +
			"Each copy keeps its hidden flag.",
		Annotations: write("Save as template", false, false),
	}, func(in SaveAsTemplateIn) string { return in.Project }, serial(s, s.saveAsTemplate))
	addTool(s, ms, &mcp.Tool{
		Name: "create_from_template",
		Description: "Create a resource from a template (with include_children, its whole subtree), with its fields, properties and documents. " +
			"Every copied resource that isn't hidden, and the new name, is checked by the spoiler guard first. Each copy keeps its hidden flag.",
		Annotations: write("Create from template", false, false),
	}, func(in CreateFromTemplateIn) string { return in.Project }, serial(s, s.createFromTemplate))
}

// ListTemplatesIn is list_templates' input.
type ListTemplatesIn struct {
	ProjectArg
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, 1-100; omit for the server default"`
}

// ListTemplatesOut is list_templates' output.
type ListTemplatesOut struct {
	Templates  []ResourceItemOut `json:"templates"`
	HasMore    bool              `json:"has_more,omitempty"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (s *server) listTemplates(ctx context.Context, c *lk.Client, in ListTemplatesIn) (toolResult[ListTemplatesOut], error) {
	var zero toolResult[ListTemplatesOut]
	if err := checkPageLimit(in.Limit); err != nil {
		return zero, err
	}
	p, err := c.Templates.List(ctx, lk.TemplateListParams{Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return zero, err
	}
	out := ListTemplatesOut{Templates: []ResourceItemOut{}, HasMore: p.HasMore, NextCursor: p.NextCursor}
	var b strings.Builder
	for _, r := range p.Items {
		it := ResourceItemOut{ID: r.ID, Name: r.Name, ParentID: r.ParentID, Tags: r.Tags, Hidden: r.IsHidden,
			HasChildren: r.HasChildren, Documents: []DocSummaryOut{}}
		fmt.Fprintf(&b, "- %s (%s)%s", r.Name, r.ID, listFlags(flagIf(r.IsHidden, "hidden"), flagIf(r.HasChildren, "has children")))
		if r.ParentID != "" && r.ParentID != lk.ParentTemplates {
			fmt.Fprintf(&b, " in template %s;", r.ParentID)
		}
		var docs []string
		for _, d := range r.Documents {
			it.Documents = append(it.Documents, DocSummaryOut{ID: d.ID, Name: d.Name, Type: string(d.Type)})
			docs = append(docs, fmt.Sprintf("%s (%s, %s)", d.Name, d.Type, d.ID))
		}
		if len(docs) > 0 {
			fmt.Fprintf(&b, " docs: %s", strings.Join(docs, ", "))
		}
		b.WriteString("\n")
		out.Templates = append(out.Templates, it)
	}
	pagedFooter(&b, len(out.Templates), "template(s)", p.NextCursor, p.HasMore)
	return toolResult[ListTemplatesOut]{out: out, text: b.String()}, nil
}

// TemplateCopyOut describes a template copy after a write (or, with
// dry_run, as it would be).
type TemplateCopyOut struct {
	ResourceID string `json:"resource_id,omitempty" jsonschema:"the new template or resource"`
	Name       string `json:"name"`
	ParentID   string `json:"parent_id,omitempty"`
	Hidden     bool   `json:"hidden"`
	// Copied is the number of resources copied (the root and, with
	// include_children, its descendants); Checked of them are visible and
	// were checked by the guard.
	Copied  int  `json:"copied"`
	Checked int  `json:"checked"`
	DryRun  bool `json:"dry_run,omitempty"`
}

// treeNode is one resource of a copied tree.
type treeNode struct {
	id, name string
	hidden   bool
}

// checkCopy runs the guard over every resource a template copy copies:
// root and, with children, the descendants children lists. Each is checked
// by its own hidden flag (A-35: a copy keeps it); a hidden one is skipped,
// but its children are still visited (hidden is not inherited, D-012). The
// root is checked under name, the name its copy gets. It returns how many
// were copied and how many checked.
func (s *server) checkCopy(ctx context.Context, c *lk.Client, root *lk.Resource, name string, descendants []treeNode) (copied, checked int, err error) {
	check := func(r *lk.Resource, as string) error {
		if r.IsHidden || s.cfg.GuardOff {
			return nil
		}
		v, err := visibleResource(ctx, c, r)
		if err != nil {
			return err
		}
		v["name"] = as
		checked++
		if err := s.check(v); err != nil {
			return fmt.Errorf("resource %q (%s) of the copy: %w", r.Name, r.ID, err)
		}
		return nil
	}
	if err := check(root, name); err != nil {
		return 0, 0, err
	}
	copied = 1
	for _, n := range descendants {
		copied++
		if n.hidden {
			continue
		}
		r, err := c.Resources.Get(ctx, n.id)
		if err != nil {
			return 0, 0, err
		}
		if err := check(r, r.Name); err != nil {
			return 0, 0, err
		}
	}
	return copied, checked, nil
}

// subtree lists every descendant of id, breadth first, with kids giving
// one resource's direct children.
func subtree(id string, kids func(string) ([]treeNode, error)) ([]treeNode, error) {
	var out []treeNode
	queue := []string{id}
	seen := map[string]bool{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		ks, err := kids(cur)
		if err != nil {
			return nil, err
		}
		for _, k := range ks {
			out = append(out, k)
			queue = append(queue, k.id)
		}
	}
	return out, nil
}

// SaveAsTemplateIn is save_as_template's input.
type SaveAsTemplateIn struct {
	ProjectArg
	DryRunArg
	ResourceID      string  `json:"resource_id" jsonschema:"the resource to copy into a template"`
	Name            *string `json:"name,omitempty" jsonschema:"the template's name; omit for the resource's name"`
	IncludeChildren *bool   `json:"include_children,omitempty" jsonschema:"also copy its descendants, keeping their tree (at most 100 resources)"`
}

func (s *server) saveAsTemplate(ctx context.Context, c *lk.Client, in SaveAsTemplateIn) (toolResult[TemplateCopyOut], error) {
	var zero toolResult[TemplateCopyOut]
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return zero, errors.New("name is empty: omit it for the resource's name")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	name := r.Name
	if in.Name != nil {
		name = *in.Name
	}
	children := in.IncludeChildren != nil && *in.IncludeChildren
	var desc []treeNode
	if children {
		if desc, err = subtree(r.ID, func(id string) ([]treeNode, error) {
			var out []treeNode
			for k, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: id, Limit: 100}) {
				if err != nil {
					return nil, err
				}
				out = append(out, treeNode{id: k.ID, name: k.Name, hidden: k.IsHidden})
			}
			return out, nil
		}); err != nil {
			return zero, err
		}
	}
	// Templates count as player-visible (D-014), so everything copied is
	// checked by its own flag, and the template's name always.
	if err := s.check(map[string]any{"templateName": name}); err != nil {
		return zero, err
	}
	copied, checked, err := s.checkCopy(ctx, c, r, name, desc)
	if err != nil {
		return zero, err
	}
	out := TemplateCopyOut{Name: name, ParentID: lk.ParentTemplates, Hidden: r.IsHidden, Copied: copied, Checked: checked, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[TemplateCopyOut]{out: out, text: fmt.Sprintf("Dry run: would save %q (%s) as template %q, copying %d resource(s), %d of them visible and checked by the spoiler guard. Nothing was written.",
			r.Name, r.ID, name, copied, checked)}, nil
	}
	body := lk.SaveAsTemplate{}
	setOpt(&body.Name, in.Name)
	setOpt(&body.IncludeChildren, in.IncludeChildren)
	t, err := c.Resources.SaveAsTemplate(ctx, r.ID, body)
	if err != nil {
		return zero, err
	}
	out.ResourceID, out.Name, out.ParentID, out.Hidden = t.ID, t.Name, t.ParentID, t.IsHidden
	return toolResult[TemplateCopyOut]{out: out, text: fmt.Sprintf("Saved %q (%s) as template %q (%s), %s, copying %d resource(s); %d checked by the spoiler guard.",
		r.Name, r.ID, t.Name, t.ID, hiddenWord(t.IsHidden), copied, checked)}, nil
}

// CreateFromTemplateIn is create_from_template's input.
type CreateFromTemplateIn struct {
	ProjectArg
	DryRunArg
	TemplateID      string  `json:"template_id" jsonschema:"the template (from list_templates)"`
	Name            *string `json:"name,omitempty" jsonschema:"the new resource's name; omit for the template's name without the word Template"`
	ParentID        *string `json:"parent_id,omitempty" jsonschema:"create it under this resource; omit for the top level"`
	IncludeChildren *bool   `json:"include_children,omitempty" jsonschema:"also copy the template's descendants, keeping their tree"`
}

func (s *server) createFromTemplate(ctx context.Context, c *lk.Client, in CreateFromTemplateIn) (toolResult[TemplateCopyOut], error) {
	var zero toolResult[TemplateCopyOut]
	if in.TemplateID == "" {
		return zero, errors.New("template_id is required")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return zero, errors.New("name is empty: omit it for the template's name")
	}
	if in.ParentID != nil && *in.ParentID == "" {
		return zero, errors.New("parent_id is empty: omit it to create the resource at the top level")
	}
	t, err := c.Resources.Get(ctx, in.TemplateID)
	if err != nil {
		return zero, err
	}
	// Without a name the server names the copy after the template, less
	// the word "Template"; the template's name covers every word of it.
	name := t.Name
	if in.Name != nil {
		name = *in.Name
	}
	children := in.IncludeChildren != nil && *in.IncludeChildren
	var desc []treeNode
	if children {
		// Templates.List lists templates at any depth, each with its parent
		// template's id.
		byParent := map[string][]treeNode{}
		for it, err := range c.Templates.All(ctx, lk.TemplateListParams{Limit: 100}) {
			if err != nil {
				return zero, err
			}
			byParent[it.ParentID] = append(byParent[it.ParentID], treeNode{id: it.ID, name: it.Name, hidden: it.IsHidden})
		}
		if desc, err = subtree(t.ID, func(id string) ([]treeNode, error) { return byParent[id], nil }); err != nil {
			return zero, err
		}
	}
	if err := s.check(map[string]any{"name": name}); err != nil {
		return zero, err
	}
	copied, checked, err := s.checkCopy(ctx, c, t, name, desc)
	if err != nil {
		return zero, err
	}
	out := TemplateCopyOut{Name: name, Hidden: t.IsHidden, Copied: copied, Checked: checked, DryRun: in.DryRun}
	where := "at the top level"
	if in.ParentID != nil {
		out.ParentID, where = *in.ParentID, "under "+*in.ParentID
	}
	if in.DryRun {
		return toolResult[TemplateCopyOut]{out: out, text: fmt.Sprintf("Dry run: would create %q %s from template %q (%s), copying %d resource(s), %d of them visible and checked by the spoiler guard. Nothing was written.",
			name, where, t.Name, t.ID, copied, checked)}, nil
	}
	body := lk.FromTemplate{TemplateID: t.ID}
	setOpt(&body.Name, in.Name)
	setOpt(&body.ParentID, in.ParentID)
	setOpt(&body.IncludeChildren, in.IncludeChildren)
	nr, err := c.Resources.CreateFromTemplate(ctx, body)
	if err != nil {
		return zero, err
	}
	out.ResourceID, out.Name, out.ParentID, out.Hidden = nr.ID, nr.Name, nr.ParentID, nr.IsHidden
	return toolResult[TemplateCopyOut]{out: out, text: fmt.Sprintf("Created %q (%s) %s from template %q (%s), %s, copying %d resource(s); %d checked by the spoiler guard.",
		nr.Name, nr.ID, where, t.Name, t.ID, hiddenWord(nr.IsHidden), copied, checked)}, nil
}
