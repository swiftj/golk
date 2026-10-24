package lk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// DocRef names one document of a resource.
type DocRef struct {
	ResourceID string
	DocumentID string
}

func (d DocRef) params() map[string]string {
	return map[string]string{"resourceId": d.ResourceID, "documentId": d.DocumentID}
}

// DocumentsService manages a resource's documents (its tabs) and page
// content.
type DocumentsService struct{ c *Client }

// List returns every document of the resource on one page. Hidden
// documents are listed only to admins and owners. Order by Rank to match
// the app.
func (s *DocumentsService) List(ctx context.Context, resourceID string, opts ...CallOption) (*Page[Document], error) {
	var w gen.DocumentList
	if err := s.c.call(ctx, gen.OpListDocuments.Method, gen.OpListDocuments.Path,
		map[string]string{"resourceId": resourceID}, request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Document, len(w.Data))
	for i, d := range w.Data {
		out[i] = documentFromWire(d)
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// All iterates over the resource's documents.
func (s *DocumentsService) All(ctx context.Context, resourceID string, opts ...CallOption) iter.Seq2[Document, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Document], error) { return s.List(ctx, resourceID, opts...) })
}

// Get returns one document, without its content.
func (s *DocumentsService) Get(ctx context.Context, doc DocRef, opts ...CallOption) (*Document, error) {
	return s.document(ctx, gen.OpGetDocument, doc.params(), nil, opts)
}

// DocumentCreate is the body of Documents.Create.
type DocumentCreate struct {
	Name string
	Type DocumentType
	// IsHidden and IsFullWidth default to false.
	IsHidden    Optional[bool]
	IsFullWidth Optional[bool]
	// CalendarID sets a timeline (time) document's calendar.
	CalendarID Optional[string]
}

// Create adds an empty document after the resource's existing ones. A new
// resource already has a blank document; write content to that one first.
func (s *DocumentsService) Create(ctx context.Context, resourceID string, d DocumentCreate, opts ...CallOption) (*Document, error) {
	body := gen.DocumentCreateParams{
		Name: d.Name, Type: gen.DocumentCreateParamsType(d.Type),
		IsHidden: d.IsHidden, IsFullWidth: d.IsFullWidth, CalendarID: d.CalendarID,
	}
	return s.document(ctx, gen.OpCreateDocument, map[string]string{"resourceId": resourceID}, body, opts)
}

// DocumentUpdate is the body of Documents.Update. Only the fields that are
// set are sent and changed; content is not touched.
type DocumentUpdate struct {
	Name Optional[string]
	// Rank moves the document among the resource's tabs.
	Rank        Optional[string]
	IsHidden    Optional[bool]
	IsFullWidth Optional[bool]
	// CalendarID changes a timeline (time) document's calendar.
	CalendarID Optional[string]
	// Sources replaces the timelines a timeline (time) document chains, in
	// order (at most 100). Some([]DocRef{}) removes them all.
	Sources Optional[[]DocRef]
	// MapImage sets or replaces a map document's image. Features keep their
	// Pos, so after a replacement with an image of another size they sit
	// over other pixels. A blank document becomes a map.
	MapImage Optional[MapImageRef]
}

// MapImageRef names the image asset to show as a map document's image.
type MapImageRef struct {
	AssetID string
}

// MapImage is a map document's image and the coordinate frame its features'
// positions are in (see PixelToPos).
type MapImage struct {
	// AssetID is empty when no image asset the caller can read has URL, or
	// the map is a legacy tiled map.
	AssetID string
	// URL is empty for a legacy tiled map.
	URL string
	// Width and Height are the image size in pixels.
	Width, Height int
	// Bounds is [[south, west], [north, east]], the [lat, lng] corners the
	// image covers. North and west are 0 at the image's top-left corner;
	// south is negative and east is positive.
	Bounds [2][2]float64
	// Tiles is nil when the map has no image id.
	Tiles *MapTiles
}

