package markdown

import (
	"errors"

	"github.com/swiftj/golk/lk/blocks"
)

// codec is the rendering the base matcher uses.
var codec = hooks{key: renderKey, opaque: rendersOpaque}

// render renders page (LK_MARKDOWN.md §2).
func render(page blocks.List, opts ToOptions) (string, error) {
	return "", errors.New("markdown: render not implemented")
}

// renderKey is the canonical rendering of one node without ids.
func renderKey(n blocks.Node) string { return "" }

// rendersOpaque reports whether n renders as ::unsupported or lk-raw.
func rendersOpaque(n blocks.Node) bool { return false }
