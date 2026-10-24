package lk

import (
	"context"
	"errors"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const templateItem = `{"id":"t1","parentId":"templates","name":"Kingdom Template","tags":["Kingdom"],"aliases":[],"rank":"a0","isHidden":false,"hasChildren":true,"documents":[]}`

func TestTemplates(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-TPL-01 list first page without query", reply: testutil.JSON(200, `{"data":[`+templateItem+`],"hasMore":true,"nextCursor":"c2"}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Templates.List(ctx, TemplateListParams{}) },
			method: "GET", path: pP + "/templates",
			check: func(t *testing.T, v any) {
				p := v.(*Page[ResourceListItem])
				if len(p.Items) != 1 || p.Items[0].ParentID != ParentTemplates || !p.Items[0].HasChildren || !p.HasMore || p.NextCursor != "c2" {
					t.Errorf("page = %+v", p)
				}
			}},
		{name: "C-TPL-01 list with limit and cursor", reply: testutil.JSON(200, `{"data":[],"hasMore":false,"nextCursor":null}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Templates.List(ctx, TemplateListParams{Limit: 10, Cursor: "c2"})
			},
			method: "GET", path: pP + "/templates", query: "cursor=c2&limit=10"},
		{name: "C-TPL-02 save as template with defaults", reply: testutil.JSON(201, resourceJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.SaveAsTemplate(ctx, "r1", SaveAsTemplate{})
			},
			method: "POST", path: pR + "/save-as-template", keyed: true, body: `{}`,
			check: func(t *testing.T, v any) {
				if r := v.(*Resource); r.ID != "r1" || len(r.Documents) != 1 {
					t.Errorf("resource = %+v", r)
				}
			}},
		{name: "C-TPL-02 save as template with every field", reply: testutil.JSON(201, resourceJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.SaveAsTemplate(ctx, "r1", SaveAsTemplate{Name: Some("Kingdom Template"), IncludeChildren: Some(true),
					After: Some("t0"), ID: Some("tplkingd")})
			},
			method: "POST", path: pR + "/save-as-template", keyed: true,
			body: `{"after":"t0","id":"tplkingd","includeChildren":true,"name":"Kingdom Template"}`},
		{name: "C-TPL-02 create from template with only the template", reply: testutil.JSON(201, resourceJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.CreateFromTemplate(ctx, FromTemplate{TemplateID: "t1"})
			},
			method: "POST", path: pP + "/resources/from-template", keyed: true, body: `{"templateId":"t1"}`},
		{name: "C-TPL-02 create from template with every field", reply: testutil.JSON(201, resourceJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Resources.CreateFromTemplate(ctx, FromTemplate{TemplateID: "t1", Name: Some("Realm"), ParentID: Some("r0"),
					IncludeChildren: Some(false), Before: Some("r5"), ID: Some("realm001")})
			},
			method: "POST", path: pP + "/resources/from-template", keyed: true,
			body: `{"before":"r5","id":"realm001","includeChildren":false,"name":"Realm","parentId":"r0","templateId":"t1"}`},
	})
}

func TestTemplates_C_TPL_01_AllFollowsCursor(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(200, `{"data":[`+templateItem+`],"hasMore":true,"nextCursor":"c2"}`),
		testutil.JSON(200, `{"data":[`+templateItem+`],"hasMore":false,"nextCursor":null}`),
	)
	c, _ := newTestClient(t, srv)
	n := 0
	for _, err := range c.Templates.All(context.Background(), TemplateListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if reqs := srv.Requests(); n != 2 || len(reqs) != 2 || reqs[1].RawQuery != "cursor=c2" {
		t.Errorf("n = %d, requests %+v", n, reqs)
	}
}

func TestTemplates_C_TPL_02_RefusesIfMatch(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	if _, err := c.Resources.SaveAsTemplate(ctx, "r1", SaveAsTemplate{}, IfMatch("v")); !errors.Is(err, ErrIfMatchNotAllowed) {
		t.Errorf("SaveAsTemplate err = %v", err)
	}
	if _, err := c.Resources.CreateFromTemplate(ctx, FromTemplate{TemplateID: "t1"}, IfMatch("v")); !errors.Is(err, ErrIfMatchNotAllowed) {
		t.Errorf("CreateFromTemplate err = %v", err)
	}
}
