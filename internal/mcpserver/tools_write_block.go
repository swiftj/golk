package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

// The block tools change one block of a page at a time. Every write takes
// the page version (from read_page or read_block) as If-Match.

// BlockDocArg names a page and the version an edit starts from.
type BlockDocArg struct {
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the page"`
	DocumentID string `json:"document_id" jsonschema:"the page document"`
	Version    string `json:"version" jsonschema:"the page version from read_page or read_block, sent as If-Match"`
}

func (a BlockDocArg) check() error {
	if a.ResourceID == "" || a.DocumentID == "" {
		return errors.New("resource_id and document_id are required")
	}
	if a.Version == "" {
		return errors.New("version is required: the page version from read_page or read_block")
	}
	return nil
}

func (a BlockDocArg) ref() lk.DocRef {
	return lk.DocRef{ResourceID: a.ResourceID, DocumentID: a.DocumentID}
}

// AnchorArgs place a block: right after or before a sibling, or as the last
// child of a parent. At most one.
type AnchorArgs struct {
	After  string `json:"after,omitempty" jsonschema:"place it right after this block id"`
	Before string `json:"before,omitempty" jsonschema:"place it right before this block id"`
	Parent string `json:"parent,omitempty" jsonschema:"make it the last child of this container block id (a list item, quote, column, secret, callout, ...)"`
}

func (a AnchorArgs) at() lk.At { return lk.At{After: a.After, Before: a.Before, Parent: a.Parent} }

func (a AnchorArgs) count() int {
	n := 0
	for _, v := range []string{a.After, a.Before, a.Parent} {
		if v != "" {
			n++
		}
	}
	return n
}

// subNodes are the nodes directly inside n: a container's children, and an
// infobox's rows with their own children.
func subNodes(n blocks.Node) []blocks.Node {
	var out []blocks.Node
	if l, ok := blocks.Children(n); ok {
		for _, b := range l {
			out = append(out, b)
		}
	}
	if ib, ok := n.(*blocks.InfoboxBlock); ok {
		for _, p := range ib.Rows {
			out = append(out, p)
		}
	}
	return out
}

// secretAt reports whether the node id is inside a secret block (strictly:
// one of its ancestors is secret), and whether it was found.
func secretAt(page blocks.List, id string) (inSecret, found bool) {
	var walk func(n blocks.Node, secret bool) (bool, bool)
	walk = func(n blocks.Node, secret bool) (bool, bool) {
		if n.NodeID() == id {
			return secret, true
		}
		secret = secret || n.Kind() == blocks.KindSecret
		for _, k := range subNodes(n) {
			if s, ok := walk(k, secret); ok {
				return s, true
			}
		}
		return false, false
	}
	for _, b := range page {
		if s, ok := walk(b, false); ok {
			return s, true
		}
	}
	return false, false
}

// blockContext is what the guard needs about a place on a page: whether
// players can see it at all.
type blockContext struct {
	shown bool
	page  blocks.List
}

// pageContext reads whether players can see the page (resource and
// document not hidden) and, when they can and the guard is on, the page.
func (s *server) pageContext(ctx context.Context, c *lk.Client, a BlockDocArg) (blockContext, error) {
	r, err := c.Resources.Get(ctx, a.ResourceID)
	if err != nil {
		return blockContext{}, err
	}
	d, err := findDoc(r, a.DocumentID)
	if err != nil {
		return blockContext{}, err
	}
	bc := blockContext{shown: !r.IsHidden && !d.IsHidden && !s.cfg.GuardOff}
	if bc.shown {
		db, err := c.Documents.GetBlocks(ctx, a.ref())
		if err != nil {
			return blockContext{}, err
		}
		bc.page = db.Blocks
	}
	return bc, nil
}

