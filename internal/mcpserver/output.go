package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/swiftj/golk/lk"
)

// pageCacheSize bounds the pages kept for cursors.
const pageCacheSize = 64

// pageCache keeps the pages behind the cursors a list tool returned, so a
// later call with that cursor can fetch the next page with Page.Next. The
// lk SDK has no entry point that starts a list at a cursor, so a cursor
// works only within one server session and only with the same filters.
type pageCache struct {
	mu    sync.Mutex
	pages map[string]any
	order []string
}

func newPageCache() *pageCache { return &pageCache{pages: map[string]any{}} }

func (pc *pageCache) put(key string, p any) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if _, ok := pc.pages[key]; !ok {
		pc.order = append(pc.order, key)
	}
	pc.pages[key] = p
	for len(pc.order) > pageCacheSize {
		delete(pc.pages, pc.order[0])
		pc.order = pc.order[1:]
	}
}

func (pc *pageCache) get(key string) (any, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	p, ok := pc.pages[key]
	return p, ok
}

// fetchPage returns the first page (cursor empty) or the page after the
// one that returned cursor. scope names the tool, project and filters, so a
// cursor can't be reused with other filters.
func fetchPage[T any](ctx context.Context, pc *pageCache, tool, scope, cursor string,
	first func(context.Context) (*lk.Page[T], error)) (*lk.Page[T], error) {
	var p *lk.Page[T]
	if cursor == "" {
		var err error
		if p, err = first(ctx); err != nil {
			return nil, err
		}
	} else {
		v, ok := pc.get(scope + "\x00" + cursor)
		prev, isPage := v.(*lk.Page[T])
		if !ok || !isPage {
			return nil, fmt.Errorf("unknown cursor for these filters. Hint: cursors last for this server session only and need the same filters; call %s again without cursor", tool)
		}
		next, err := prev.Next(ctx)
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, fmt.Errorf("no page follows that cursor. Hint: call %s again without cursor", tool)
		}
		p = next
	}
	if p.HasMore && p.NextCursor != "" {
		pc.put(scope+"\x00"+p.NextCursor, p)
	}
	return p, nil
}

// checkLimit validates an optional 1–100 page size.
func checkPageLimit(limit int) error {
	if limit < 0 || limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100, not %d (omit it for the server default)", limit)
	}
	return nil
}

// flags renders bracketed flags, such as " [hidden]", or "".
func listFlags(fs ...string) string {
	var on []string
	for _, f := range fs {
		if f != "" {
			on = append(on, f)
		}
	}
	if len(on) == 0 {
		return ""
	}
	return " [" + strings.Join(on, ", ") + "]"
}

func flagIf(b bool, name string) string {
	if b {
		return name
	}
	return ""
}

// moreLine is the closing line of a paged listing.
func pagedFooter(b *strings.Builder, n int, noun, next string, hasMore bool) {
	switch {
	case hasMore && next != "":
		fmt.Fprintf(b, "%d %s on this page; more follow: pass cursor %q for the next page.", n, noun, next)
	case hasMore:
		fmt.Fprintf(b, "%d %s on this page; the server says more follow but gave no cursor.", n, noun)
	default:
		fmt.Fprintf(b, "%d %s; no more pages.", n, noun)
	}
}
