package lk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk/blocks"
)

const (
	pP = "/api/v3/projects/p1"
	pR = pP + "/resources/r1"
	pD = pR + "/docs/d1"
	pB = pD + "/blocks/b1"
)

var doc = DocRef{ResourceID: "r1", DocumentID: "d1"}

const (
	docJSON       = `{"id":"d1","name":"Main","type":"page","rank":"a0","isHidden":false,"isFullWidth":true,"calendarId":"gregorian","createdAt":"2026-01-02T03:04:05Z"}`
	docBlocksJSON = `{"id":"d1","version":"v2","schema":1,"text":"Hi","blocks":[{"kind":"paragraph","id":"b1","content":[{"text":"Hi"}]}]}`
	blockJSON     = `{"kind":"paragraph","id":"b1","content":[{"text":"Hi"}],"version":"v3"}`
	rowJSON       = `{"id":"row1","rank":"a","key":"Species","content":[{"text":"Magic dog"}],"text":"Magic dog"}`
	rowListJSON   = `{"data":[` + rowJSON + `],"version":"v5"}`
	propJSON      = `{"kind":"keyValue","id":"pr1","rank":"a","title":"Stats","rows":[]}`
	vPropJSON     = `{"kind":"keyValue","id":"pr1","rank":"a","title":"Stats","rows":[],"version":"v6"}`
	assetJSON     = `{"id":"a1","name":"Portrait","type":"IMAGE","folderId":"f1","isHidden":false,"description":null,"url":"https://assets.example/p.png","width":4,"height":3,"createdAt":"2026-01-02T03:04:05Z"}`
)

func etag(r testutil.Reply, v string) testutil.Reply {
	r.Header["ETag"] = `W/"` + v + `"`
	return r
}

func noContent() testutil.Reply {
	return testutil.Reply{Status: http.StatusNoContent, Header: map[string]string{}}
}

// wikiCase is one operation and the exact request it must send.
type wikiCase struct {
	name  string
	reply testutil.Reply
	call  func(context.Context, *Client) (any, error)
	// method, path, query and body are the exact request; body "" means none.
	method, path, query, body string
	ifMatch                   string
	keyed                     bool
	check                     func(*testing.T, any)
}

