package lk

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// geoJSON is a shape as the server stores it (the sandbox, A-39), with a
// prop the SDK doesn't model and a number that must not be reformatted.
const geoJSON = `{"id":"shape:a","typeName":"shape","type":"geo","x":0,"y":0,"rotation":0,"index":"a1","parentId":"page:page","isLocked":false,"opacity":1,` +
	`"props":{"geo":"rectangle","h":50,"scale":1.0,"text":"Alpha","w":100,"zz":{"new":true}},"meta":{}}`

const boardJSON = `{"schema":{"schemaVersion":2,"sequences":{"com.tldraw.shape":4}},` +
	`"pages":[{"id":"page:page","typeName":"page","name":"Page 1","index":"a1","meta":{}}],` +
	`"shapes":[` + geoJSON + `],` +
	`"bindings":[{"id":"binding:x","typeName":"binding","type":"arrow","fromId":"shape:ab","toId":"shape:a","props":{"terminal":"start"},"meta":{}}],` +
	`"assets":[{"id":"asset:i","typeName":"asset","type":"image","props":{"src":"https://assets.example/p.png","w":4},"meta":{}}],` +
	`"version":"bv1"}`

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestBoards(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-BOARD-01 get content", reply: testutil.JSON(200, boardJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Boards.GetContent(ctx, doc) },
			method: "GET", path: pD + "/board",
			check: func(t *testing.T, v any) {
				b := v.(*Board)
				if b.Version != "bv1" || len(b.Pages) != 1 || b.Pages[0].Name != "Page 1" || len(b.Shapes) != 1 ||
					b.Shapes[0].Type != "geo" || b.Shapes[0].ParentID != "page:page" || b.Shapes[0].Opacity != 1 ||
					len(b.Bindings) != 1 || b.Bindings[0].FromID != "shape:ab" || len(b.Assets) != 1 || b.Assets[0].Type != BoardAssetImage ||
					b.Schema.Sequences["com.tldraw.shape"] != 4 {
					t.Errorf("board = %+v", b)
				}
			}},
		{name: "C-BOARD-01 get content takes the version from the ETag", reply: etag(testutil.JSON(200, `{"schema":{"schemaVersion":2,"sequences":{}},"pages":[],"shapes":[],"bindings":[],"assets":[]}`), "bv7"),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Boards.GetContent(ctx, doc) },
			method: "GET", path: pD + "/board",
			check: func(t *testing.T, v any) {
				if b := v.(*Board); b.Version != "bv7" || len(b.Shapes) != 0 {
					t.Errorf("board = %+v", b)
				}
			}},
		{name: "C-BOARD-01 list shapes", reply: testutil.JSON(200, `{"data":[`+geoJSON+`],"version":"bv1"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.ListShapes(ctx, doc, BoardShapeListParams{})
			},
			method: "GET", path: pD + "/board/shapes",
			check: func(t *testing.T, v any) {
				l := v.(*VersionedList[BoardShape])
				if l.Version != "bv1" || len(l.Items) != 1 || l.Items[0].ID != "shape:a" {
					t.Errorf("list = %+v", l)
				}
			}},
		{name: "C-BOARD-01 list shapes on one page", reply: etag(testutil.JSON(200, `{"data":[]}`), "bv2"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.ListShapes(ctx, doc, BoardShapeListParams{PageID: "page:page"})
			},
			method: "GET", path: pD + "/board/shapes", query: "pageId=page%3Apage",
			check: func(t *testing.T, v any) {
				if l := v.(*VersionedList[BoardShape]); l.Version != "bv2" {
					t.Errorf("Version = %q, want bv2 from the ETag", l.Version)
				}
			}},
		{name: "C-BOARD-02 create sends only what is given", reply: testutil.JSON(201, `{"shapes":[`+geoJSON+`],"bindings":[],"assets":[],"version":"bv2"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.CreateShapes(ctx, doc, BoardRecords{Shapes: []BoardShapeInput{
					{ID: Some("shape:a"), Type: "geo", X: Some(0.0), Props: map[string]json.RawMessage{"text": raw(`"Alpha"`), "w": raw(`100`)}},
					{Type: "note"},
				}}, IfMatch("bv1"))
			},
			method: "POST", path: pD + "/board/shapes", ifMatch: "bv1", keyed: true,
			body: `{"shapes":[{"id":"shape:a","type":"geo","x":0,"props":{"text":"Alpha","w":100}},{"type":"note"}]}`,
			check: func(t *testing.T, v any) {
				if b := v.(*BoardCreated); b.Version != "bv2" || len(b.Shapes) != 1 {
					t.Errorf("created = %+v", b)
				}
			}},
		{name: "C-BOARD-02 create with bindings and assets", reply: etag(testutil.JSON(201, `{"shapes":[],"bindings":[],"assets":[]}`), "bv3"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.CreateShapes(ctx, doc, BoardRecords{
					Bindings: []BoardBindingInput{{Type: "arrow", FromID: "shape:ab", ToID: "shape:a", Props: map[string]json.RawMessage{"terminal": raw(`"end"`)}}},
					Assets:   []BoardAssetInput{{Type: BoardAssetImage, Props: map[string]json.RawMessage{"src": raw(`"https://assets.example/p.png"`)}}},
				}, IfMatch("bv2"))
			},
			method: "POST", path: pD + "/board/shapes", ifMatch: "bv2", keyed: true,
			body: `{"shapes":[],"bindings":[{"type":"arrow","fromId":"shape:ab","toId":"shape:a","props":{"terminal":"end"}}],"assets":[{"type":"image","props":{"src":"https://assets.example/p.png"}}]}`,
			check: func(t *testing.T, v any) {
				if b := v.(*BoardCreated); b.Version != "bv3" {
					t.Errorf("Version = %q, want bv3 from the ETag", b.Version)
				}
			}},
		{name: "C-BOARD-03 update sends each id and only the fields set", reply: testutil.JSON(200, `{"shapes":[`+geoJSON+`],"version":"bv4"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.UpdateShapes(ctx, doc, []BoardShapePatch{
					{ID: "shape:a", Y: Some(60.0), Props: map[string]json.RawMessage{"color": raw(`"blue"`)}},
					{ID: "shape:b", IsLocked: Some(false)},
				}, IfMatch("bv3"))
			},
			method: "PATCH", path: pD + "/board/shapes", ifMatch: "bv3",
			body: `{"shapes":[{"id":"shape:a","y":60,"props":{"color":"blue"}},{"id":"shape:b","isLocked":false}]}`,
			check: func(t *testing.T, v any) {
				if l := v.(*VersionedList[BoardShape]); l.Version != "bv4" || len(l.Items) != 1 {
					t.Errorf("updated = %+v", l)
				}
			}},
		{name: "C-BOARD-04 delete", reply: testutil.JSON(200, `{"shapeIds":["shape:f","shape:in-f"],"bindingIds":["binding:x"],"version":"bv5"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.DeleteShapes(ctx, doc, []string{"shape:f"}, IfMatch("bv4"))
			},
			method: "POST", path: pD + "/board/shapes/delete", ifMatch: "bv4", keyed: true,
			body: `{"ids":["shape:f"]}`,
			check: func(t *testing.T, v any) {
				d := v.(*BoardDeleted)
				if d.Version != "bv5" || !slices.Equal(d.ShapeIDs, []string{"shape:f", "shape:in-f"}) || !slices.Equal(d.BindingIDs, []string{"binding:x"}) {
					t.Errorf("deleted = %+v", d)
				}
			}},
		{name: "C-BOARD-05 replace sends shapes even when empty", reply: testutil.JSON(200, boardJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Boards.ReplaceContent(ctx, doc, BoardRecords{}, IfMatch("bv5"))
			},
			method: "PUT", path: pD + "/board", ifMatch: "bv5", body: `{"shapes":[]}`,
			check: func(t *testing.T, v any) {
				if b := v.(*Board); b.Version != "bv1" || len(b.Pages) != 1 {
					t.Errorf("board = %+v", b)
				}
			}},
	})
}

