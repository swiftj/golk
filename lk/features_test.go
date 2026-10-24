package lk

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

const pF = pD + "/features"

// featureJSON has every field of the spec's Feature schema, with unknown
// enum values in iconShape and labelSize.
const featureJSON = `{"id":"f1","type":"region","name":"Ice Kingdom","parentId":"fold1","rank":"a0","isHidden":true,
"uri":"lk://resources/r2","pos":[12.5,-40.25],"polygon":[[40,-60],[40,-20],[30,-20]],"polygons":[[[1,2],[3,4]],[[5,6]]],
"polyline":[[7,8],[9,10]],"iconGlyph":"tree","iconColor":"#112233","iconShape":"hexagon","iconSize":"small",
"pinImage":{"url":"https://assets.example/p.png","width":64,"height":32},"color":"#a8d8ff","fillOpacity":0,
"fillStyle":"striped","borderStyle":"dashed","strokeOpacity":0.5,"strokeStyle":"dotted","strokeWidth":3,
"labelSize":"medium","labelVisibility":"hover","textColor":"#ffffff","textEffect":"background","textEffectColor":"#000000",
"updatedAt":"2026-01-02T08:04:05+05:00"}`

const folderJSON = `{"id":"fold1","type":"folder","name":"Homes"}`

func TestFeatures(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-FEAT-01 list is one page, no type", reply: testutil.JSON(200, `{"data":[`+featureJSON+`,`+folderJSON+`],"hasMore":false,"nextCursor":null}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.List(ctx, doc, FeatureListParams{})
			},
			method: "GET", path: pF,
			check: func(t *testing.T, v any) {
				p := v.(*Page[Feature])
				if len(p.Items) != 2 || p.HasMore || p.Items[1].Type != FeatureFolder || p.Items[1].Pos.IsSet() {
					t.Errorf("page = %+v", p)
				}
			}},
		{name: "C-FEAT-01 list by type", reply: testutil.JSON(200, `{"data":[],"hasMore":false}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.List(ctx, doc, FeatureListParams{Type: FeaturePin})
			},
			method: "GET", path: pF, query: "type=pin"},
		{name: "C-FEAT-02 create sends only set fields", reply: testutil.JSON(201, `{"id":"f2","type":"pin","name":"The Tree Fort","pos":[12.5,-40.25]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.Create(ctx, doc, FeatureCreate{
					Type: Some(FeaturePin), Name: Some("The Tree Fort"), Pos: Some([2]float64{12.5, -40.25}),
					ParentID: Some("fold1"), URI: Some("lk://resources/r2"), IconShape: Some(IconShapeNone),
				})
			},
			method: "POST", path: pF, keyed: true,
			body: `{"iconShape":"none","name":"The Tree Fort","parentId":"fold1","pos":[12.5,-40.25],"type":"pin","uri":"lk://resources/r2"}`,
			check: func(t *testing.T, v any) {
				f := v.(*Feature)
				if p, _ := f.Pos.Get(); f.ID != "f2" || p != [2]float64{12.5, -40.25} {
					t.Errorf("feature = %+v", f)
				}
			}},
		{name: "C-FEAT-02 create empty sends {}", reply: testutil.JSON(201, `{"id":"f3"}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Features.Create(ctx, doc, FeatureCreate{}) },
			method: "POST", path: pF, keyed: true, body: `{}`},
		{name: "C-FEAT-02 create every field", reply: testutil.JSON(201, `{"id":"f4"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.Create(ctx, doc, FeatureCreate{
					ID: Some("abcdefgh"), Type: Some(FeatureRegion), Name: Some("N"), ParentID: Some("p"), Rank: Some("a1"),
					IsHidden: Some(false), URI: Some("lk://documents/d2"), Pos: Some([2]float64{-1.5, 2}),
					Polygon: Some([][2]float64{{1, 2}, {3, 4}}), Polygons: Some([][][2]float64{{{1, 2}}, {}}),
					Polyline: Some([][2]float64{}), VisibilityRange: Some([2]float64{0, 1}),
					IconGlyph: Some("tree"), IconColor: Some("#111111"), IconShape: Some(IconShapeFlag), IconSize: Some(IconSizeSmall),
					PinImage: Some(PinImage{URL: "https://assets.example/p.png", Width: 64, Height: 32}),
					Color:    Some("#222222"), FillOpacity: Some(0.0), FillStyle: Some(FillDotted), BorderStyle: Some(BorderNone),
					StrokeOpacity: Some(1.0), StrokeStyle: Some(StrokeSolid), StrokeWidth: Some(2.5),
					LabelSize: Some(Label2XL), LabelVisibility: Some(LabelNever), TextColor: Some("#333333"),
					TextEffect: Some(TextEffectOutline), TextEffectColor: Some("#444444"),
				})
			},
			method: "POST", path: pF, keyed: true,
			body: mustJSON(t, `{"borderStyle":"none","color":"#222222","fillOpacity":0,"fillStyle":"dotted","iconColor":"#111111",
"iconGlyph":"tree","iconShape":"flag","iconSize":"small","id":"abcdefgh","isHidden":false,"labelSize":"2xl","labelVisibility":"never",
"name":"N","parentId":"p","pinImage":{"height":32,"url":"https://assets.example/p.png","width":64},"polygon":[[1,2],[3,4]],
"polygons":[[[1,2]],[]],"polyline":[],"pos":[-1.5,2],"rank":"a1","strokeOpacity":1,"strokeStyle":"solid","strokeWidth":2.5,
"textColor":"#333333","textEffect":"outline","textEffectColor":"#444444","type":"region","uri":"lk://documents/d2","visibilityRange":[0,1]}`)},
		{name: "C-FEAT-03 get maps every field, unknown enums kept", reply: testutil.JSON(200, featureJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Features.Get(ctx, doc, "f1") },
			method: "GET", path: pF + "/f1",
			check: func(t *testing.T, v any) {
				want := &Feature{
					ID: "f1", Type: FeatureRegion, Name: "Ice Kingdom", ParentID: "fold1", Rank: "a0", IsHidden: true,
					URI: "lk://resources/r2", Pos: Some([2]float64{12.5, -40.25}),
					Polygon:   [][2]float64{{40, -60}, {40, -20}, {30, -20}},
					Polygons:  [][][2]float64{{{1, 2}, {3, 4}}, {{5, 6}}},
					Polyline:  [][2]float64{{7, 8}, {9, 10}},
					IconGlyph: "tree", IconColor: "#112233", IconShape: "hexagon", IconSize: IconSizeSmall,
					PinImage: &PinImage{URL: "https://assets.example/p.png", Width: 64, Height: 32},
					Color:    "#a8d8ff", FillOpacity: Some(0.0), FillStyle: FillStriped, BorderStyle: BorderDashed,
					StrokeOpacity: Some(0.5), StrokeStyle: StrokeDotted, StrokeWidth: Some(3.0),
					LabelSize: "medium", LabelVisibility: LabelHover, TextColor: "#ffffff", TextEffect: TextEffectBackground,
					TextEffectColor: "#000000", UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				}
				got := v.(*Feature)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("feature\n got: %+v\nwant: %+v", got, want)
				}
				if got.UpdatedAt.Location() != time.UTC {
					t.Errorf("UpdatedAt not UTC: %v", got.UpdatedAt)
				}
			}},
		{name: "C-FEAT-03 update sends only set fields", reply: testutil.JSON(200, `{"id":"f1"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.Update(ctx, doc, "f1", FeatureUpdate{Pos: Some([2]float64{12.75, -40.5}), IconGlyph: Some("tree")})
			},
			method: "PATCH", path: pF + "/f1", body: `{"iconGlyph":"tree","pos":[12.75,-40.5]}`},
		{name: "C-FEAT-03 update uri empty clears it", reply: testutil.JSON(200, `{"id":"f1"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.Update(ctx, doc, "f1", FeatureUpdate{URI: Some("")})
			},
			method: "PATCH", path: pF + "/f1", body: `{"uri":""}`},
		{name: "C-FEAT-03 update empty sends {}", reply: testutil.JSON(200, `{"id":"f1"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Features.Update(ctx, doc, "f1", FeatureUpdate{})
			},
			method: "PATCH", path: pF + "/f1", body: `{}`},
		{name: "C-FEAT-04 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Features.Delete(ctx, doc, "f1") },
			method: "DELETE", path: pF + "/f1"},
	})
}

// A-33: feature operations are unversioned, so IfMatch is refused before any
// request is sent.
func TestFeatures_C_FEAT_03_NoIfMatch(t *testing.T) {
	calls := map[string]func(context.Context, *Client) error{
		"list": func(ctx context.Context, c *Client) error {
			_, err := c.Features.List(ctx, doc, FeatureListParams{}, IfMatch("v1"))
			return err
		},
		"get": func(ctx context.Context, c *Client) error {
			_, err := c.Features.Get(ctx, doc, "f1", IfMatch("v1"))
			return err
		},
		"create": func(ctx context.Context, c *Client) error {
			_, err := c.Features.Create(ctx, doc, FeatureCreate{}, IfMatch("v1"))
			return err
		},
		"update": func(ctx context.Context, c *Client) error {
			_, err := c.Features.Update(ctx, doc, "f1", FeatureUpdate{}, IfMatch("v1"))
			return err
		},
		"delete": func(ctx context.Context, c *Client) error { return c.Features.Delete(ctx, doc, "f1", IfMatch("v1")) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"id":"f1"}`))
			c, _ := newTestClient(t, srv)
			if err := call(context.Background(), c); !errors.Is(err, ErrIfMatchNotAllowed) {
				t.Errorf("err = %v, want ErrIfMatchNotAllowed", err)
			}
			if n := len(srv.Requests()); n != 0 {
				t.Errorf("%d requests sent", n)
			}
		})
	}
}