func runWiki(t *testing.T, cases []wikiCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t, tc.reply)
			c, _ := newTestClient(t, srv)
			got, err := tc.call(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			r := srv.Requests()[0]
			if r.Method != tc.method || r.Path != tc.path || r.RawQuery != tc.query {
				t.Errorf("request = %s %s ?%s, want %s %s ?%s", r.Method, r.Path, r.RawQuery, tc.method, tc.path, tc.query)
			}
			assertJSON(t, r.Body, tc.body)
			if h := r.Header.Get("If-Match"); h != tc.ifMatch {
				t.Errorf("If-Match = %q, want %q", h, tc.ifMatch)
			}
			if k := r.Header.Get("Idempotency-Key"); (k != "") != tc.keyed {
				t.Errorf("Idempotency-Key = %q, want keyed %v", k, tc.keyed)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestWiki_Documents(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-DOC-01 list is one page", reply: testutil.JSON(200, `{"data":[`+docJSON+`],"hasMore":false,"nextCursor":null}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.List(ctx, "r1") },
			method: "GET", path: pR + "/docs",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Document])
				if len(p.Items) != 1 || p.HasMore || p.Items[0].Type != DocumentPage {
					t.Errorf("page = %+v", p)
				}
			}},
		{name: "C-DOC-02 get maps fields", reply: testutil.JSON(200, docJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.Get(ctx, doc) },
			method: "GET", path: pD,
			check: func(t *testing.T, v any) {
				d := v.(*Document)
				if d.ID != "d1" || d.Name != "Main" || d.Rank != "a0" || !d.IsFullWidth || d.CalendarID != "gregorian" || d.CreatedAt != epoch {
					t.Errorf("document = %+v", d)
				}
			}},
		{name: "C-DOC-03 create", reply: testutil.JSON(201, docJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Create(ctx, "r1", DocumentCreate{Name: "Lore", Type: DocumentPage, IsHidden: Some(true)})
			},
			method: "POST", path: pR + "/docs", body: `{"isHidden":true,"name":"Lore","type":"page"}`, keyed: true},
		{name: "C-DOC-04 update sends only set fields", reply: testutil.JSON(200, docJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{Name: Some("Lore"), IsFullWidth: Some(false)})
			},
			method: "PATCH", path: pD, body: `{"isFullWidth":false,"name":"Lore"}`},
		{name: "C-DOC-05 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Documents.Delete(ctx, doc) },
			method: "DELETE", path: pD},
		{name: "C-DOC-05 restore", reply: testutil.JSON(200, docJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.Restore(ctx, doc) },
			method: "POST", path: pD + "/restore", keyed: true},
		{name: "C-DOC-06 getBlocks", reply: testutil.JSON(200, docBlocksJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.GetBlocks(ctx, doc) },
			method: "GET", path: pD + "/blocks",
			check: func(t *testing.T, v any) {
				d := v.(*DocumentBlocks)
				if d.ID != "d1" || d.Version != "v2" || d.Schema != 1 || d.Text != "Hi" || len(d.Blocks) != 1 {
					t.Errorf("blocks = %+v", d)
				}
			}},
		{name: "C-DOC-06 unknown schema is kept", reply: testutil.JSON(200, `{"id":"d1","version":"v","schema":2,"text":"","blocks":[]}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.GetBlocks(ctx, doc) },
			method: "GET", path: pD + "/blocks",
			check: func(t *testing.T, v any) {
				if v.(*DocumentBlocks).Schema != 2 {
					t.Errorf("schema = %d", v.(*DocumentBlocks).Schema)
				}
			}},
		{name: "C-DOC-07 replaceBlocks", reply: testutil.JSON(200, docBlocksJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.P("Hi")}, IfMatch("v1"))
			},
			method: "PUT", path: pD + "/blocks", body: `{"blocks":[{"kind":"paragraph","content":[{"text":"Hi"}]}]}`, ifMatch: "v1",
			check: func(t *testing.T, v any) {
				if v.(*DocumentBlocks).Version != "v2" || v.(*DocumentBlocks).Blocks[0].NodeID() != "b1" {
					t.Errorf("written = %+v", v)
				}
			}},
		{name: "C-DOC-08 assignBlockIds", reply: testutil.JSON(200, `{"assigned":3,"version":"v9"}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.AssignBlockIDs(ctx, doc) },
			method: "POST", path: pD + "/block-ids", keyed: true,
			check: func(t *testing.T, v any) {
				if r := v.(*BlockIDsResult); r.Assigned != 3 || r.Version != "v9" {
					t.Errorf("result = %+v", r)
				}
			}},
		{name: "C-DOC-09 create a resource with page content", reply: testutil.JSON(201, okResource),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.Create(ctx, ResourceCreate{Name: "Jake", Documents: Some([]ResourceDocument{
					{Name: Some("Main"), Blocks: blocks.List{blocks.H(1, "Jake")}},
					{Name: Some("Notes"), Type: Some(DocumentBlank)},
				})})
			},
			method: "POST", path: pP + "/resources", keyed: true,
			body: `{"documents":[{"blocks":[{"kind":"heading","level":1,"content":[{"text":"Jake"}]}],"name":"Main"},{"name":"Notes","type":"blank"}],"name":"Jake"}`},
	})
}