// A board read and replaced with its own records sends them back unchanged:
// every field, the props the SDK doesn't model, and numbers as read.
func TestBoards_C_BOARD_06_RoundTrip(t *testing.T) {
	var b Board
	if err := json.Unmarshal([]byte(boardJSON), &b); err != nil {
		t.Fatal(err)
	}
	r := BoardRecords{}
	for _, s := range b.Shapes {
		r.Shapes = append(r.Shapes, s.Input())
	}
	for _, x := range b.Bindings {
		r.Bindings = append(r.Bindings, x.Input())
	}
	for _, a := range b.Assets {
		r.Assets = append(r.Assets, a.Input())
	}
	srv := testutil.NewFakeServer(t, testutil.JSON(200, boardJSON))
	c, _ := newTestClient(t, srv)
	if _, err := c.Boards.ReplaceContent(context.Background(), doc, r, IfMatch("bv1")); err != nil {
		t.Fatal(err)
	}
	// typeName is the record's constant and empty meta is the default, so
	// neither is sent; everything else is, as read.
	assertJSON(t, srv.Requests()[0].Body, `{"shapes":[{"id":"shape:a","type":"geo","x":0,"y":0,"rotation":0,"index":"a1","parentId":"page:page","isLocked":false,"opacity":1,`+
		`"props":{"geo":"rectangle","h":50,"scale":1.0,"text":"Alpha","w":100,"zz":{"new":true}}}],`+
		`"bindings":[{"id":"binding:x","type":"arrow","fromId":"shape:ab","toId":"shape:a","props":{"terminal":"start"}}],`+
		`"assets":[{"id":"asset:i","type":"image","props":{"src":"https://assets.example/p.png","w":4}}]}`)
}

