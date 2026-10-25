package lk

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/swiftj/golk/lk/internal/gen"
)

// BoardsService reads and writes board documents: tldraw 3.9 pages, shapes,
// bindings and assets. Every write takes IfMatch, is atomic and returns the
// board's new version (G§boards).
type BoardsService struct{ c *Client }

// MaxBoardBatch is the most shapes, bindings, assets or ids one board write
// takes of each.
const MaxBoardBatch = 500

// ErrBoardBatchTooLarge is returned, before any request, by a board write
// with more than MaxBoardBatch records of one kind.
var ErrBoardBatchTooLarge = errors.New("lk: a board write takes at most 500 shapes, bindings, assets or ids of each")

// The records below are tldraw's, as the server stores them. Props and Meta
// hold each value exactly as read, so a record read and sent back (see the
// Input methods) is unchanged.

// BoardPage is a tldraw page record. The app shows one page per board.
type BoardPage struct {
	ID    string                     `json:"id"`
	Name  string                     `json:"name"`
	Index string                     `json:"index"`
	Meta  map[string]json.RawMessage `json:"meta"`
}

// BoardShape is a tldraw shape record. Type is arrow, bookmark,
// content-block, draw, element, embed, frame, geo, group, highlight, image,
// line, note, text or video; Props are that type's props as tldraw defines
// them.
type BoardShape struct {
	ID   string  `json:"id"`
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	// Rotation is in radians.
	Rotation float64 `json:"rotation"`
	// Index orders the shape among its parent's children; higher draws
	// above.
	Index string `json:"index"`
	// ParentID is the page (page:…) or the frame or group (shape:…) that
	// contains the shape.
	ParentID string                     `json:"parentId"`
	IsLocked bool                       `json:"isLocked"`
	Opacity  float64                    `json:"opacity"`
	Props    map[string]json.RawMessage `json:"props"`
	Meta     map[string]json.RawMessage `json:"meta"`
}

// BoardBinding is a tldraw binding record: an arrow (FromID) attached at one
// terminal to a shape (ToID).
type BoardBinding struct {
	ID     string                     `json:"id"`
	Type   string                     `json:"type"`
	FromID string                     `json:"fromId"`
	ToID   string                     `json:"toId"`
	Props  map[string]json.RawMessage `json:"props"`
	Meta   map[string]json.RawMessage `json:"meta"`
}

// BoardAssetType is the kind of a board asset.
type BoardAssetType string

// Board asset types.
const (
	BoardAssetImage    BoardAssetType = "image"
	BoardAssetVideo    BoardAssetType = "video"
	BoardAssetBookmark BoardAssetType = "bookmark"
)

// BoardAsset is a tldraw asset record. Image and video shapes reference one
// by props.assetId. An image src is the url of an image asset of the same
// project.
type BoardAsset struct {
	ID    string                     `json:"id"`
	Type  BoardAssetType             `json:"type"`
	Props map[string]json.RawMessage `json:"props"`
	Meta  map[string]json.RawMessage `json:"meta"`
}

// BoardSchema is the serialized tldraw schema that writes are validated
// against.
type BoardSchema struct {
	SchemaVersion float64            `json:"schemaVersion"`
	Sequences     map[string]float64 `json:"sequences"`
}

// Board is every record of a board, with its version. Pass Version to
// IfMatch on the next write.
type Board struct {
	Schema   BoardSchema    `json:"schema"`
	Pages    []BoardPage    `json:"pages"`
	Shapes   []BoardShape   `json:"shapes"`
	Bindings []BoardBinding `json:"bindings"`
	Assets   []BoardAsset   `json:"assets"`
	Version  string         `json:"version"`
}

func (b *Board) setVersionIfEmpty(v string) {
	if b.Version == "" {
		b.Version = v
	}
}

// BoardShapeInput is a shape to create. Only Type is required; every field
// not set takes the app's default, as does every prop not given. A nil
// Props or Meta is not sent.
type BoardShapeInput struct {
	// ID is an optional shape:… id; the server mints one when omitted.
	ID       Optional[string]           `json:"id,omitzero"`
	Type     string                     `json:"type"`
	X        Optional[float64]          `json:"x,omitzero"`
	Y        Optional[float64]          `json:"y,omitzero"`
	Rotation Optional[float64]          `json:"rotation,omitzero"`
	Index    Optional[string]           `json:"index,omitzero"`
	ParentID Optional[string]           `json:"parentId,omitzero"`
	IsLocked Optional[bool]             `json:"isLocked,omitzero"`
	Opacity  Optional[float64]          `json:"opacity,omitzero"`
	Props    map[string]json.RawMessage `json:"props,omitempty"`
	Meta     map[string]json.RawMessage `json:"meta,omitempty"`
}