func TestWiki_Blocks(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-BLOCK-06 get", reply: testutil.JSON(200, blockJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Blocks.Get(ctx, doc, "b1") },
			method: "GET", path: pB,
			check: func(t *testing.T, v any) {
				r := v.(*Versioned[blocks.Block])
				if r.Version != "v3" || r.Value.NodeID() != "b1" {
					t.Errorf("block = %+v", r)
				}
				// The version is not kept as an unmodelled block field.
				assertJSON(t, mustMarshal(t, r.Value), `{"kind":"paragraph","id":"b1","content":[{"text":"Hi"}]}`)
			}},
		{name: "C-BLOCK-07 insert after an anchor", reply: testutil.JSON(201, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Insert(ctx, doc, blocks.P("Where Finn and Jake live."), At{After: "title"}, IfMatch("v2"))
			},
			method: "POST", path: pD + "/blocks", ifMatch: "v2", keyed: true,
			body: `{"block":{"kind":"paragraph","content":[{"text":"Where Finn and Jake live."}]},"after":"title"}`},
		{name: "C-BLOCK-07 insert appends with no anchor", reply: testutil.JSON(201, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Insert(ctx, doc, blocks.Rule(), At{})
			},
			method: "POST", path: pD + "/blocks", keyed: true, body: `{"block":{"kind":"rule"}}`},
		{name: "C-BLOCK-08 update sends only set fields", reply: testutil.JSON(200, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Update(ctx, doc, "b1", BlockPatch{Content: Some(blocks.R("Where Finn and Jake live, with ", blocks.Bold("BMO"), ".")),
					IsTitleHidden: Some(false)}, IfMatch("v3"))
			},
			method: "PATCH", path: pB, ifMatch: "v3",
			body: `{"content":[{"text":"Where Finn and Jake live, with "},{"text":"BMO","bold":true},{"text":"."}],"isTitleHidden":false}`},
		{name: "C-BLOCK-08 empty patch", reply: testutil.JSON(200, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Update(ctx, doc, "b1", BlockPatch{})
			},
			method: "PATCH", path: pB, body: `{}`},
		{name: "C-BLOCK-08 rows and items", reply: testutil.JSON(200, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Update(ctx, doc, "b1", BlockPatch{Rows: Some(blocks.KVRows{blocks.KV("k", "v")}),
					ResourceItems: Some(blocks.ResourceItems{{ResourceID: "finn"}})})
			},
			method: "PATCH", path: pB, body: `{"rows":[{"key":"k","content":[{"text":"v"}]}],"items":[{"resourceId":"finn"}]}`},
		{name: "C-BLOCK-09 replace", reply: testutil.JSON(200, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Replace(ctx, doc, "b1", blocks.H(2, "The Tree Fort, home of heroes"), IfMatch("v4"))
			},
			method: "PUT", path: pB, ifMatch: "v4", body: `{"kind":"heading","level":2,"content":[{"text":"The Tree Fort, home of heroes"}]}`},
		{name: "C-BLOCK-10 move", reply: testutil.JSON(200, blockJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.Move(ctx, doc, "b1", At{Before: "intro"}, IfMatch("v5"))
			},
			method: "POST", path: pB + "/move", ifMatch: "v5", keyed: true, body: `{"before":"intro"}`},
		{name: "C-BLOCK-11 delete returns no version", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Blocks.Delete(ctx, doc, "b1", IfMatch("v6"))
			},
			method: "DELETE", path: pB, ifMatch: "v6"},
		{name: "C-BLOCK-13 list infobox properties", reply: testutil.JSON(200, `{"data":[`+propJSON+`,{"kind":"unsupported","id":"u1"}],"version":"v7"}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Blocks.ListProperties(ctx, doc, "b1") },
			method: "GET", path: pB + "/properties",
			check: func(t *testing.T, v any) {
				l := v.(*VersionedList[blocks.Property])
				if l.Version != "v7" || len(l.Items) != 2 || l.Items[0].Kind() != blocks.KindKeyValue {
					t.Errorf("list = %+v", l)
				}
				if _, ok := l.Items[1].(*blocks.UnsupportedProperty); !ok {
					t.Errorf("unsupported = %#v", l.Items[1])
				}
			}},
		{name: "C-BLOCK-13 get infobox property", reply: testutil.JSON(200, vPropJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Blocks.GetProperty(ctx, doc, "b1", "pr1") },
			method: "GET", path: pB + "/properties/pr1",
			check: func(t *testing.T, v any) {
				if r := v.(*Versioned[blocks.Property]); r.Version != "v6" || r.Value.PropertyEnvelope().Title.Or("") != "Stats" {
					t.Errorf("property = %+v", r)
				}
			}},
		{name: "C-BLOCK-13 create infobox property", reply: testutil.JSON(201, vPropJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				p := blocks.AsProperty(blocks.TextField("Got his powers from a magic mud puddle."), blocks.Envelope{Title: Some("Origin"), Rank: "zz"})
				return c.Blocks.CreateProperty(ctx, doc, "b1", p, Place{Before: "stats"}, IfMatch("v6"))
			},
			method: "POST", path: pB + "/properties", ifMatch: "v6", keyed: true,
			body: `{"before":"stats","children":[{"kind":"paragraph","content":[{"text":"Got his powers from a magic mud puddle."}]}],"kind":"textField","title":"Origin"}`},
		{name: "C-BLOCK-13 update infobox property", reply: testutil.JSON(200, vPropJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.UpdateProperty(ctx, doc, "b1", "pr1", PropertyPatch{IsHidden: Some(true)}, IfMatch("v7"))
			},
			method: "PATCH", path: pB + "/properties/pr1", ifMatch: "v7", body: `{"isHidden":true}`},
		{name: "C-BLOCK-13 delete infobox property", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Blocks.DeleteProperty(ctx, doc, "b1", "pr1", IfMatch("v8"))
			},
			method: "DELETE", path: pB + "/properties/pr1", ifMatch: "v8"},
	})
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWiki_Rows(t *testing.T) {
	row := etag(testutil.JSON(201, rowJSON), "v4")
	checkRow := func(t *testing.T, v any) {
		r := v.(*Versioned[blocks.KVRow])
		if r.Version != "v4" || r.Value.ID != "row1" || r.Value.Key != "Species" || r.Value.Text.Or("") != "Magic dog" {
			t.Errorf("row = %+v", r)
		}
	}
	checkList := func(t *testing.T, v any) {
		l := v.(*VersionedList[blocks.KVRow])
		if l.Version != "v5" || len(l.Items) != 1 || l.Items[0].Rank != "a" {
			t.Errorf("rows = %+v", l)
		}
	}
	create := RowCreate{Key: "Best friend", Content: blocks.R(blocks.Bold("Football"), blocks.BR(), "the reflection"), After: Some("plays")}
	createBody := `{"key":"Best friend","content":[{"text":"Football","bold":true},{"break":true},{"text":"the reflection"}],"after":"plays"}`
	runWiki(t, []wikiCase{
		{name: "C-ROW-01 C-ROW-05 block rows", reply: testutil.JSON(200, rowListJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Blocks.ListRows(ctx, doc, "b1") },
			method: "GET", path: pB + "/rows", check: checkList},
		{name: "C-ROW-03 create block row", reply: row,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.CreateRow(ctx, doc, "b1", create, IfMatch("v3"))
			},
			method: "POST", path: pB + "/rows", ifMatch: "v3", keyed: true, body: createBody, check: checkRow},
		{name: "C-ROW-03 create with plain text", reply: row,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.CreateRow(ctx, doc, "b1", RowCreate{Key: "Age", Text: Some("28")})
			},
			method: "POST", path: pB + "/rows", keyed: true, body: `{"key":"Age","text":"28"}`},
		{name: "C-ROW-04 update clears content with null", reply: etag(testutil.JSON(200, rowJSON), "v4"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.UpdateRow(ctx, doc, "b1", "row1", RowPatch{Content: Null[blocks.Runs]()}, IfMatch("v3"))
			},
			method: "PATCH", path: pB + "/rows/row1", ifMatch: "v3", body: `{"content":null}`, check: checkRow},
		{name: "C-ROW-04 update key only", reply: etag(testutil.JSON(200, rowJSON), "v4"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.UpdateRow(ctx, doc, "b1", "row1", RowPatch{Key: Some("Kind")})
			},
			method: "PATCH", path: pB + "/rows/row1", body: `{"key":"Kind"}`},
		{name: "C-ROW-05 delete block row", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Blocks.DeleteRow(ctx, doc, "b1", "row1", IfMatch("v4"))
			},
			method: "DELETE", path: pB + "/rows/row1", ifMatch: "v4"},
		{name: "C-ROW-02 sidebar property rows", reply: testutil.JSON(200, rowListJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Properties.ListRows(ctx, "r1", "pr1") },
			method: "GET", path: pR + "/properties/pr1/rows", check: checkList},
		{name: "C-ROW-02 create sidebar row", reply: row,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Properties.CreateRow(ctx, "r1", "pr1", create, IfMatch("v3"))
			},
			method: "POST", path: pR + "/properties/pr1/rows", ifMatch: "v3", keyed: true, body: createBody, check: checkRow},
		{name: "C-ROW-02 update sidebar row", reply: etag(testutil.JSON(200, rowJSON), "v4"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Properties.UpdateRow(ctx, "r1", "pr1", "row1", RowPatch{Text: Some("x")}, IfMatch("v3"))
			},
			method: "PATCH", path: pR + "/properties/pr1/rows/row1", ifMatch: "v3", body: `{"text":"x"}`},
		{name: "C-ROW-02 delete sidebar row", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Properties.DeleteRow(ctx, "r1", "pr1", "row1", IfMatch("v4"))
			},
			method: "DELETE", path: pR + "/properties/pr1/rows/row1", ifMatch: "v4"},
		{name: "C-ROW-02 infobox property rows", reply: testutil.JSON(200, rowListJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.ListPropertyRows(ctx, doc, "b1", "pr1")
			},
			method: "GET", path: pB + "/properties/pr1/rows", check: checkList},
		{name: "C-ROW-02 create infobox row", reply: row,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.CreatePropertyRow(ctx, doc, "b1", "pr1", create, IfMatch("v3"))
			},
			method: "POST", path: pB + "/properties/pr1/rows", ifMatch: "v3", keyed: true, body: createBody, check: checkRow},
		{name: "C-ROW-02 update infobox row", reply: etag(testutil.JSON(200, rowJSON), "v4"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Blocks.UpdatePropertyRow(ctx, doc, "b1", "pr1", "row1", RowPatch{Key: Some("k")}, IfMatch("v3"))
			},
			method: "PATCH", path: pB + "/properties/pr1/rows/row1", ifMatch: "v3", body: `{"key":"k"}`},
		{name: "C-ROW-02 delete infobox row", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Blocks.DeletePropertyRow(ctx, doc, "b1", "pr1", "row1", IfMatch("v4"))
			},
			method: "DELETE", path: pB + "/properties/pr1/rows/row1", ifMatch: "v4"},
	})
}

func TestWiki_Properties(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-PROP-02 list", reply: testutil.JSON(200, `{"data":[`+propJSON+`,{"kind":"chart","id":"c1"}],"hasMore":false,"nextCursor":null}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Properties.List(ctx, "r1") },
			method: "GET", path: pR + "/properties",
			check: func(t *testing.T, v any) {
				p := v.(*Page[blocks.Property])
				if len(p.Items) != 2 || p.Items[0].Kind() != blocks.KindKeyValue || p.Items[1].Kind() != "chart" {
					t.Errorf("properties = %+v", p.Items)
				}
			}},
		{name: "C-PROP-02 get", reply: testutil.JSON(200, propJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Properties.Get(ctx, "r1", "pr1") },
			method: "GET", path: pR + "/properties/pr1"},
		{name: "C-PROP-03 create drops rank and places", reply: testutil.JSON(201, propJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				p := &blocks.KeyValueProperty{Envelope: blocks.Envelope{ID: "stats", Rank: "a", Title: Some("Stats")}, Rows: blocks.KVRows{blocks.KV("Species", "Magic dog")}}
				return c.Properties.Create(ctx, "r1", p, Place{After: "portrait"})
			},
			method: "POST", path: pR + "/properties", keyed: true,
			body: `{"after":"portrait","id":"stats","kind":"keyValue","rows":[{"key":"Species","content":[{"text":"Magic dog"}]}],"title":"Stats"}`},
		{name: "C-PROP-04 update merges set fields", reply: testutil.JSON(200, propJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Properties.Update(ctx, "r1", "pr1", PropertyPatch{Rank: Some("b"), Display: Some(blocks.DisplayGrid),
					MeterItems: Some(blocks.MeterItems{{Name: "HP", Icon: "heart", Current: 1, Max: 2}})})
			},
			method: "PATCH", path: pR + "/properties/pr1", body: `{"rank":"b","display":"grid","items":[{"name":"HP","icon":"heart","current":1,"max":2}]}`},
		{name: "C-PROP-05 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Properties.Delete(ctx, "r1", "pr1") },
			method: "DELETE", path: pR + "/properties/pr1"},
		{name: "C-PROP-06 unsupported round-trips and patches its envelope", reply: testutil.JSON(200, `{"kind":"unsupported","id":"u1","rank":"a","isHidden":true}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Properties.Update(ctx, "r1", "u1", PropertyPatch{Title: Some("Legacy")})
			},
			method: "PATCH", path: pR + "/properties/u1", body: `{"title":"Legacy"}`,
			check: func(t *testing.T, v any) {
				u, ok := v.(*blocks.UnsupportedProperty)
				if !ok {
					t.Fatalf("got %#v", v)
				}
				assertJSON(t, mustMarshal(t, u), `{"kind":"unsupported","id":"u1","rank":"a","isHidden":true}`)
			}},
	})
}

func TestWiki_C_PROP_01_SidebarPropertiesTyped(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"id":"r1","name":"Jake","tags":[],"aliases":[],"rank":"a","isHidden":false,"permissions":{},"documents":[],
"properties":[{"kind":"image","id":"p1","rank":"a","title":"Portrait","isTitleHidden":true,"url":"https://assets.example/j.png","fill":true},{"kind":"meter","id":"p2","mode":"bar","items":[]},{"kind":"unsupported","id":"p3"}]}`))
	c, _ := newTestClient(t, srv)
	r, err := c.Resources.Get(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	img, ok := r.Properties[0].(*blocks.ImageProperty)
	if !ok || img.ID != "p1" || img.Title.Or("") != "Portrait" || !img.IsTitleHidden.Or(false) || !img.Fill.Or(false) {
		t.Errorf("image property = %#v", r.Properties[0])
	}
	if _, ok := r.Properties[1].(*blocks.MeterProperty); !ok {
		t.Errorf("meter property = %#v", r.Properties[1])
	}
	if _, ok := r.Properties[2].(*blocks.UnsupportedProperty); !ok {
		t.Errorf("unsupported property = %#v", r.Properties[2])
	}
}

