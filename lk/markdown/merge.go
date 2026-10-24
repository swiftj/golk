package markdown

import (
	"errors"

	"github.com/swiftj/golk/lk/blocks"
)

// reconcile matches parsed against base (LK_MARKDOWN.md §3.1) and applies the
// id rules M-ID-01..03, M-SAFE-04 and M-SAFE-06. base may be nil.
func reconcile(parsed, base blocks.List, h hooks, allowRemoveOpaque bool) (blocks.List, *Report, error) {
	return nil, nil, errors.New("markdown: reconcile not implemented")
}
