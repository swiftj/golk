package lk

import (
	"context"
	"iter"
	"net/url"
	"strconv"

	"github.com/swiftj/golk/lk/internal/gen"
)

// SearchService searches the project.
type SearchService struct{ c *Client }

// SearchParams is the input of Search.Search.
type SearchParams struct {
	// Query is matched against resource names, aliases and document
	// content, up to 500 characters. An empty query returns no results.
	Query string
	// Limit is the number of results (1–100). Zero uses the server default
	// (32).
	Limit int
}

// SearchHit is one result: a resource, and the document whose content
// matched when the match is in content. A resource appears once per
// matching document.
type SearchHit struct {
	Resource SearchResource
	// Document is nil when the resource matched by name or alias.
	Document *SearchDocument
}

// SearchResource is the resource of a search hit.
type SearchResource struct {
	ID        string
	Name      string
	IconGlyph string
	IconColor string
	IconShape IconShape
	IconSize  IconSize
	// ParentID is empty for a top-level resource.
	ParentID string
	Tags     []string
	// Path is the resource's ancestors from the root down.
	Path []ResourceAncestor
}

// SearchDocument is the document of a content match.
type SearchDocument struct {
	ID         string
	Name       string
	Highlights []SearchHighlight
	// IsSolo reports that this is the resource's only document in the
	// results.
	IsSolo bool
}

// SearchHighlight is the matching passage with the offsets of its first
// match.
type SearchHighlight struct {
	Segment    string
	Index      int
	MatchStart int
	MatchEnd   int
}

// Search returns the best matches first, on one page.
func (s *SearchService) Search(ctx context.Context, p SearchParams, opts ...CallOption) (*Page[SearchHit], error) {
	q := url.Values{"q": {p.Query}}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var w gen.SearchResultList
	if err := s.c.call(ctx, gen.OpSearch.Method, gen.OpSearch.Path, nil, request{query: q, out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]SearchHit, len(w.Data))
	for i, h := range w.Data {
		r := h.Resource
		hit := SearchHit{Resource: SearchResource{
			ID: r.ID, Name: r.Name, IconGlyph: r.IconGlyph.Or(""), IconColor: r.IconColor.Or(""),
			IconShape: IconShape(r.IconShape.Or("")), IconSize: IconSize(r.IconSize.Or("")),
			ParentID: r.ParentID.Or(""), Tags: r.Tags,
		}}
		for _, a := range r.Path.Or(nil) {
			hit.Resource.Path = append(hit.Resource.Path, ResourceAncestor{ID: a.ID, Name: a.Name})
		}
		if d, ok := h.Document.Get(); ok {
			doc := &SearchDocument{ID: d.ID, Name: d.Name, IsSolo: d.IsSolo.Or(false)}
			for _, hl := range d.Highlights.Or(nil) {
				doc.Highlights = append(doc.Highlights, SearchHighlight{
					Segment: hl.Segment, Index: int(hl.Index), MatchStart: int(hl.MatchStart), MatchEnd: int(hl.MatchEnd),
				})
			}
			hit.Document = doc
		}
		out[i] = hit
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// TagsService lists the project's tags.
type TagsService struct{ c *Client }

// Tag is a distinct tag and the number of resources that carry it.
type Tag struct {
	Tag   string
	Count int
}

// List returns every tag on the project's resources, most used first, on
// one page. Resources in the trash are not counted. It needs the admin
// role; a member can read tags from Resources.List instead.
func (s *TagsService) List(ctx context.Context, opts ...CallOption) (*Page[Tag], error) {
	var w gen.TagList
	if err := s.c.call(ctx, gen.OpListTags.Method, gen.OpListTags.Path, nil, request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Tag, len(w.Data))
	for i, t := range w.Data {
		out[i] = Tag{Tag: t.Tag, Count: int(t.Count)}
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// All iterates over the project's tags.
func (s *TagsService) All(ctx context.Context, opts ...CallOption) iter.Seq2[Tag, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Tag], error) { return s.List(ctx, opts...) })
}
