package lk

import (
	"context"
	"iter"
	"net/url"

	"github.com/swiftj/golk/lk/internal/opt"
)

// Page is one page of a list. Server-paged lists (assets, projects,
// resources, templates, timeline events) return pages of up to their limit;
// every other list returns the whole collection on one page.
type Page[T any] struct {
	// Items are the items on this page.
	Items []T
	// HasMore reports whether another page follows.
	HasMore bool
	// NextCursor fetches the next page. It is opaque: pass it back unchanged.
	NextCursor string

	inconsistent bool
	fetch        func(ctx context.Context, cursor string) (*Page[T], error)
}

// Next fetches the page after p. It returns (nil, nil) after the last page.
// Each call sends a request, so keep the page it returns.
func (p *Page[T]) Next(ctx context.Context) (*Page[T], error) {
	if p.inconsistent {
		return nil, ErrPaginationInconsistent
	}
	if !p.HasMore || p.NextCursor == "" || p.fetch == nil {
		return nil, nil
	}
	return p.fetch(ctx, p.NextCursor)
}

// All yields every item from p to the end of the collection, fetching pages
// as it goes. On an error it yields (zero, err) once and stops. Stopping the
// loop early sends no further requests.
func (p *Page[T]) All(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for page := p; page != nil; {
			for _, it := range page.Items {
				if !yield(it, nil) {
					return
				}
			}
			next, err := page.Next(ctx)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			page = next
		}
	}
}

// iterate turns a first-page fetch into an iterator over the whole collection.
func iterate[T any](ctx context.Context, first func(context.Context) (*Page[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		p, err := first(ctx)
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		p.All(ctx)(yield)
	}
}

// pager fetches pages of a server-paged list. The filters in query are kept
// for every page; only the cursor changes.
type pager[W any, T any] struct {
	c      *Client
	tmpl   string
	params map[string]string
	query  url.Values
	opts   []CallOption
	// items extracts the page fields from the decoded wire list.
	items func(W) ([]T, opt.Optional[bool], opt.Nullable[string])
}

func (pg *pager[W, T]) page(ctx context.Context, cursor string) (*Page[T], error) {
	q := url.Values{}
	for k, v := range pg.query {
		q[k] = append([]string(nil), v...)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	params := map[string]string{}
	for k, v := range pg.params {
		params[k] = v
	}
	var w W
	if err := pg.c.call(ctx, "GET", pg.tmpl, params, request{query: q, out: &w}, pg.opts); err != nil {
		return nil, err
	}
	items, hasMore, next := pg.items(w)
	cur := next.Or("")
	more := hasMore.Or(cur != "")
	return &Page[T]{
		Items:        items,
		HasMore:      more,
		NextCursor:   cur,
		inconsistent: more && cur == "",
		fetch:        pg.page,
	}, nil
}