func TestFeatures_C_FEAT_01_All(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"data":[`+folderJSON+`],"hasMore":false}`))
	c, _ := newTestClient(t, srv)
	var names []string
	for f, err := range c.Features.All(context.Background(), doc, FeatureListParams{Type: FeatureFolder}) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, f.Name)
	}
	if !reflect.DeepEqual(names, []string{"Homes"}) || srv.Requests()[0].RawQuery != "type=folder" {
		t.Errorf("names = %v, query = %q", names, srv.Requests()[0].RawQuery)
	}
}

func TestFeatures_C_FEAT_03_BadPos(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"id":"f1","pos":[1]}`))
	c, _ := newTestClient(t, srv)
	if _, err := c.Features.Get(context.Background(), doc, "f1"); err == nil {
		t.Error("a one-number pos decoded without error")
	}
}

const mapDocJSON = `{"id":"d1","name":"Ooo","type":"map","rank":"a0","isHidden":false,
"mapImage":{"assetId":"a1","url":"https://assets.example/m.png","width":1024,"height":768,
"bounds":[[-3,0],[0,4]],"tiles":{"urlTemplate":"https://tiles.example/{z}/{x}/{y}.png","maxNativeZoom":2}}}`

const timeDocJSON = `{"id":"d1","name":"Ages","type":"time","rank":"a0","isHidden":false,
"sources":[{"resourceId":"r2","documentId":"d2"},{"resourceId":"r3","documentId":"d3"}]}`

