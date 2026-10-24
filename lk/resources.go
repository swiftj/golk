package lk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// Sentinel parent IDs for ResourceListParams.ParentID.
const (
	// ParentRoot lists the top-level resources.
	ParentRoot = "root"
	// ParentTemplates lists the top-level templates.
	ParentTemplates = "templates"
)

// IconShape is the shape drawn behind a resource's glyph on a map pin.
type IconShape string

// Icon shapes.
const (
	IconShapePin     IconShape = "pin"
	IconShapeDiamond IconShape = "diamond"
	IconShapeFlag    IconShape = "flag"
	IconShapeMarker  IconShape = "marker"
	IconShapeShield  IconShape = "shield"
	IconShapeCircle  IconShape = "circle"
	IconShapeBanner  IconShape = "banner"
	IconShapeArch    IconShape = "arch"
	IconShapeNone    IconShape = "none"
)

// IconSize is the size of a map pin's shape.
type IconSize string

// Icon sizes.
const (
	IconSizeMedium IconSize = "medium"
	IconSizeSmall  IconSize = "small"
)

// DocumentType is the kind of a resource's document. A blank document takes
// a type on its first typed write.
type DocumentType string

// Document types.
const (
	DocumentBlank DocumentType = "blank"
	DocumentPage  DocumentType = "page"
	DocumentBoard DocumentType = "board"
	DocumentMap   DocumentType = "map"
	DocumentTime  DocumentType = "time"
)

// PermissionLevel is an access level in Resource.Permissions, keyed by a role
// (MEMBER, ADMIN or OWNER) or a user ID.
type PermissionLevel int

// Permission levels.
const (
	PermissionNoAccess    PermissionLevel = 0
	PermissionCanRead     PermissionLevel = 1
	PermissionCanEdit     PermissionLevel = 2
	PermissionFullAccess  PermissionLevel = 3
	PermissionDeferToRole PermissionLevel = 4
	PermissionInherit     PermissionLevel = 5
)

// Banner is a resource's banner image. URL must be an image asset URL of the
// same project.
type Banner struct {
	Enabled   bool
	URL       string
	YPosition float64
}

// Document is a document of a resource, without its content.
type Document struct {
	ID          string
	Name        string
	Type        DocumentType
	Rank        string
	IsHidden    bool
	IsFullWidth bool
	// CalendarID is set on a timeline (time) document.
	CalendarID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  time.Time
}

// DocumentSummary is a document as resource list items carry it.
type DocumentSummary struct {
	ID   string
	Name string
	Type DocumentType
}

// ResourceAncestor is one ancestor of a resource, root first.
type ResourceAncestor struct {
	ID   string
	Name string
}

// Resource is an entry in the project tree, such as a character, a place or
// a faction.
type Resource struct {
	ID string
	// ParentID is empty for a top-level resource.
	ParentID string
	Name     string
	Tags     []string
	// Aliases are alternative names that search matches.
	Aliases []string
	// IconGlyph is a Font Awesome icon name, such as "crown".
	IconGlyph string
	// IconColor is #RRGGBB.
	IconColor string
	IconShape IconShape
	IconSize  IconSize
	// Rank orders siblings. Compare ranks as plain strings (SortByRank).
	Rank            string
	IsHidden        bool
	IsLocked        bool
	IsFullWidth     bool
	ShowPropertyBar bool
	ImageURL        string
	// Banner is nil when the resource has none.
	Banner      *Banner
	Permissions map[string]PermissionLevel
	Documents   []Document
	// Properties are the sidebar properties, sorted by rank.
	Properties []blocks.Property
	// Ancestors is filled by Resources.Get only.
	Ancestors []ResourceAncestor
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ResourceListItem is a resource as Resources.List returns it.
type ResourceListItem struct {
	ID          string
	ParentID    string
	Name        string
	Tags        []string
	Aliases     []string
	IconGlyph   string
	IconColor   string
	IconShape   IconShape
	IconSize    IconSize
	Rank        string
	IsHidden    bool
	HasChildren bool
	Documents   []DocumentSummary
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ResourcesService manages resources.
type ResourcesService struct{ c *Client }

// ResourceListParams filters Resources.List.
type ResourceListParams struct {
	// ParentID lists the direct children of one resource. ParentRoot and
	// ParentTemplates list the top-level resources or templates. Empty lists
	// the whole project.
	ParentID string
	// Limit is the page size (1–100). Zero uses the server default (50).
	Limit int
	// Cursor starts at the page a previous Page.NextCursor names, with the
	// same ParentID and Limit. Empty starts at the first page.
	Cursor string
}

// List returns one page of resources: the first, or the one Cursor names.
func (s *ResourcesService) List(ctx context.Context, p ResourceListParams, opts ...CallOption) (*Page[ResourceListItem], error) {
	return s.pager(p, opts).page(ctx, p.Cursor)
}

// All iterates over every resource that p selects, fetching pages as it goes.
func (s *ResourcesService) All(ctx context.Context, p ResourceListParams, opts ...CallOption) iter.Seq2[ResourceListItem, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[ResourceListItem], error) { return s.List(ctx, p, opts...) })
}