// destSecret reports whether a block placed at an anchor sits inside a
// secret block: under a secret parent (or one inside a secret), or next to
// a sibling inside one.
func (bc blockContext) destSecret(a AnchorArgs) bool {
	switch {
	case a.Parent != "":
		in, _ := secretAt(bc.page, a.Parent)
		if n := blocks.FindBlock(bc.page, a.Parent); n != nil && n.Kind() == blocks.KindSecret {
			return true
		}
		return in
	case a.After != "" || a.Before != "":
		in, _ := secretAt(bc.page, a.After+a.Before)
		return in
	}
	return false
}

// oneBlock parses Markdown holding exactly one block.
func oneBlock(md string) (blocks.Block, error) {
	l, _, err := markdown.FromMarkdown(md, markdown.FromOptions{})
	if err != nil {
		return nil, err
	}
	if len(l) != 1 {
		return nil, fmt.Errorf("markdown must hold exactly one block, not %d. Hint: wrap several in a container, or insert them one at a time", len(l))
	}
	return l[0], nil
}

// BlockOut is the block tools' output.
type BlockOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	BlockID    string `json:"block_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Version    string `json:"version,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func blockMarkdown(b blocks.Block) string {
	md, err := markdown.ToMarkdown(blocks.List{b}, markdown.ToOptions{IDs: true})
	if err != nil {
		return "(can't render: " + err.Error() + ")"
	}
	return md
}

// ReadBlockIn is read_block's input.
type ReadBlockIn struct {
	ProjectArg
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the page"`
	DocumentID string `json:"document_id" jsonschema:"the page document"`
	BlockID    string `json:"block_id" jsonschema:"the block (read_page with ids shows block ids)"`
}

func (s *server) readBlock(ctx context.Context, c *lk.Client, in ReadBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if in.ResourceID == "" || in.DocumentID == "" || in.BlockID == "" {
		return zero, errors.New("resource_id, document_id and block_id are required")
	}
	v, err := c.Blocks.Get(ctx, lk.DocRef{ResourceID: in.ResourceID, DocumentID: in.DocumentID}, in.BlockID)
	if err != nil {
		return zero, err
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, BlockID: in.BlockID, Kind: string(v.Value.Kind()), Version: v.Version}
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Block %s (%s) of page %s, page version %s, as LK Markdown with ids:\n\n%s",
		in.BlockID, v.Value.Kind(), in.DocumentID, v.Version, blockMarkdown(v.Value))}, nil
}

// InsertBlockIn is insert_block's input.
type InsertBlockIn struct {
	ProjectArg
	DryRunArg
	BlockDocArg
	AnchorArgs
	Markdown string `json:"markdown" jsonschema:"exactly one block as LK Markdown (a paragraph, heading, list, :::callout, :::secret, table, ...)"`
}