func TestBoards_C_BOARD_07_Limits(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	many := make([]BoardShapeInput, MaxBoardBatch+1)
	ids := make([]string, MaxBoardBatch+1)
	for _, err := range []error{
		func() error { _, err := c.Boards.CreateShapes(ctx, doc, BoardRecords{Shapes: many}); return err }(),
		func() error {
			_, err := c.Boards.CreateShapes(ctx, doc, BoardRecords{Bindings: make([]BoardBindingInput, MaxBoardBatch+1)})
			return err
		}(),
		func() error {
			_, err := c.Boards.ReplaceContent(ctx, doc, BoardRecords{Assets: make([]BoardAssetInput, MaxBoardBatch+1)})
			return err
		}(),
		func() error {
			_, err := c.Boards.UpdateShapes(ctx, doc, make([]BoardShapePatch, MaxBoardBatch+1))
			return err
		}(),
		func() error { _, err := c.Boards.DeleteShapes(ctx, doc, ids); return err }(),
	} {
		if !errors.Is(err, ErrBoardBatchTooLarge) {
			t.Errorf("err = %v, want ErrBoardBatchTooLarge", err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d requests sent, want none", n)
	}
}

func TestBoards_C_BOARD_07_StaleVersion(t *testing.T) {
	r := testutil.JSON(412, `{"message":"Document version is bv2, If-Match was bv1","code":"VERSION_MISMATCH","requestId":"rq1"}`)
	srv := testutil.NewFakeServer(t, r)
	c, _ := newTestClient(t, srv)
	_, err := c.Boards.UpdateShapes(context.Background(), doc, []BoardShapePatch{{ID: "shape:a", X: Some(1.0)}}, IfMatch("bv1"))
	var ae *APIError
	if !errors.Is(err, ErrVersionMismatch) || !errors.As(err, &ae) || ae.CurrentVersion != "bv2" {
		t.Fatalf("err = %v (%+v), want ErrVersionMismatch with CurrentVersion bv2", err, ae)
	}
}

// The board reads report the version but take no IfMatch.
func TestBoards_C_BOARD_01_ReadsRefuseIfMatch(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	if _, err := c.Boards.GetContent(context.Background(), doc, IfMatch("bv1")); !errors.Is(err, ErrIfMatchNotAllowed) {
		t.Errorf("err = %v, want ErrIfMatchNotAllowed", err)
	}
}