// BoardBindingInput is a binding to create. FromID names an arrow and ToID
// the shape its terminal attaches to; either may be created in the same
// call. An arrow binding needs props.terminal ("start" or "end").
type BoardBindingInput struct {
	ID     Optional[string]           `json:"id,omitzero"`
	Type   string                     `json:"type"`
	FromID string                     `json:"fromId"`
	ToID   string                     `json:"toId"`
	Props  map[string]json.RawMessage `json:"props,omitempty"`
	Meta   map[string]json.RawMessage `json:"meta,omitempty"`
}

// BoardAssetInput is an asset to create. Props must hold every prop of the
// asset type.
type BoardAssetInput struct {
	ID    Optional[string]           `json:"id,omitzero"`
	Type  BoardAssetType             `json:"type"`
	Props map[string]json.RawMessage `json:"props"`
	Meta  map[string]json.RawMessage `json:"meta,omitempty"`
}

// BoardShapePatch changes one shape. Fields set replace the stored ones;
// Props and Meta merge one level deep, as tldraw's updateShapes does.
type BoardShapePatch struct {
	ID string `json:"id"`
	// Type, when set, must equal the shape's type.
	Type     Optional[string]           `json:"type,omitzero"`
	X        Optional[float64]          `json:"x,omitzero"`
	Y        Optional[float64]          `json:"y,omitzero"`
	Rotation Optional[float64]          `json:"rotation,omitzero"`
	Index    Optional[string]           `json:"index,omitzero"`
	ParentID Optional[string]           `json:"parentId,omitzero"`
	IsLocked Optional[bool]             `json:"isLocked,omitzero"`
	Opacity  Optional[float64]          `json:"opacity,omitzero"`
	Props    map[string]json.RawMessage `json:"props,omitempty"`
	Meta     map[string]json.RawMessage `json:"meta,omitempty"`
}

// Input returns the shape as a BoardShapeInput with every field set, for
// sending it back with ReplaceContent.
func (s BoardShape) Input() BoardShapeInput {
	return BoardShapeInput{
		ID: Some(s.ID), Type: s.Type, X: Some(s.X), Y: Some(s.Y), Rotation: Some(s.Rotation),
		Index: Some(s.Index), ParentID: Some(s.ParentID), IsLocked: Some(s.IsLocked), Opacity: Some(s.Opacity),
		Props: s.Props, Meta: s.Meta,
	}
}

// Input returns the binding as a BoardBindingInput with every field set.
func (b BoardBinding) Input() BoardBindingInput {
	return BoardBindingInput{ID: Some(b.ID), Type: b.Type, FromID: b.FromID, ToID: b.ToID, Props: b.Props, Meta: b.Meta}
}

// Input returns the asset as a BoardAssetInput with every field set.
func (a BoardAsset) Input() BoardAssetInput {
	return BoardAssetInput{ID: Some(a.ID), Type: a.Type, Props: a.Props, Meta: a.Meta}
}

// BoardRecords are the records Boards.CreateShapes and
// Boards.ReplaceContent send. Bindings and Assets are sent only when
// non-nil.
type BoardRecords struct {
	Shapes   []BoardShapeInput   `json:"shapes"`
	Bindings []BoardBindingInput `json:"bindings,omitempty"`
	Assets   []BoardAssetInput   `json:"assets,omitempty"`
}

func (r BoardRecords) body() (BoardRecords, error) {
	if len(r.Shapes) > MaxBoardBatch || len(r.Bindings) > MaxBoardBatch || len(r.Assets) > MaxBoardBatch {
		return r, ErrBoardBatchTooLarge
	}
	if r.Shapes == nil {
		r.Shapes = []BoardShapeInput{}
	}
	return r, nil
}

// BoardCreated is the result of Boards.CreateShapes: the records as stored,
// with every default filled in, and the board's new version.
type BoardCreated struct {
	Shapes   []BoardShape   `json:"shapes"`
	Bindings []BoardBinding `json:"bindings"`
	Assets   []BoardAsset   `json:"assets"`
	Version  string         `json:"version"`
}

func (b *BoardCreated) setVersionIfEmpty(v string) {
	if b.Version == "" {
		b.Version = v
	}
}

// BoardDeleted is the result of Boards.DeleteShapes.
type BoardDeleted struct {
	// ShapeIDs are every shape deleted: those named and the shapes they
	// contained.
	ShapeIDs []string `json:"shapeIds"`
	// BindingIDs are the bindings deleted because one of their ends was.
	BindingIDs []string `json:"bindingIds"`
	Version    string   `json:"version"`
}

