package markdown

import (
	"context"
	"errors"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// Page is a document's content as LK Markdown, with the version and blocks
// it was rendered from. Pass it to Replace as the base of an edit.
type Page struct {
	Markdown string
	Version  string
	Blocks   blocks.List
}

// Get reads a page and renders it as LK Markdown.
func Get(ctx context.Context, c *lk.Client, doc lk.DocRef, opts ...lk.CallOption) (*Page, error) {
	db, err := c.Documents.GetBlocks(ctx, doc, opts...)
	if err != nil {
		return nil, err
	}
	md, err := ToMarkdown(db.Blocks, ToOptions{})
	if err != nil {
		return nil, err
	}
	return &Page{Markdown: md, Version: db.Version, Blocks: db.Blocks}, nil
}

// ReplaceOptions configures Replace.
type ReplaceOptions struct {
	// DryRun parses and matches the edit but sends nothing. Replace then
	// returns the blocks it would write, with base's version.
	DryRun bool
	// AllowRemoveOpaque permits an edit that drops an unsupported or lk-raw
	// block of the base page (M-SAFE-04).
	AllowRemoveOpaque bool
}

// ErrNoBase reports a Replace without a base page or its version.
var ErrNoBase = errors.New("markdown: Replace needs the base page and its version (from Get)")

// Replace parses md against base and replaces the page with the result. It
// always sends IfMatch(base.Version), so if the page changed since base was
// read it fails with lk.ErrVersionMismatch instead of overwriting the
// change (M-API-04). The Report says what the edit kept, changed, added and
// removed.
func Replace(ctx context.Context, c *lk.Client, doc lk.DocRef, base *Page, md string, ro ReplaceOptions, opts ...lk.CallOption) (*lk.DocumentBlocks, *Report, error) {
	if base == nil || base.Version == "" {
		return nil, nil, ErrNoBase
	}
	content, rep, err := FromMarkdown(md, FromOptions{Base: base.Blocks, AllowRemoveOpaque: ro.AllowRemoveOpaque})
	if err != nil {
		return nil, nil, err
	}
	if ro.DryRun {
		return &lk.DocumentBlocks{
			ID: doc.DocumentID, Version: base.Version, Text: blocks.PlainText(content), Blocks: content,
		}, rep, nil
	}
	// IfMatch goes last so a caller's own IfMatch can't replace it.
	opts = append(opts[:len(opts):len(opts)], lk.IfMatch(base.Version))
	db, err := c.Documents.ReplaceBlocks(ctx, doc, content, opts...)
	if err != nil {
		return nil, nil, err
	}
	return db, rep, nil
}
