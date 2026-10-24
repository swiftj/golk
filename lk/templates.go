package lk

import (
	"context"
	"iter"
	"net/url"
	"strconv"

	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// TemplatesService lists the project's resource templates. A template is a
// resource under the virtual root ParentTemplates; read, update, move and
// delete templates with the Resources operations.
type TemplatesService struct{ c *Client }

// TemplateListParams pages Templates.List.
type TemplateListParams struct {
	// Limit is the page size (1–100). Zero uses the server default.
	Limit int
	// Cursor starts at the page a previous Page.NextCursor names, with the
	// same Limit. Empty starts at the first page.
	Cursor string
}

// List returns one page of the project's templates at any depth: the
// first, or the one p.Cursor names. A top-level template has ParentID
// ParentTemplates; a nested one has its parent template's ID.
func (s *TemplatesService) List(ctx context.Context, p TemplateListParams, opts ...CallOption) (*Page[ResourceListItem], error) {
	q := url.Values{}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	pg := &pager[gen.ResourceList, ResourceListItem]{
		c: s.c, tmpl: gen.OpListTemplates.Path, query: q, opts: opts,
		items: func(w gen.ResourceList) ([]ResourceListItem, opt.Optional[bool], opt.Nullable[string]) {
			out := make([]ResourceListItem, len(w.Data))
			for i, r := range w.Data {
				out[i] = listItemFromWire(r)
			}
			return out, w.HasMore, w.NextCursor
		},
	}
	return pg.page(ctx, p.Cursor)
}

// All iterates over every template, fetching pages as it goes.
func (s *TemplatesService) All(ctx context.Context, p TemplateListParams, opts ...CallOption) iter.Seq2[ResourceListItem, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[ResourceListItem], error) { return s.List(ctx, p, opts...) })
}

// SaveAsTemplate is the body of Resources.SaveAsTemplate. Fields left unset
// are not sent.
type SaveAsTemplate struct {
	// Name defaults to the resource's name.
	Name Optional[string]
	// IncludeChildren also copies the descendants the caller can view,
	// keeping their tree. A copy holds at most 100 resources.
	IncludeChildren Optional[bool]
	// After or Before places the template next to a sibling template.
	After  Optional[string]
	Before Optional[string]
	// ID is a client-chosen ID for the template (see NewID).
	ID Optional[string]
}

// SaveAsTemplate copies the resource into a new top-level template and
// returns the template. The resource does not change. Each copied resource
// keeps its hidden flag (DECISIONS.md D-014, A-35). It needs the admin role.
func (s *ResourcesService) SaveAsTemplate(ctx context.Context, id string, t SaveAsTemplate, opts ...CallOption) (*Resource, error) {
	body := gen.ResourceSaveAsTemplateParams{
		Name: t.Name, IncludeChildren: t.IncludeChildren, After: t.After, Before: t.Before, ID: t.ID,
	}
	return s.created(ctx, gen.OpSaveResourceAsTemplate, map[string]string{"resourceId": id}, body, opts)
}

// FromTemplate is the body of Resources.CreateFromTemplate. TemplateID is
// required; fields left unset are not sent.
type FromTemplate struct {
	// TemplateID names a template at any depth. A resource that is not a
	// template fails with ErrValidation.
	TemplateID string
	// Name defaults to the template's name without the word "Template".
	Name Optional[string]
	// ParentID places the copy under a resource; unset creates it at the
	// root.
	ParentID        Optional[string]
	IncludeChildren Optional[bool]
	After           Optional[string]
	Before          Optional[string]
	// ID is a client-chosen ID for the new resource (see NewID).
	ID Optional[string]
}

// CreateFromTemplate copies a template into a new resource and returns it.
// The copy has the template's fields, properties and documents with their
// content, under new IDs; each copied resource keeps its hidden flag.
func (s *ResourcesService) CreateFromTemplate(ctx context.Context, t FromTemplate, opts ...CallOption) (*Resource, error) {
	body := gen.ResourceFromTemplateParams{
		TemplateID: t.TemplateID, Name: t.Name, ParentID: t.ParentID, IncludeChildren: t.IncludeChildren,
		After: t.After, Before: t.Before, ID: t.ID,
	}
	return s.created(ctx, gen.OpCreateResourceFromTemplate, nil, body, opts)
}

func (s *ResourcesService) created(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Resource, error) {
	var w gen.Resource
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	r := resourceFromWire(w)
	return &r, nil
}