func TestDocuments_C_DOC_10(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-DOC-10 sources set", reply: testutil.JSON(200, timeDocJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{Sources: Some([]DocRef{{"r2", "d2"}, {"r3", "d3"}})})
			},
			method: "PATCH", path: pD, body: `{"sources":[{"documentId":"d2","resourceId":"r2"},{"documentId":"d3","resourceId":"r3"}]}`,
			check: func(t *testing.T, v any) {
				d := v.(*Document)
				if !reflect.DeepEqual(d.Sources, []DocRef{{"r2", "d2"}, {"r3", "d3"}}) || d.MapImage != nil {
					t.Errorf("document = %+v", d)
				}
			}},
		{name: "C-DOC-10 sources empty sends []", reply: testutil.JSON(200, `{"id":"d1","name":"Ages","type":"time","rank":"a0","isHidden":false,"sources":[]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{Sources: Some([]DocRef{})})
			},
			method: "PATCH", path: pD, body: `{"sources":[]}`,
			check: func(t *testing.T, v any) {
				if s := v.(*Document).Sources; s == nil || len(s) != 0 {
					t.Errorf("sources = %#v, want empty non-nil", s)
				}
			}},
		{name: "C-DOC-10 sources nil slice still sends []", reply: testutil.JSON(200, docJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{Sources: Some([]DocRef(nil))})
			},
			method: "PATCH", path: pD, body: `{"sources":[]}`},
		{name: "C-DOC-10 sources and mapImage unset are not sent", reply: testutil.JSON(200, docJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{Name: Some("Ooo")})
			},
			method: "PATCH", path: pD, body: `{"name":"Ooo"}`,
			check: func(t *testing.T, v any) {
				d := v.(*Document)
				if d.Sources != nil || d.MapImage != nil {
					t.Errorf("document = %+v", d)
				}
			}},
		{name: "C-DOC-10 mapImage set and read back", reply: testutil.JSON(200, mapDocJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Update(ctx, doc, DocumentUpdate{MapImage: Some(MapImageRef{AssetID: "a1"})})
			},
			method: "PATCH", path: pD, body: `{"mapImage":{"assetId":"a1"}}`,
			check: func(t *testing.T, v any) {
				want := &MapImage{AssetID: "a1", URL: "https://assets.example/m.png", Width: 1024, Height: 768,
					Bounds: [2][2]float64{{-3, 0}, {0, 4}},
					Tiles:  &MapTiles{URLTemplate: "https://tiles.example/{z}/{x}/{y}.png", MaxNativeZoom: 2}}
				if got := v.(*Document).MapImage; !reflect.DeepEqual(got, want) {
					t.Errorf("mapImage = %+v, want %+v", got, want)
				}
			}},
		{name: "C-DOC-10 get reads mapImage", reply: testutil.JSON(200, mapDocJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.Get(ctx, doc) },
			method: "GET", path: pD,
			check: func(t *testing.T, v any) {
				if m := v.(*Document).MapImage; m == nil || m.Width != 1024 {
					t.Errorf("mapImage = %+v", m)
				}
			}},
		{name: "C-DOC-10 create reads mapImage null as nil", reply: testutil.JSON(201, `{"id":"d1","name":"Ooo","type":"map","rank":"a0","isHidden":false,"mapImage":null}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Documents.Create(ctx, "r1", DocumentCreate{Name: "Ooo", Type: DocumentMap})
			},
			method: "POST", path: pR + "/docs", body: `{"name":"Ooo","type":"map"}`, keyed: true,
			check: func(t *testing.T, v any) {
				if v.(*Document).MapImage != nil {
					t.Error("mapImage null read as non-nil")
				}
			}},
		{name: "C-DOC-10 legacy tiled map has null assetId, url and tiles", reply: testutil.JSON(200,
			`{"id":"d1","name":"Old","type":"map","rank":"a0","isHidden":false,"mapImage":{"assetId":null,"url":null,"width":10,"height":20,"bounds":[[-1,0],[0,2]],"tiles":null}}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.Get(ctx, doc) },
			method: "GET", path: pD,
			check: func(t *testing.T, v any) {
				m := v.(*Document).MapImage
				if m == nil || m.AssetID != "" || m.URL != "" || m.Tiles != nil || m.Bounds != [2][2]float64{{-1, 0}, {0, 2}} {
					t.Errorf("mapImage = %+v", m)
				}
			}},
		{name: "C-DOC-10 list reads sources", reply: testutil.JSON(200, `{"data":[`+timeDocJSON+`],"hasMore":false}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Documents.List(ctx, "r1") },
			method: "GET", path: pR + "/docs",
			check: func(t *testing.T, v any) {
				if s := v.(*Page[Document]).Items[0].Sources; len(s) != 2 || s[1] != (DocRef{"r3", "d3"}) {
					t.Errorf("sources = %+v", s)
				}
			}},
	})
}