func (s *ResourcesService) pager(p ResourceListParams, opts []CallOption) *pager[gen.ResourceList, ResourceListItem] {
	q := url.Values{}
	if p.ParentID != "" {
		q.Set("parentId", p.ParentID)
	}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	return &pager[gen.ResourceList, ResourceListItem]{
		c: s.c, tmpl: gen.OpListResources.Path, query: q, opts: opts,
		items: func(w gen.ResourceList) ([]ResourceListItem, opt.Optional[bool], opt.Nullable[string]) {
			out := make([]ResourceListItem, len(w.Data))
			for i, r := range w.Data {
				out[i] = listItemFromWire(r)
			}
			return out, w.HasMore, w.NextCursor
		},
	}
}

// Get returns the resource with its documents, sidebar properties and ancestors.
func (s *ResourcesService) Get(ctx context.Context, id string, opts ...CallOption) (*Resource, error) {
	var w gen.ResourceWithAncestors
	if err := s.c.call(ctx, gen.OpGetResource.Method, gen.OpGetResource.Path,
		map[string]string{"resourceId": id}, request{out: &w}, opts); err != nil {
		return nil, err
	}
	r := resourceFromWire(gen.Resource{
		Aliases: w.Aliases, CreatedAt: w.CreatedAt, Documents: w.Documents, IconColor: w.IconColor,
		IconGlyph: w.IconGlyph, IconShape: convOpt[gen.ResourceIconShape](w.IconShape),
		IconSize: convOpt[gen.ResourceIconSize](w.IconSize), ID: w.ID, ImageURL: w.ImageURL,
		IsFullWidth: w.IsFullWidth, IsHidden: w.IsHidden, IsLocked: w.IsLocked, Name: w.Name,
		ParentID: w.ParentID, Permissions: w.Permissions, Properties: w.Properties, Rank: w.Rank,
		ShowPropertyBar: w.ShowPropertyBar, Tags: w.Tags, UpdatedAt: w.UpdatedAt,
	})
	if b, ok := w.Banner.Get(); ok {
		r.Banner = &Banner{Enabled: b.Enabled, URL: b.URL, YPosition: b.YPosition}
	}
	for _, a := range w.Ancestors {
		r.Ancestors = append(r.Ancestors, ResourceAncestor{ID: a.ID, Name: a.Name})
	}
	return &r, nil
}

// ResourceCreate is the body of Resources.Create. Only Name is required;
// fields left unset are not sent.
type ResourceCreate struct {
	// ID is a client-chosen ID (see NewID). An ID already used in the
	// project, even by a deleted resource, fails with ErrDuplicateID.
	ID Optional[string]
	// Name is 1 or more characters.
	Name string
	// ParentID places the resource under another; unset creates it at the root.
	ParentID Optional[string]
	// After or Before places it next to a sibling; unset places it last.
	After     Optional[string]
	Before    Optional[string]
	Tags      Optional[[]string]
	Aliases   Optional[[]string]
	IconGlyph Optional[string]
	IconColor Optional[string]
	IconShape Optional[IconShape]
	IconSize  Optional[IconSize]
	IsHidden  Optional[bool]
	IsLocked  Optional[bool]
	Banner    Optional[Banner]
	// Documents creates up to 5 documents, in order. Unset gives the resource
	// one blank document named "Main".
	Documents Optional[[]ResourceDocument]
}