func (s *server) insertBlock(ctx context.Context, c *lk.Client, in InsertBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.count() > 1 {
		return zero, errors.New("give at most one of after, before and parent (none appends to the end of the page)")
	}
	b, err := oneBlock(in.Markdown)
	if err != nil {
		return zero, err
	}
	bc, err := s.pageContext(ctx, c, in.BlockDocArg)
	if err != nil {
		return zero, err
	}
	if bc.shown && !bc.destSecret(in.AnchorArgs) {
		if err := s.check(map[string]any{"blocks": blocks.List{b}}); err != nil {
			return zero, err
		}
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, Kind: string(b.Kind()), DryRun: in.DryRun}
	where := anchorText(in.AnchorArgs)
	if in.DryRun {
		return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Dry run: would insert a %s block %s on page %s. Nothing was written.", b.Kind(), where, in.DocumentID)}, nil
	}
	v, err := c.Blocks.Insert(ctx, in.ref(), b, in.at(), lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.BlockID, out.Version = v.Value.NodeID(), v.Version
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Inserted %s block %s %s on page %s; new version %s.", b.Kind(), out.BlockID, where, in.DocumentID, v.Version)}, nil
}

func anchorText(a AnchorArgs) string {
	switch {
	case a.After != "":
		return "after block " + a.After
	case a.Before != "":
		return "before block " + a.Before
	case a.Parent != "":
		return "at the end of block " + a.Parent
	}
	return "at the end of the page"
}

// ReplaceBlockIn is replace_block's input.
type ReplaceBlockIn struct {
	ProjectArg
	DryRunArg
	BlockDocArg
	BlockID  string `json:"block_id" jsonschema:"the block to replace, with its subtree"`
	Markdown string `json:"markdown" jsonschema:"exactly one block as LK Markdown; it keeps the block's id"`
}

func (s *server) replaceBlock(ctx context.Context, c *lk.Client, in ReplaceBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.BlockID == "" {
		return zero, errors.New("block_id is required")
	}
	b, err := oneBlock(in.Markdown)
	if err != nil {
		return zero, err
	}
	bc, err := s.pageContext(ctx, c, in.BlockDocArg)
	if err != nil {
		return zero, err
	}
	if bc.shown {
		in2, found := secretAt(bc.page, in.BlockID)
		if !found {
			return zero, fmt.Errorf("page %s has no block %s", in.DocumentID, in.BlockID)
		}
		if !in2 {
			if err := s.check(map[string]any{"blocks": blocks.List{b}}); err != nil {
				return zero, err
			}
		}
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, BlockID: in.BlockID, Kind: string(b.Kind()), DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Dry run: would replace block %s of page %s with a %s block. Nothing was written.", in.BlockID, in.DocumentID, b.Kind())}, nil
	}
	v, err := c.Blocks.Replace(ctx, in.ref(), in.BlockID, b, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version = v.Version
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Replaced block %s of page %s with a %s block; new version %s.", in.BlockID, in.DocumentID, b.Kind(), v.Version)}, nil
}

// UpdateBlockIn is update_block's input.
type UpdateBlockIn struct {
	ProjectArg
	DryRunArg
	BlockDocArg
	BlockID string `json:"block_id" jsonschema:"the block whose own fields to change; its children stay"`
	// Patch is in the API's field names, so every updateBlock field is
	// reachable.
	Patch           map[string]any `json:"patch,omitempty" jsonschema:"fields to set, in the API's names: level, align, background, text, language, checked, title, isTitleHidden, fill, tone, icon, color, header, colspan, rowspan, start, width, url, alt, provider, layout, display, rows, mode, items, showText, showIcon, showMax, filter, resourceId, documentId, viewport, displayHeight, content (runs)"`
	ContentMarkdown *string        `json:"content_markdown,omitempty" jsonschema:"the block's text as inline LK Markdown (one paragraph), instead of content runs in patch"`
}

// decodeBlockPatch turns a JSON patch into an lk.BlockPatch. rows and items
// mean different things by block kind.
func decodeBlockPatch(raw map[string]any, kind blocks.Kind) (lk.BlockPatch, error) {
	var p lk.BlockPatch
	rest := map[string]any{}
	for k, v := range raw {
		rest[k] = v
	}
	rows, hasRows := rest["rows"]
	items, hasItems := rest["items"]
	delete(rest, "rows")
	delete(rest, "items")
	strict := func(v any, into any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		return dec.Decode(into)
	}
	if err := strict(rest, &p); err != nil {
		return p, fmt.Errorf("patch: %w", err)
	}
	if hasRows {
		if kind == blocks.KindInfobox {
			var v blocks.Properties
			if err := strict(rows, &v); err != nil {
				return p, fmt.Errorf("patch.rows: %w", err)
			}
			p.InfoboxRows = lk.Some(v)
		} else {
			var v blocks.KVRows
			if err := strict(rows, &v); err != nil {
				return p, fmt.Errorf("patch.rows: %w", err)
			}
			p.Rows = lk.Some(v)
		}
	}
	if hasItems {
		if kind == blocks.KindMeter {
			var v blocks.MeterItems
			if err := strict(items, &v); err != nil {
				return p, fmt.Errorf("patch.items: %w", err)
			}
			p.MeterItems = lk.Some(v)
		} else {
			var v blocks.ResourceItems
			if err := strict(items, &v); err != nil {
				return p, fmt.Errorf("patch.items: %w", err)
			}
			p.ResourceItems = lk.Some(v)
		}
	}
	return p, nil
}

func (s *server) updateBlock(ctx context.Context, c *lk.Client, in UpdateBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.BlockID == "" {
		return zero, errors.New("block_id is required")
	}
	if len(in.Patch) == 0 && in.ContentMarkdown == nil {
		return zero, errors.New("nothing to change: give patch fields or content_markdown")
	}
	if _, ok := in.Patch["content"]; ok && in.ContentMarkdown != nil {
		return zero, errors.New("give content in patch or content_markdown, not both")
	}
	cur, err := c.Blocks.Get(ctx, in.ref(), in.BlockID)
	if err != nil {
		return zero, err
	}
	p, err := decodeBlockPatch(in.Patch, cur.Value.Kind())
	if err != nil {
		return zero, err
	}
	shown := map[string]any{}
	for k, v := range in.Patch {
		shown[k] = v
	}
	if in.ContentMarkdown != nil {
		runs, err := inlineRuns("content", *in.ContentMarkdown)
		if err != nil {
			return zero, err
		}
		p.Content = lk.Some(runs)
		shown["content"] = runs
	}
	bc, err := s.pageContext(ctx, c, in.BlockDocArg)
	if err != nil {
		return zero, err
	}
	if bc.shown {
		if in2, _ := secretAt(bc.page, in.BlockID); !in2 && cur.Value.Kind() != blocks.KindSecret {
			if err := s.check(map[string]any{"block": shown}); err != nil {
				return zero, err
			}
		}
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, BlockID: in.BlockID, Kind: string(cur.Value.Kind()), DryRun: in.DryRun}
	fields := make([]string, 0, len(shown))
	for k := range shown {
		fields = append(fields, k)
	}
	if in.DryRun {
		return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Dry run: would set %s on %s block %s of page %s. Nothing was written.",
			strings.Join(sortedStrings(fields), ", "), cur.Value.Kind(), in.BlockID, in.DocumentID)}, nil
	}
	v, err := c.Blocks.Update(ctx, in.ref(), in.BlockID, p, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version = v.Version
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Updated %s block %s of page %s (%s); new version %s.",
		cur.Value.Kind(), in.BlockID, in.DocumentID, strings.Join(sortedStrings(fields), ", "), v.Version)}, nil
}