// MapTiles describes a map image's 256px tiles.
type MapTiles struct {
	// URLTemplate has {z}, {x} and {y} placeholders.
	URLTemplate string
	// MaxNativeZoom is the highest zoom with native tiles.
	MaxNativeZoom int
}

// Update changes the document's fields and returns it.
func (s *DocumentsService) Update(ctx context.Context, doc DocRef, u DocumentUpdate, opts ...CallOption) (*Document, error) {
	body := gen.DocumentUpdateParams{
		Name: u.Name, Rank: u.Rank, IsHidden: u.IsHidden, IsFullWidth: u.IsFullWidth, CalendarID: u.CalendarID,
	}
	if src, ok := u.Sources.Get(); ok {
		// Never nil, so Some([]DocRef{}) is sent as [] and clears the chain.
		w := make([]gen.TimelineSource, len(src))
		for i, d := range src {
			w[i] = gen.TimelineSource{ResourceID: d.ResourceID, DocumentID: d.DocumentID}
		}
		body.Sources = opt.Some(w)
	}
	if m, ok := u.MapImage.Get(); ok {
		body.MapImage = opt.Some(gen.MapImageParams{AssetID: m.AssetID})
	}
	return s.document(ctx, gen.OpUpdateDocument, doc.params(), body, opts)
}

// Delete moves the document and its content to the project's trash.
func (s *DocumentsService) Delete(ctx context.Context, doc DocRef, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteDocument.Method, gen.OpDeleteDocument.Path, doc.params(), request{}, opts)
}

// Restore brings a deleted document back in its old place. A document that
// is not deleted fails with ErrNotDeleted; one whose resource is deleted
// fails with ErrParentDeleted (restore the resource instead).
func (s *DocumentsService) Restore(ctx context.Context, doc DocRef, opts ...CallOption) (*Document, error) {
	return s.document(ctx, gen.OpRestoreDocument, doc.params(), nil, opts)
}

func (s *DocumentsService) document(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Document, error) {
	var w gen.Document
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	// documentFromWire drops a malformed mapImage; a single-document
	// response reports it instead.
	if m, ok := w.MapImage.Get(); ok {
		if _, err := mapImageFromWire(m); err != nil {
			return nil, fmt.Errorf("lk: decoding document %s mapImage: %w", w.ID, err)
		}
	}
	d := documentFromWire(w)
	return &d, nil
}

func mapImageFromWire(w gen.MapImage) (*MapImage, error) {
	if len(w.Bounds) != 2 {
		return nil, fmt.Errorf("bounds: want 2 corners, got %d", len(w.Bounds))
	}
	m := &MapImage{AssetID: w.AssetID.Or(""), URL: w.URL.Or(""), Width: int(w.Width), Height: int(w.Height)}
	for i, raw := range w.Bounds {
		if err := json.Unmarshal(raw, &m.Bounds[i]); err != nil {
			return nil, fmt.Errorf("bounds[%d]: %w", i, err)
		}
	}
	if t, ok := w.Tiles.Get(); ok {
		m.Tiles = &MapTiles{URLTemplate: t.URLTemplate, MaxNativeZoom: int(t.MaxNativeZoom)}
	}
	return m, nil
}

// DocumentBlocks is a page's content.
type DocumentBlocks struct {
	ID string
	// Version is the document version; pass it to IfMatch on the next write.
	Version string
	// Schema is the block model version (1).
	Schema int
	// Text is the page's plain text, one line per text-bearing block
	// (blocks.PlainText derives the same).
	Text   string
	Blocks blocks.List
}

type documentBlocksWire struct {
	ID      string      `json:"id"`
	Version string      `json:"version"`
	Schema  json.Number `json:"schema"`
	Text    string      `json:"text"`
	Blocks  blocks.List `json:"blocks"`
}

func (w *documentBlocksWire) setVersionIfEmpty(v string) {
	if w.Version == "" {
		w.Version = v
	}
}