// ResourceDocument is one document to create with a resource.
type ResourceDocument struct {
	// Type defaults to page when Blocks is set, otherwise to blank.
	Type Optional[DocumentType]
	// Name defaults to "Main".
	Name Optional[string]
	// CalendarID sets a timeline document's calendar.
	CalendarID Optional[string]
	// Blocks is a page document's content, validated as
	// Documents.ReplaceBlocks validates it. Nil sends none. The created
	// resource is returned without content.
	Blocks blocks.List
}

func (r ResourceCreate) wire() (gen.ResourceCreateParams, error) {
	w := gen.ResourceCreateParams{
		ID: r.ID, Name: r.Name, ParentID: r.ParentID, After: r.After, Before: r.Before,
		Tags: r.Tags, Aliases: r.Aliases, IconGlyph: r.IconGlyph, IconColor: r.IconColor,
		IconShape: convOpt[gen.ResourceCreateParamsIconShape](r.IconShape),
		IconSize:  convOpt[gen.ResourceCreateParamsIconSize](r.IconSize),
		IsHidden:  r.IsHidden, IsLocked: r.IsLocked,
	}
	if b, ok := r.Banner.Get(); ok {
		w.Banner = opt.Some(gen.ResourceCreateParamsBanner{Enabled: b.Enabled, URL: b.URL, YPosition: b.YPosition})
	}
	if docs, ok := r.Documents.Get(); ok {
		wd := make([]gen.ResourceDocumentParams, len(docs))
		for i, d := range docs {
			wd[i] = gen.ResourceDocumentParams{
				Type: convOpt[gen.ResourceDocumentParamsType](d.Type), Name: d.Name, CalendarID: d.CalendarID,
			}
			if d.Blocks != nil {
				bs, err := rawBlocks(d.Blocks)
				if err != nil {
					return w, fmt.Errorf("lk: documents[%d].blocks: %w", i, err)
				}
				wd[i].Blocks = opt.Some(bs)
			}
		}
		w.Documents = opt.Some(wd)
	}
	return w, nil
}

// rawBlocks encodes blocks for a generated wire field.
func rawBlocks(l blocks.List) ([]gen.Block, error) {
	out := make([]gen.Block, len(l))
	for i, b := range l {
		raw, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		out[i] = gen.Block{Kind: string(b.Kind()), Raw: raw}
	}
	return out, nil
}