func TestWiki_Assets(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-ASSET-01 list is paged", reply: testutil.JSON(200, `{"data":[`+assetJSON+`],"hasMore":true,"nextCursor":"n1"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Assets.List(ctx, AssetListParams{FolderID: "f1", Limit: 10})
			},
			method: "GET", path: pP + "/assets", query: "folderId=f1&limit=10",
			check: func(t *testing.T, v any) {
				if p := v.(*Page[Asset]); !p.HasMore || p.NextCursor != "n1" {
					t.Errorf("page = %+v", p)
				}
			}},
		{name: "C-ASSET-01 list starts at a cursor", reply: testutil.JSON(200, `{"data":[],"hasMore":false}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Assets.List(ctx, AssetListParams{FolderID: "f1", Cursor: "n1"})
			},
			method: "GET", path: pP + "/assets", query: "cursor=n1&folderId=f1"},
		{name: "C-PAGE-01 resources list starts at a cursor", reply: testutil.JSON(200, `{"data":[],"hasMore":false}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.List(ctx, ResourceListParams{ParentID: ParentRoot, Limit: 5, Cursor: "c9"})
			},
			method: "GET", path: pP + "/resources", query: "cursor=c9&limit=5&parentId=root"},
		{name: "C-ASSET-02 get maps fields", reply: testutil.JSON(200, assetJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Assets.Get(ctx, "a1") },
			method: "GET", path: pP + "/assets/a1",
			check: func(t *testing.T, v any) {
				a := v.(*Asset)
				if a.Type != AssetImage || a.FolderID != "f1" || a.URL == "" || a.Width != 4 || a.Height != 3 || a.Description != "" || a.CreatedAt != epoch {
					t.Errorf("asset = %+v", a)
				}
			}},
		{name: "C-ASSET-02 unknown type kept", reply: testutil.JSON(200, `{"id":"a2","name":"x","type":"HOLOGRAM"}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Assets.Get(ctx, "a2") },
			method: "GET", path: pP + "/assets/a2",
			check: func(t *testing.T, v any) {
				if v.(*Asset).Type != "HOLOGRAM" {
					t.Errorf("type = %q", v.(*Asset).Type)
				}
			}},
		{name: "C-ASSET-03 create folder", reply: testutil.JSON(201, assetJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Assets.CreateFolder(ctx, AssetFolderCreate{Name: "Treasure", FolderID: Some("f0")})
			},
			method: "POST", path: pP + "/assets", keyed: true, body: `{"folderId":"f0","name":"Treasure"}`},
		{name: "C-ASSET-04 update moves to root with null", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Assets.Update(ctx, "a1", AssetUpdate{FolderID: Null[string](), Name: Some("Gold")})
			},
			method: "PATCH", path: pP + "/assets/a1", body: `{"folderId":null,"name":"Gold"}`},
		{name: "C-ASSET-05 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Assets.Delete(ctx, "a1") },
			method: "DELETE", path: pP + "/assets/a1"},
		{name: "C-ASSET-06 create upload", reply: testutil.JSON(200, `{"uploadKey":"k1","uploadUrl":"https://storage.example/put","method":"PUT","headers":{"content-type":"image/png"},"expiresAt":"2026-01-02T03:14:05Z"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Assets.CreateImageUpload(ctx, ImageUploadCreate{ContentType: "image/png", ContentLength: 42})
			},
			method: "POST", path: pP + "/assets/uploads", keyed: true, body: `{"contentLength":42,"contentType":"image/png"}`,
			check: func(t *testing.T, v any) {
				u := v.(*ImageUpload)
				if u.UploadKey != "k1" || u.Method != "PUT" || u.Headers["content-type"] != "image/png" || !u.ExpiresAt.Equal(epoch.Add(600e9)) {
					t.Errorf("upload = %+v", u)
				}
			}},
		{name: "C-ASSET-07 create image", reply: testutil.JSON(201, assetJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Assets.CreateImage(ctx, ImageCreate{UploadKey: "k1", Name: "Portrait", Description: Some("Jake")})
			},
			method: "POST", path: pP + "/assets/images", keyed: true, body: `{"description":"Jake","name":"Portrait","uploadKey":"k1"}`},
	})
}

