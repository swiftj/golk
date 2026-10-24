package lk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/gen"
)

// BlocksService reads and changes single blocks of a page, the rows of a
// keyValue block, and the properties and rows of an infobox. Every write
// takes IfMatch and returns the document version after it, except the
// deletes, which return no version (read the document for the next one).
type BlocksService struct{ c *Client }

// ErrAnchor reports a block placement with the wrong number of anchors:
// Insert takes at most one of After, Before and Parent, and Move exactly
// one.
var ErrAnchor = errors.New("lk: set at most one of After, Before and Parent (exactly one for Move)")

// At places a block: after or before a sibling, or as the last child of a
// Parent container. Set one field.
type At struct {
	After  string
	Before string
	Parent string
}

func (a At) count() int {
	n := 0
	for _, s := range []string{a.After, a.Before, a.Parent} {
		if s != "" {
			n++
		}
	}
	return n
}

type atWire struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Parent string `json:"parent,omitempty"`
}

func blockParams(doc DocRef, blockID string) map[string]string {
	p := doc.params()
	p["blockId"] = blockID
	return p
}

// versioned sends a versioned write (or, with read, a read that reports the
// version) and returns the body without its version, and the version.
func (c *Client) versioned(ctx context.Context, op gen.Op, params map[string]string, body any, read bool, opts []CallOption) (*versionedBody, error) {
	var w versionedBody
	req := request{body: body, out: &w, versioned: !read, readsVersion: read}
	if err := c.call(ctx, op.Method, op.Path, params, req, opts); err != nil {
		return nil, err
	}
	return &w, nil
}

func decodeVersioned[T any](w *versionedBody, err error, decode func([]byte) (T, error)) (*Versioned[T], error) {
	if err != nil {
		return nil, err
	}
	v, err := decode(w.raw)
	if err != nil {
		return nil, fmt.Errorf("lk: decoding response: %w", err)
	}
	return &Versioned[T]{Value: v, Version: w.version}, nil
}

func decodeRow(raw []byte) (blocks.KVRow, error) {
	var r blocks.KVRow
	err := json.Unmarshal(raw, &r)
	return r, err
}

// Get returns the block with its subtree and the document version.
func (s *BlocksService) Get(ctx context.Context, doc DocRef, blockID string, opts ...CallOption) (*Versioned[blocks.Block], error) {
	w, err := s.c.versioned(ctx, gen.OpGetBlock, blockParams(doc, blockID), nil, true, opts)
	return decodeVersioned(w, err, blocks.Unmarshal)
}

// Insert inserts block at at, or appends it to the end of the document when
// at is empty. An id in the block is kept; one the document already uses
// fails with ErrDuplicateID. A blank document becomes a page.
func (s *BlocksService) Insert(ctx context.Context, doc DocRef, block blocks.Block, at At, opts ...CallOption) (*Versioned[blocks.Block], error) {
	if at.count() > 1 {
		return nil, ErrAnchor
	}
	body := struct {
		Block blocks.Block `json:"block"`
		atWire
	}{block, atWire(at)}
	w, err := s.c.versioned(ctx, gen.OpInsertBlock, doc.params(), body, false, opts)
	return decodeVersioned(w, err, blocks.Unmarshal)
}

// Update changes the block's own fields; its children stay as they are.
func (s *BlocksService) Update(ctx context.Context, doc DocRef, blockID string, p BlockPatch, opts ...CallOption) (*Versioned[blocks.Block], error) {
	w, err := s.c.versioned(ctx, gen.OpUpdateBlock, blockParams(doc, blockID), p, false, opts)
	return decodeVersioned(w, err, blocks.Unmarshal)
}

// Replace replaces the block and its subtree. The block keeps its id, and
// ids from the old subtree are kept when echoed. Optional fields left out
// are removed, except layout, width and the title fields (D-009, A-16).
func (s *BlocksService) Replace(ctx context.Context, doc DocRef, blockID string, block blocks.Block, opts ...CallOption) (*Versioned[blocks.Block], error) {
	w, err := s.c.versioned(ctx, gen.OpReplaceBlock, blockParams(doc, blockID), block, false, opts)
	return decodeVersioned(w, err, blocks.Unmarshal)
}