// Create creates a resource and returns it.
func (s *ResourcesService) Create(ctx context.Context, r ResourceCreate, opts ...CallOption) (*Resource, error) {
	body, err := r.wire()
	if err != nil {
		return nil, err
	}
	var w gen.Resource
	if err := s.c.call(ctx, gen.OpCreateResource.Method, gen.OpCreateResource.Path, nil,
		request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	res := resourceFromWire(w)
	return &res, nil
}

// CreateMany creates 1–100 resources in one call, which counts as one write,
// and returns them in order. The server checks every item first: if one
// fails, nothing is created and errors[].path starts with resources[i]. An
// item can name an earlier item's ID as its ParentID. Give every item an ID,
// so a retry after a partial failure answers ErrDuplicateID instead of
// creating twice.
func (s *ResourcesService) CreateMany(ctx context.Context, rs []ResourceCreate, opts ...CallOption) ([]Resource, error) {
	body := gen.ResourceBatchCreateParams{Resources: make([]gen.ResourceCreateParams, len(rs))}
	for i, r := range rs {
		w, err := r.wire()
		if err != nil {
			return nil, fmt.Errorf("resources[%d]: %w", i, err)
		}
		body.Resources[i] = w
	}
	var w gen.ResourceBatch
	if err := s.c.call(ctx, gen.OpCreateManyResources.Method, gen.OpCreateManyResources.Path, nil,
		request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Resource, len(w.Resources))
	for i, r := range w.Resources {
		out[i] = resourceFromWire(r)
	}
	return out, nil
}

// ResourceUpdate is the body of Resources.Update. Only the fields that are
// set are sent and changed. Tags and Aliases replace the whole list.
type ResourceUpdate struct {
	Name      Optional[string]
	Tags      Optional[[]string]
	Aliases   Optional[[]string]
	IconGlyph Optional[string]
	IconColor Optional[string]
	IconShape Optional[IconShape]
	IconSize  Optional[IconSize]
	// IsHidden can be changed by project admins only.
	IsHidden Optional[bool]
	IsLocked Optional[bool]
	Banner   Optional[Banner]
	// Permissions entries not sent keep their value.
	Permissions Optional[map[string]PermissionLevel]
}

func (u ResourceUpdate) wire() gen.ResourceUpdateParams {
	w := gen.ResourceUpdateParams{
		Name: u.Name, Tags: u.Tags, Aliases: u.Aliases, IconGlyph: u.IconGlyph, IconColor: u.IconColor,
		IconShape: convOpt[gen.ResourceUpdateParamsIconShape](u.IconShape),
		IconSize:  convOpt[gen.ResourceUpdateParamsIconSize](u.IconSize),
		IsHidden:  u.IsHidden, IsLocked: u.IsLocked,
	}
	if b, ok := u.Banner.Get(); ok {
		w.Banner = opt.Some(gen.ResourceUpdateParamsBanner{Enabled: b.Enabled, URL: b.URL, YPosition: b.YPosition})
	}
	if p, ok := u.Permissions.Get(); ok {
		m := make(map[string]int64, len(p))
		for k, v := range p {
			m[k] = int64(v)
		}
		w.Permissions = opt.Some(m)
	}
	return w
}

// Update changes only the fields set in u and returns the resource.
func (s *ResourcesService) Update(ctx context.Context, id string, u ResourceUpdate, opts ...CallOption) (*Resource, error) {
	var w gen.Resource
	if err := s.c.call(ctx, gen.OpUpdateResource.Method, gen.OpUpdateResource.Path,
		map[string]string{"resourceId": id}, request{body: u.wire(), out: &w}, opts); err != nil {
		return nil, err
	}
	res := resourceFromWire(w)
	return &res, nil
}

// ResourceMove is the body of Resources.Move. Set any of the fields.
type ResourceMove struct {
	// ParentID is the new parent, or Null for the root. Unset keeps the parent.
	ParentID Nullable[string]
	// After or Before places the resource next to a sibling under the
	// destination parent. Setting both fails with ErrValidation.
	After  Optional[string]
	Before Optional[string]
}

// Move moves the resource and its children and returns the resource.
func (s *ResourcesService) Move(ctx context.Context, id string, m ResourceMove, opts ...CallOption) (*Resource, error) {
	body := gen.ResourceMoveParams{ParentID: m.ParentID, After: m.After, Before: m.Before}
	var w gen.Resource
	if err := s.c.call(ctx, gen.OpMoveResource.Method, gen.OpMoveResource.Path,
		map[string]string{"resourceId": id}, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	res := resourceFromWire(w)
	return &res, nil
}

// ResourceDeleteParams configures Resources.Delete.
type ResourceDeleteParams struct {
	// Cascade deletes every descendant that is not deleted together with the
	// resource. Without it, a resource with live children fails with
	// ErrHasChildren and nothing is deleted.
	Cascade bool
}

// Delete deletes the resource with its documents and properties. A retried
// delete whose first attempt was applied answers ErrNotFound or
// ErrAlreadyDeleted; read the resource before treating either as a failure.
func (s *ResourcesService) Delete(ctx context.Context, id string, p ResourceDeleteParams, opts ...CallOption) error {
	var q url.Values
	if p.Cascade {
		q = url.Values{"cascade": {"true"}}
	}
	return s.c.call(ctx, gen.OpDeleteResource.Method, gen.OpDeleteResource.Path,
		map[string]string{"resourceId": id}, request{query: q}, opts)
}

// Restore restores a deleted resource and the descendants deleted with it,
// and returns it. It needs the admin role.
func (s *ResourcesService) Restore(ctx context.Context, id string, opts ...CallOption) (*Resource, error) {
	var w gen.Resource
	if err := s.c.call(ctx, http.MethodPost, gen.OpRestoreResource.Path,
		map[string]string{"resourceId": id}, request{out: &w}, opts); err != nil {
		return nil, err
	}
	res := resourceFromWire(w)
	return &res, nil
}

// convOpt converts an Optional between two string-based enum types.
func convOpt[B, A ~string](o opt.Optional[A]) opt.Optional[B] {
	if v, ok := o.Get(); ok {
		return opt.Some(B(v))
	}
	return opt.Optional[B]{}
}

func resourceFromWire(w gen.Resource) Resource {
	r := Resource{
		ID:              w.ID,
		ParentID:        w.ParentID.Or(""),
		Name:            w.Name,
		Tags:            w.Tags,
		Aliases:         w.Aliases,
		IconGlyph:       w.IconGlyph.Or(""),
		IconColor:       w.IconColor.Or(""),
		IconShape:       IconShape(w.IconShape.Or("")),
		IconSize:        IconSize(w.IconSize.Or("")),
		Rank:            w.Rank,
		IsHidden:        w.IsHidden,
		IsLocked:        w.IsLocked.Or(false),
		IsFullWidth:     w.IsFullWidth.Or(false),
		ShowPropertyBar: w.ShowPropertyBar.Or(false),
		ImageURL:        w.ImageURL.Or(""),
		CreatedAt:       utc(w.CreatedAt.Or(time.Time{})),
		UpdatedAt:       utc(w.UpdatedAt.Or(time.Time{})),
	}
	if b, ok := w.Banner.Get(); ok {
		r.Banner = &Banner{Enabled: b.Enabled, URL: b.URL, YPosition: b.YPosition}
	}
	if w.Permissions != nil {
		r.Permissions = make(map[string]PermissionLevel, len(w.Permissions))
		for k, v := range w.Permissions {
			r.Permissions[k] = PermissionLevel(v)
		}
	}
	for _, d := range w.Documents {
		r.Documents = append(r.Documents, documentFromWire(d))
	}
	for _, p := range w.Properties {
		prop, err := blocks.UnmarshalProperty(p.Raw)
		if err != nil {
			// Not a property object the blocks package can read; keep it
			// as is rather than fail the whole resource.
			prop = &blocks.RawProperty{KindName: blocks.Kind(p.Kind), JSON: p.Raw}
		}
		r.Properties = append(r.Properties, prop)
	}
	return r
}

func listItemFromWire(w gen.ResourceListItem) ResourceListItem {
	r := ResourceListItem{
		ID:          w.ID,
		ParentID:    w.ParentID.Or(""),
		Name:        w.Name,
		Tags:        w.Tags,
		Aliases:     w.Aliases,
		IconGlyph:   w.IconGlyph.Or(""),
		IconColor:   w.IconColor.Or(""),
		IconShape:   IconShape(w.IconShape.Or("")),
		IconSize:    IconSize(w.IconSize.Or("")),
		Rank:        w.Rank.Or(""),
		IsHidden:    w.IsHidden.Or(false),
		HasChildren: w.HasChildren,
		CreatedAt:   utc(w.CreatedAt.Or(time.Time{})),
		UpdatedAt:   utc(w.UpdatedAt.Or(time.Time{})),
	}
	for _, d := range w.Documents {
		r.Documents = append(r.Documents, DocumentSummary{ID: d.ID, Name: d.Name.Or(""), Type: DocumentType(d.Type.Or(""))})
	}
	return r
}

// SortByRank orders siblings the way the app does: by Rank compared as plain
// byte strings, never with locale collation.
func SortByRank[T interface{ rank() string }](items []T) {
	slices.SortStableFunc(items, func(a, b T) int { return strings.Compare(a.rank(), b.rank()) })
}

func (r Resource) rank() string         { return r.Rank }
func (r ResourceListItem) rank() string { return r.Rank }