func TestWiki_SearchAndTags(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-SEARCH-01 search", reply: testutil.JSON(200, `{"data":[
{"resource":{"id":"r1","name":"Ice King","tags":["Royalty"],"iconGlyph":null,"parentId":"ooo","path":[{"id":"ooo","name":"Ooo"}]},
 "document":{"id":"d1","name":"Main","highlights":[{"segment":"Simon Petrikov","index":0,"matchStart":0,"matchEnd":5}],"isSolo":true}},
{"resource":{"id":"r1","name":"Ice King","tags":[]},"document":{"id":"d2","name":"Notes"}},
{"resource":{"id":"r2","name":"Simon","tags":[]}}],"hasMore":false,"nextCursor":null}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Search.Search(ctx, SearchParams{Query: "Simon"})
			},
			method: "GET", path: pP + "/search", query: "q=Simon",
			check: func(t *testing.T, v any) {
				hits := v.(*Page[SearchHit]).Items
				if len(hits) != 3 || hits[0].Resource.ParentID != "ooo" || hits[0].Resource.Path[0].Name != "Ooo" ||
					hits[0].Document.Highlights[0].MatchEnd != 5 || !hits[0].Document.IsSolo || hits[2].Document != nil {
					t.Errorf("hits = %+v", hits)
				}
				if hits[1].Resource.ID != hits[0].Resource.ID {
					t.Error("per-document duplicates must be kept")
				}
			}},
		{name: "C-SEARCH-01 limit sent only when set", reply: testutil.JSON(200, `{"data":[]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Search.Search(ctx, SearchParams{Query: "Fionna and Cake", Limit: 5})
			},
			method: "GET", path: pP + "/search", query: "limit=5&q=Fionna+and+Cake"},
		{name: "C-TAG-01 list", reply: testutil.JSON(200, `{"data":[{"tag":"Kingdom","count":3},{"tag":"Cold","count":1}],"hasMore":false,"nextCursor":null}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Tags.List(ctx) },
			method: "GET", path: pP + "/tags",
			check: func(t *testing.T, v any) {
				if tags := v.(*Page[Tag]).Items; len(tags) != 2 || tags[0] != (Tag{Tag: "Kingdom", Count: 3}) {
					t.Errorf("tags = %+v", tags)
				}
			}},
	})
}