// MoveBlockIn is move_block's input.
type MoveBlockIn struct {
	ProjectArg
	DryRunArg
	BlockDocArg
	AnchorArgs
	BlockID string `json:"block_id" jsonschema:"the block to move, with its subtree"`
}

func (s *server) moveBlock(ctx context.Context, c *lk.Client, in MoveBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.BlockID == "" || in.count() != 1 {
		return zero, errors.New("give block_id and exactly one of after, before and parent")
	}
	bc, err := s.pageContext(ctx, c, in.BlockDocArg)
	if err != nil {
		return zero, err
	}
	// Moving a block out of a secret shows it to players (X-GUARD-15).
	if bc.shown {
		from, found := secretAt(bc.page, in.BlockID)
		if !found {
			return zero, fmt.Errorf("page %s has no block %s", in.DocumentID, in.BlockID)
		}
		if from && !bc.destSecret(in.AnchorArgs) {
			if err := s.check(map[string]any{"blocks": blocks.List{blocks.FindBlock(bc.page, in.BlockID).(blocks.Block)}}); err != nil {
				return zero, err
			}
		}
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, BlockID: in.BlockID, DryRun: in.DryRun}
	where := anchorText(in.AnchorArgs)
	if in.DryRun {
		return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Dry run: would move block %s %s on page %s. Nothing was written.", in.BlockID, where, in.DocumentID)}, nil
	}
	v, err := c.Blocks.Move(ctx, in.ref(), in.BlockID, in.at(), lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Kind, out.Version = string(v.Value.Kind()), v.Version
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Moved block %s %s on page %s; new version %s.", in.BlockID, where, in.DocumentID, v.Version)}, nil
}

// DeleteBlockIn is delete_block's input.
type DeleteBlockIn struct {
	ProjectArg
	DryRunArg
	BlockDocArg
	BlockID string `json:"block_id" jsonschema:"the block to delete, with its subtree"`
}

func (s *server) deleteBlock(ctx context.Context, c *lk.Client, in DeleteBlockIn) (toolResult[BlockOut], error) {
	var zero toolResult[BlockOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.BlockID == "" {
		return zero, errors.New("block_id is required")
	}
	// No guard: deleting shows players nothing.
	cur, err := c.Blocks.Get(ctx, in.ref(), in.BlockID)
	if err != nil {
		return zero, err
	}
	out := BlockOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, BlockID: in.BlockID, Kind: string(cur.Value.Kind()), DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Dry run: would delete this %s block (with everything inside it) from page %s:\n\n%s\nNothing was written.",
			cur.Value.Kind(), in.DocumentID, blockMarkdown(cur.Value))}, nil
	}
	if err := c.Blocks.Delete(ctx, in.ref(), in.BlockID, lk.IfMatch(in.Version)); err != nil {
		return zero, err
	}
	return toolResult[BlockOut]{out: out, text: fmt.Sprintf("Deleted %s block %s from page %s. The page has a new version; read it again before the next edit.", cur.Value.Kind(), in.BlockID, in.DocumentID)}, nil
}

