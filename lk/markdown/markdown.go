// Package markdown converts LegendKeeper pages (blocks.List) to LK Markdown
// and back. The format is specified in docs/LK_MARKDOWN.md: CommonMark + GFM
// for standard blocks, ::: directives for LegendKeeper-only blocks, and
// lk-raw fences for anything Markdown can't express, so a round trip never
// drops content. Parsing fails closed: input the converter does not fully
// understand is an error, never a guess.
package markdown

import (
	"github.com/swiftj/golk/lk/blocks"
)

// ToOptions configures ToMarkdown.
type ToOptions struct {
	// IDs writes {#id} on every block that has one (LK_MARKDOWN.md §3.2).
	// Off by default: with FromOptions.Base, ids are restored without
	// appearing in the text.
	IDs bool
}

// FromOptions configures FromMarkdown.
type FromOptions struct {
	// Base is the page the Markdown was rendered from. Blocks the edit left
	// unchanged come back exactly as they are in Base, and changed blocks
	// keep their ids (LK_MARKDOWN.md §3.1).
	Base blocks.List
	// AllowRemoveOpaque permits an edit that drops an unsupported or lk-raw
	// block of Base (M-SAFE-04).
	AllowRemoveOpaque bool
}

// Report says what FromMarkdown did relative to Base, so a caller can show
// it before writing.
type Report struct {
	// Kept lists the ids of base blocks restored verbatim.
	Kept []string
	// Changed lists the ids of base blocks that were edited in place.
	Changed []string
	// Added lists the paths (as blocks.Walk writes them) of new blocks.
	Added []string
	// Removed lists the base blocks the edit dropped.
	Removed []RemovedBlock
}

// RemovedBlock is a base block an edit dropped.
type RemovedBlock struct {
	ID   string
	Kind blocks.Kind
	// Secret is true for a secret block, or a block holding one.
	Secret bool
	// Opaque is true for an unsupported or lk-raw block, or a block holding
	// one.
	Opaque bool
}

// ToMarkdown renders a page as LK Markdown.
func ToMarkdown(page blocks.List, opts ToOptions) (string, error) {
	return render(page, opts)
}

// FromMarkdown parses LK Markdown into a page. On error it returns no
// blocks; a *SyntaxError reports where the input is wrong.
func FromMarkdown(src string, opts FromOptions) (blocks.List, *Report, error) {
	parsed, err := parse(src)
	if err != nil {
		return nil, nil, err
	}
	return reconcile(parsed, opts.Base, codec, opts.AllowRemoveOpaque)
}

// hooks is what base matching needs from the codec.
type hooks struct {
	// key is the canonical rendering of one node (a block, or an infobox
	// row), ignoring ids and every other field Markdown doesn't show. Two
	// nodes with equal keys parse to equal blocks.
	key func(blocks.Node) string
	// opaque reports whether a node renders as ::unsupported or lk-raw.
	opaque func(blocks.Node) bool
}