// Move moves the block and its subtree to exactly one anchor of at. Ids are
// kept.
func (s *BlocksService) Move(ctx context.Context, doc DocRef, blockID string, at At, opts ...CallOption) (*Versioned[blocks.Block], error) {
	if at.count() != 1 {
		return nil, ErrAnchor
	}
	w, err := s.c.versioned(ctx, gen.OpMoveBlock, blockParams(doc, blockID), atWire(at), false, opts)
	return decodeVersioned(w, err, blocks.Unmarshal)
}

// Delete removes the block and its subtree.
func (s *BlocksService) Delete(ctx context.Context, doc DocRef, blockID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteBlock.Method, gen.OpDeleteBlock.Path, blockParams(doc, blockID),
		request{versioned: true}, opts)
}

// ListRows returns the rows of a keyValue block and the document version.
func (s *BlocksService) ListRows(ctx context.Context, doc DocRef, blockID string, opts ...CallOption) (*VersionedList[blocks.KVRow], error) {
	return listRows(ctx, s.c, gen.OpListBlockRows, blockParams(doc, blockID), opts)
}

// CreateRow adds a row to a keyValue block.
func (s *BlocksService) CreateRow(ctx context.Context, doc DocRef, blockID string, r RowCreate, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	w, err := s.c.versioned(ctx, gen.OpCreateBlockRow, blockParams(doc, blockID), r, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// UpdateRow changes a row of a keyValue block.
func (s *BlocksService) UpdateRow(ctx context.Context, doc DocRef, blockID, rowID string, p RowPatch, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	params := blockParams(doc, blockID)
	params["rowId"] = rowID
	w, err := s.c.versioned(ctx, gen.OpUpdateBlockRow, params, p, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// DeleteRow removes a row from a keyValue block.
func (s *BlocksService) DeleteRow(ctx context.Context, doc DocRef, blockID, rowID string, opts ...CallOption) error {
	params := blockParams(doc, blockID)
	params["rowId"] = rowID
	return s.c.call(ctx, gen.OpDeleteBlockRow.Method, gen.OpDeleteBlockRow.Path, params, request{versioned: true}, opts)
}

func propertyParams(doc DocRef, blockID, propertyID string) map[string]string {
	p := blockParams(doc, blockID)
	p["propertyId"] = propertyID
	return p
}

// ListProperties returns the properties of an infobox and the document
// version.
func (s *BlocksService) ListProperties(ctx context.Context, doc DocRef, blockID string, opts ...CallOption) (*VersionedList[blocks.Property], error) {
	w, err := s.c.versioned(ctx, gen.OpListBlockProperties, blockParams(doc, blockID), nil, true, opts)
	if err != nil {
		return nil, err
	}
	var l struct {
		Data blocks.Properties `json:"data"`
	}
	if err := json.Unmarshal(w.raw, &l); err != nil {
		return nil, fmt.Errorf("lk: decoding infobox properties: %w", err)
	}
	return &VersionedList[blocks.Property]{Items: l.Data, Version: w.version}, nil
}

// GetProperty returns one property of an infobox.
func (s *BlocksService) GetProperty(ctx context.Context, doc DocRef, blockID, propertyID string, opts ...CallOption) (*Versioned[blocks.Property], error) {
	w, err := s.c.versioned(ctx, gen.OpGetBlockProperty, propertyParams(doc, blockID, propertyID), nil, true, opts)
	return decodeVersioned(w, err, blocks.UnmarshalProperty)
}

// CreateProperty adds a property to an infobox at place.
func (s *BlocksService) CreateProperty(ctx context.Context, doc DocRef, blockID string, p blocks.Property, place Place, opts ...CallOption) (*Versioned[blocks.Property], error) {
	body, err := propertyCreateBody(p, place)
	if err != nil {
		return nil, err
	}
	w, err := s.c.versioned(ctx, gen.OpCreateBlockProperty, blockParams(doc, blockID), body, false, opts)
	return decodeVersioned(w, err, blocks.UnmarshalProperty)
}

// UpdateProperty merges p into a property of an infobox.
func (s *BlocksService) UpdateProperty(ctx context.Context, doc DocRef, blockID, propertyID string, p PropertyPatch, opts ...CallOption) (*Versioned[blocks.Property], error) {
	w, err := s.c.versioned(ctx, gen.OpUpdateBlockProperty, propertyParams(doc, blockID, propertyID), p, false, opts)
	return decodeVersioned(w, err, blocks.UnmarshalProperty)
}

// DeleteProperty removes a property, and its rows, from an infobox.
func (s *BlocksService) DeleteProperty(ctx context.Context, doc DocRef, blockID, propertyID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteBlockProperty.Method, gen.OpDeleteBlockProperty.Path,
		propertyParams(doc, blockID, propertyID), request{versioned: true}, opts)
}

// ListPropertyRows returns the rows of a keyValue property in an infobox.
func (s *BlocksService) ListPropertyRows(ctx context.Context, doc DocRef, blockID, propertyID string, opts ...CallOption) (*VersionedList[blocks.KVRow], error) {
	return listRows(ctx, s.c, gen.OpListBlockPropertyRows, propertyParams(doc, blockID, propertyID), opts)
}

// CreatePropertyRow adds a row to a keyValue property in an infobox.
func (s *BlocksService) CreatePropertyRow(ctx context.Context, doc DocRef, blockID, propertyID string, r RowCreate, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	w, err := s.c.versioned(ctx, gen.OpCreateBlockPropertyRow, propertyParams(doc, blockID, propertyID), r, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// UpdatePropertyRow changes a row of a keyValue property in an infobox.
func (s *BlocksService) UpdatePropertyRow(ctx context.Context, doc DocRef, blockID, propertyID, rowID string, p RowPatch, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	params := propertyParams(doc, blockID, propertyID)
	params["rowId"] = rowID
	w, err := s.c.versioned(ctx, gen.OpUpdateBlockPropertyRow, params, p, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// DeletePropertyRow removes a row from a keyValue property in an infobox.
func (s *BlocksService) DeletePropertyRow(ctx context.Context, doc DocRef, blockID, propertyID, rowID string, opts ...CallOption) error {
	params := propertyParams(doc, blockID, propertyID)
	params["rowId"] = rowID
	return s.c.call(ctx, gen.OpDeleteBlockPropertyRow.Method, gen.OpDeleteBlockPropertyRow.Path, params,
		request{versioned: true}, opts)
}

func listRows(ctx context.Context, c *Client, op gen.Op, params map[string]string, opts []CallOption) (*VersionedList[blocks.KVRow], error) {
	w, err := c.versioned(ctx, op, params, nil, true, opts)
	if err != nil {
		return nil, err
	}
	var l struct {
		Data []blocks.KVRow `json:"data"`
	}
	if err := json.Unmarshal(w.raw, &l); err != nil {
		return nil, fmt.Errorf("lk: decoding rows: %w", err)
	}
	return &VersionedList[blocks.KVRow]{Items: l.Data, Version: w.version}, nil
}

// RowCreate is the body of the CreateRow operations. Set Key and the value,
// as Content runs or plain Text; when both are set Content wins. With
// neither Before nor After the row is appended.
type RowCreate struct {
	Key     string           `json:"key"`
	Content blocks.Runs      `json:"content,omitzero"`
	Text    Optional[string] `json:"text,omitzero"`
	Before  Optional[string] `json:"before,omitzero"`
	After   Optional[string] `json:"after,omitzero"`
}

// RowPatch is the body of the UpdateRow operations. Only the fields set are
// sent. Content set to Null clears the value; Text cannot be null.
type RowPatch struct {
	Key     Optional[string]      `json:"key,omitzero"`
	Content Nullable[blocks.Runs] `json:"content,omitzero"`
	Text    Optional[string]      `json:"text,omitzero"`
}

// Place positions a new property next to another; with neither set it is
// appended.
type Place struct {
	Before string
	After  string
}

// propertyCreateBody is the property with its placement and without rank,
// which a create body may not carry (the server answers 400).
func propertyCreateBody(p blocks.Property, place Place) (json.RawMessage, error) {
	if p == nil {
		return nil, errors.New("lk: nil property")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	delete(fields, "rank")
	for k, v := range map[string]string{"before": place.Before, "after": place.After} {
		if v != "" {
			fields[k], _ = json.Marshal(v)
		}
	}
	return json.Marshal(fields)
}