func (b *BoardDeleted) setVersionIfEmpty(v string) {
	if b.Version == "" {
		b.Version = v
	}
}

// boardShapes receives listBoardShapes ("data") and updateBoardShapes
// ("shapes").
type boardShapes struct {
	Data    []BoardShape `json:"data"`
	Shapes  []BoardShape `json:"shapes"`
	Version string       `json:"version"`
}

func (b *boardShapes) setVersionIfEmpty(v string) {
	if b.Version == "" {
		b.Version = v
	}
}

// BoardShapeListParams filters Boards.ListShapes.
type BoardShapeListParams struct {
	// PageID lists only the shapes on this page; empty lists every shape.
	PageID string
}

// GetContent returns every page, shape, binding and asset of the board,
// with its schema and version. A blank document reads as an empty board
// and is not converted.
func (s *BoardsService) GetContent(ctx context.Context, doc DocRef, opts ...CallOption) (*Board, error) {
	var b Board
	if err := s.c.call(ctx, gen.OpGetBoardContent.Method, gen.OpGetBoardContent.Path, doc.params(),
		request{out: &b, readsVersion: true}, opts); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListShapes returns the board's shapes ordered by index, or only those on
// p.PageID, with the board version. It is not paginated.
func (s *BoardsService) ListShapes(ctx context.Context, doc DocRef, p BoardShapeListParams, opts ...CallOption) (*VersionedList[BoardShape], error) {
	var w boardShapes
	req := request{out: &w, readsVersion: true}
	if p.PageID != "" {
		req.query = url.Values{"pageId": {p.PageID}}
	}
	if err := s.c.call(ctx, gen.OpListBoardShapes.Method, gen.OpListBoardShapes.Path, doc.params(), req, opts); err != nil {
		return nil, err
	}
	return &VersionedList[BoardShape]{Items: w.Data, Version: w.Version}, nil
}

// CreateShapes creates r's shapes, bindings and assets. The first create on
// a blank document makes it a board.
func (s *BoardsService) CreateShapes(ctx context.Context, doc DocRef, r BoardRecords, opts ...CallOption) (*BoardCreated, error) {
	body, err := r.body()
	if err != nil {
		return nil, err
	}
	var out BoardCreated
	if err := s.c.call(ctx, gen.OpCreateBoardShapes.Method, gen.OpCreateBoardShapes.Path, doc.params(),
		request{body: body, out: &out, versioned: true}, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateShapes applies the patches and returns the updated shapes with the
// board's new version.
func (s *BoardsService) UpdateShapes(ctx context.Context, doc DocRef, patches []BoardShapePatch, opts ...CallOption) (*VersionedList[BoardShape], error) {
	if len(patches) > MaxBoardBatch {
		return nil, ErrBoardBatchTooLarge
	}
	body := struct {
		Shapes []BoardShapePatch `json:"shapes"`
	}{patches}
	var w boardShapes
	if err := s.c.call(ctx, gen.OpUpdateBoardShapes.Method, gen.OpUpdateBoardShapes.Path, doc.params(),
		request{body: body, out: &w, versioned: true}, opts); err != nil {
		return nil, err
	}
	return &VersionedList[BoardShape]{Items: w.Shapes, Version: w.Version}, nil
}

// DeleteShapes deletes the shapes named by ids, the shapes they contain and
// the bindings that lose an end.
func (s *BoardsService) DeleteShapes(ctx context.Context, doc DocRef, ids []string, opts ...CallOption) (*BoardDeleted, error) {
	if len(ids) > MaxBoardBatch {
		return nil, ErrBoardBatchTooLarge
	}
	body := struct {
		IDs []string `json:"ids"`
	}{ids}
	var out BoardDeleted
	if err := s.c.call(ctx, gen.OpDeleteBoardShapes.Method, gen.OpDeleteBoardShapes.Path, doc.params(),
		request{body: body, out: &out, versioned: true}, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReplaceContent deletes every shape, binding and asset of the board and
// stores r's instead. Pages are kept. It returns the whole board.
func (s *BoardsService) ReplaceContent(ctx context.Context, doc DocRef, r BoardRecords, opts ...CallOption) (*Board, error) {
	body, err := r.body()
	if err != nil {
		return nil, err
	}
	var b Board
	if err := s.c.call(ctx, gen.OpReplaceBoardContent.Method, gen.OpReplaceBoardContent.Path, doc.params(),
		request{body: body, out: &b, versioned: true}, opts); err != nil {
		return nil, err
	}
	return &b, nil
}