func (w *documentBlocksWire) public() *DocumentBlocks {
	schema, _ := w.Schema.Int64()
	return &DocumentBlocks{ID: w.ID, Version: w.Version, Schema: int(schema), Text: w.Text, Blocks: w.Blocks}
}

// GetBlocks returns the page content. Reading a blank document returns an
// empty page and leaves it blank.
func (s *DocumentsService) GetBlocks(ctx context.Context, doc DocRef, opts ...CallOption) (*DocumentBlocks, error) {
	var w documentBlocksWire
	if err := s.c.call(ctx, gen.OpGetDocumentBlocks.Method, gen.OpGetDocumentBlocks.Path, doc.params(),
		request{out: &w, readsVersion: true}, opts); err != nil {
		return nil, err
	}
	return w.public(), nil
}

// ReplaceBlocks replaces the whole page and returns it with ids and the new
// version. It takes IfMatch. A blank document becomes a page.
//
// Blocks written with an id keep it. Leaving out a block deletes it, which
// includes an UnsupportedBlock: echo those by id. A replace that omits a
// block's optional fields removes most of them; only layout, width and the
// title fields are kept (DECISIONS.md D-009, A-16). To change part of a page
// without touching the rest, use the Blocks operations.
func (s *DocumentsService) ReplaceBlocks(ctx context.Context, doc DocRef, content blocks.List, opts ...CallOption) (*DocumentBlocks, error) {
	body := struct {
		Blocks blocks.List `json:"blocks"`
	}{content}
	var w documentBlocksWire
	if err := s.c.call(ctx, gen.OpReplaceDocumentBlocks.Method, gen.OpReplaceDocumentBlocks.Path, doc.params(),
		request{body: body, out: &w, versioned: true}, opts); err != nil {
		return nil, err
	}
	return w.public(), nil
}

// BlockIDsResult is the result of Documents.AssignBlockIDs.
type BlockIDsResult struct {
	// Assigned is how many blocks lacked an id.
	Assigned int
	Version  string
}

// AssignBlockIDs gives an id to every block that lacks one. Documents
// written in the app before block ids existed lack them; run it once before
// addressing their blocks. It does not take IfMatch.
func (s *DocumentsService) AssignBlockIDs(ctx context.Context, doc DocRef, opts ...CallOption) (*BlockIDsResult, error) {
	var w versionedBody
	if err := s.c.call(ctx, gen.OpAssignBlockIDs.Method, gen.OpAssignBlockIDs.Path, doc.params(),
		request{out: &w, readsVersion: true}, opts); err != nil {
		return nil, err
	}
	var r struct {
		Assigned int `json:"assigned"`
	}
	if err := json.Unmarshal(w.raw, &r); err != nil {
		return nil, fmt.Errorf("lk: decoding assignBlockIds response: %w", err)
	}
	return &BlockIDsResult{Assigned: r.Assigned, Version: w.version}, nil
}

func documentFromWire(d gen.Document) Document {
	doc := Document{
		ID: d.ID, Name: d.Name, Type: DocumentType(d.Type), Rank: d.Rank,
		IsHidden: d.IsHidden, IsFullWidth: d.IsFullWidth.Or(false), CalendarID: d.CalendarID.Or(""),
		CreatedAt: utc(d.CreatedAt.Or(time.Time{})), UpdatedAt: utc(d.UpdatedAt.Or(time.Time{})),
		DeletedAt: utc(d.DeletedAt.Or(time.Time{})),
		Sources:   sourcesFromWire(d.Sources.Or(nil)),
	}
	if m, ok := d.MapImage.Get(); ok {
		if img, err := mapImageFromWire(m); err == nil {
			doc.MapImage = img
		}
	}
	return doc
}

func sourcesFromWire(w []gen.TimelineSource) []DocRef {
	if w == nil {
		return nil
	}
	out := make([]DocRef, len(w))
	for i, s := range w {
		out[i] = DocRef{ResourceID: s.ResourceID, DocumentID: s.DocumentID}
	}
	return out
}