func TestWiki_C_CORE2_03_C_ROW_05_VersionFromETag(t *testing.T) {
	// Row writes have no version in the body; it comes from the ETag.
	srv := testutil.NewFakeServer(t, etag(testutil.JSON(201, rowJSON), "v-etag"), etag(testutil.JSON(200, rowListJSON), "ignored"))
	c, _ := newTestClient(t, srv)
	r, err := c.Blocks.CreateRow(context.Background(), doc, "b1", RowCreate{Key: "k"})
	if err != nil || r.Version != "v-etag" {
		t.Fatalf("row = %+v, %v", r, err)
	}
	// A body version wins over the ETag.
	l, err := c.Blocks.ListRows(context.Background(), doc, "b1")
	if err != nil || l.Version != "v5" {
		t.Fatalf("rows = %+v, %v", l, err)
	}
}

func TestWiki_C_CORE2_02_C_CORE2_06_C_VER_06_IfMatchOnlyOnVersionedWrites(t *testing.T) {
	ctx := context.Background()
	refused := map[string]func(*Client) error{
		"assignBlockIds": func(c *Client) error { _, err := c.Documents.AssignBlockIDs(ctx, doc, IfMatch("v")); return err },
		"getBlocks":      func(c *Client) error { _, err := c.Documents.GetBlocks(ctx, doc, IfMatch("v")); return err },
		"blocks.get":     func(c *Client) error { _, err := c.Blocks.Get(ctx, doc, "b1", IfMatch("v")); return err },
		"listRows":       func(c *Client) error { _, err := c.Blocks.ListRows(ctx, doc, "b1", IfMatch("v")); return err },
		"documents.update": func(c *Client) error {
			_, err := c.Documents.Update(ctx, doc, DocumentUpdate{}, IfMatch("v"))
			return err
		},
		"properties.create": func(c *Client) error {
			_, err := c.Properties.Create(ctx, "r1", &blocks.TextFieldProperty{}, Place{}, IfMatch("v"))
			return err
		},
		"properties.update": func(c *Client) error {
			_, err := c.Properties.Update(ctx, "r1", "p", PropertyPatch{}, IfMatch("v"))
			return err
		},
		"properties.delete": func(c *Client) error { return c.Properties.Delete(ctx, "r1", "p", IfMatch("v")) },
		"assets.update":     func(c *Client) error { return c.Assets.Update(ctx, "a", AssetUpdate{}, IfMatch("v")) },
	}
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	for name, f := range refused {
		if err := f(c); !errors.Is(err, ErrIfMatchNotAllowed) {
			t.Errorf("%s with IfMatch: err = %v", name, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestWiki_C_BLOCK_07_C_BLOCK_10_AnchorChecks(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	if _, err := c.Blocks.Insert(ctx, doc, blocks.Rule(), At{After: "a", Parent: "p"}); !errors.Is(err, ErrAnchor) {
		t.Errorf("insert with two anchors: %v", err)
	}
	for _, at := range []At{{}, {After: "a", Before: "b"}} {
		if _, err := c.Blocks.Move(ctx, doc, "b1", at); !errors.Is(err, ErrAnchor) {
			t.Errorf("move %+v: %v", at, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestWiki_C_BLOCK_12_C_VER_05_EveryBlocksWriteTakesIfMatch(t *testing.T) {
	ctx := context.Background()
	writes := map[string]struct {
		reply testutil.Reply
		call  func(*Client) (string, error)
	}{
		"insert": {testutil.JSON(201, blockJSON), func(c *Client) (string, error) {
			r, err := c.Blocks.Insert(ctx, doc, blocks.Rule(), At{}, IfMatch("v"))
			return ver(r, err)
		}},
		"update": {testutil.JSON(200, blockJSON), func(c *Client) (string, error) {
			r, err := c.Blocks.Update(ctx, doc, "b1", BlockPatch{}, IfMatch("v"))
			return ver(r, err)
		}},
		"replace": {testutil.JSON(200, blockJSON), func(c *Client) (string, error) {
			r, err := c.Blocks.Replace(ctx, doc, "b1", blocks.Rule(), IfMatch("v"))
			return ver(r, err)
		}},
		"move": {testutil.JSON(200, blockJSON), func(c *Client) (string, error) {
			r, err := c.Blocks.Move(ctx, doc, "b1", At{Parent: "p"}, IfMatch("v"))
			return ver(r, err)
		}},
		"delete": {etag(noContent(), "v-after"), func(c *Client) (string, error) {
			return "", c.Blocks.Delete(ctx, doc, "b1", IfMatch("v"))
		}},
	}
	for name, w := range writes {
		srv := testutil.NewFakeServer(t, w.reply)
		c, _ := newTestClient(t, srv)
		got, err := w.call(c)
		if err != nil || srv.Requests()[0].Header.Get("If-Match") != "v" {
			t.Errorf("%s: err %v, If-Match %q", name, err, srv.Requests()[0].Header.Get("If-Match"))
		}
		if want := map[bool]string{true: "", false: "v3"}[name == "delete"]; got != want {
			t.Errorf("%s: version %q, want %q", name, got, want)
		}
	}
}

func ver(r *Versioned[blocks.Block], err error) (string, error) {
	if err != nil {
		return "", err
	}
	return r.Version, nil
}