func sortedStrings(v []string) []string {
	out := slices.Clone(v)
	slices.Sort(out)
	return out
}

func (s *server) addBlockTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name:        "read_block",
		Description: "Read one block of a page, with everything inside it, as LK Markdown with ids, plus the page version for the block tools.",
		Annotations: readOnly("Read block"),
	}, func(in ReadBlockIn) string { return in.Project }, s.readBlock)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "insert_block",
		Description: "Insert one block (given as LK Markdown) into a page: after or before a block, as the last child of a container block (parent), or at the end. " +
			"version (from read_page or read_block) is required. The block is checked by the spoiler guard unless the page or resource is hidden or it lands inside a :::secret block.",
		Annotations: write("Insert block", false, false),
	}, func(in InsertBlockIn) string { return in.Project }, serial(s, s.insertBlock))
	addTool(s, ms, &mcp.Tool{
		Name:        "replace_block",
		Description: "Replace one block, with its subtree, by a block given as LK Markdown; it keeps its id. version is required. Checked by the spoiler guard like insert_block.",
		Annotations: write("Replace block", true, false),
	}, func(in ReplaceBlockIn) string { return in.Project }, serial(s, s.replaceBlock))
	addTool(s, ms, &mcp.Tool{
		Name: "update_block",
		Description: "Change one block's own fields without touching its children: e.g. tick a task (checked), a heading's level, a callout's tone or icon, alignment, a table cell's span, " +
			"an embed's url, or its text (content_markdown). patch uses the API's field names. version is required.",
		Annotations: write("Update block", false, true),
	}, func(in UpdateBlockIn) string { return in.Project }, serial(s, s.updateBlock))
	addTool(s, ms, &mcp.Tool{
		Name: "move_block",
		Description: "Move one block, with its subtree, after or before another block or into a container block (parent). version is required. " +
			"Moving a block out of a :::secret block into player-visible content is checked by the spoiler guard.",
		Annotations: write("Move block", false, false),
	}, func(in MoveBlockIn) string { return in.Project }, serial(s, s.moveBlock))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_block",
		Description: "Delete one block with everything inside it. version is required. Use dry_run to see the block that would go.",
		Annotations: write("Delete block", true, false),
	}, func(in DeleteBlockIn) string { return in.Project }, serial(s, s.deleteBlock))
}
