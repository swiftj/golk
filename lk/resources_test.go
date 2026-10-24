package lk

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/gen"
)

// fullResource sets every field the spec defines for Resource, so the
// mapping test notices a field the public type drops.
const fullResource = `{
 "id":"r1","name":"Ice Kingdom","parentId":"ooo","tags":["Kingdom"],"aliases":["Land of Ice"],
 "iconGlyph":"snowflake","iconColor":"#4caf50","iconShape":"shield","iconSize":"small","rank":"a0V",
 "isHidden":true,"isLocked":true,"isFullWidth":true,"showPropertyBar":true,"imageUrl":"https://x/i.png",
 "banner":{"enabled":true,"url":"https://x/b.png","yPosition":0.5},"permissions":{"MEMBER":2},
 "documents":[{"id":"d1","name":"Main","type":"page","rank":"a0","isHidden":true,"isFullWidth":true,"calendarId":"cal",
   "sources":[{"resourceId":"r2","documentId":"d2"}],
   "mapImage":{"assetId":"a1","url":"https://x/m.png","width":2,"height":1,"bounds":[[-1,0],[0,2]],"tiles":{"urlTemplate":"https://x/{z}/{x}/{y}","maxNativeZoom":1}},
   "createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z","deletedAt":"2026-01-02T03:04:05Z"}],
 "properties":[{"kind":"keyValue","id":"p1","rows":[]}],
 "ancestors":[{"id":"ooo","name":"The Land of Ooo"}],
 "createdAt":"2026-01-02T04:04:05+01:00","updatedAt":"2026-01-02T03:04:05Z"}`

type sent struct {
	method, path, query, body string
}

func lastRequest(t *testing.T, srv *testutil.FakeServer) sent {
	t.Helper()
	r := srv.Requests()
	l := r[len(r)-1]
	return sent{l.Method, l.Path, l.RawQuery, string(l.Body)}
}

func TestRes_C_RES_01_C_RES_09_ListRequests(t *testing.T) {
	srv := testutil.NewFakeServer(t, listPage("", no, "a"), listPage("", no, "b"),
		testutil.JSON(200, `{"data":[{"id":"p1"}],"hasMore":false,"nextCursor":null}`))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	_, _ = c.Resources.List(ctx, ResourceListParams{})
	if got := lastRequest(t, srv); got != (sent{"GET", "/api/v3/projects/p1/resources", "", ""}) {
		t.Errorf("list = %+v", got)
	}
	_, _ = c.Resources.List(ctx, ResourceListParams{ParentID: ParentRoot, Limit: 10})
	if got := lastRequest(t, srv); got.query != "limit=10&parentId=root" {
		t.Errorf("list root query = %q", got.query)
	}
	_, _ = c.Projects.List(ctx, ProjectListParams{})
	if got := lastRequest(t, srv); got != (sent{"GET", "/api/v3/projects", "", ""}) {
		t.Errorf("projects.list = %+v", got)
	}
}

func TestRes_C_RES_02_C_WIRE_06_GetMapsEveryField(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, fullResource))
	c, _ := newTestClient(t, srv)
	r, err := c.Resources.Get(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got := lastRequest(t, srv); got != (sent{"GET", "/api/v3/projects/p1/resources/r1", "", ""}) {
		t.Errorf("request = %+v", got)
	}
	assertNoZeroFields(t, reflect.ValueOf(*r), "Resource")
	if r.CreatedAt != epoch || r.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v in UTC", r.CreatedAt, epoch)
	}
	if r.Permissions["MEMBER"] != PermissionCanEdit || r.IconShape != IconShapeShield || r.Documents[0].Type != DocumentPage {
		t.Errorf("enum mapping: %+v", r)
	}
	if _, ok := r.Properties[0].(*blocks.KeyValueProperty); !ok {
		t.Errorf("property = %+v", r.Properties[0])
	}
}

// assertNoZeroFields fails for any zero field, recursively through structs
// and the first element of slices.
func assertNoZeroFields(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[time.Time]() {
			if v.Interface().(time.Time).IsZero() {
				t.Errorf("%s is zero", path)
			}
			return
		}
		for i := range v.NumField() {
			assertNoZeroFields(t, v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	case reflect.Slice:
		if v.Len() == 0 {
			t.Errorf("%s is empty", path)
			return
		}
		if v.Type().Elem().Kind() == reflect.Struct {
			assertNoZeroFields(t, v.Index(0), path+"[0]")
		}
	case reflect.Pointer:
		if v.IsNil() {
			t.Errorf("%s is nil", path)
			return
		}
		assertNoZeroFields(t, v.Elem(), path)
	default:
		if v.IsZero() && v.Kind() != reflect.Int { // PermissionNoAccess etc. are 0
			t.Errorf("%s is zero", path)
		}
	}
}

// When the spec adds a field to a response schema, regenerating adds it to
// the wire type; this test then fails until the public type maps it.
func TestRes_C_WIRE_07_SpecDriftDetected(t *testing.T) {
	fields := func(v any) []string {
		var out []string
		rt := reflect.TypeOf(v)
		for i := range rt.NumField() {
			out = append(out, rt.Field(i).Name)
		}
		slices.Sort(out)
		return out
	}
	check := func(name string, wire any, mapped ...string) {
		t.Helper()
		slices.Sort(mapped)
		if got := fields(wire); !slices.Equal(got, mapped) {
			t.Errorf("%s wire fields changed:\n got: %v\nmapped: %v", name, got, mapped)
		}
	}
	check("Resource", gen.Resource{}, "Aliases", "Banner", "CreatedAt", "Documents", "IconColor", "IconGlyph", "IconShape",
		"IconSize", "ID", "ImageURL", "IsFullWidth", "IsHidden", "IsLocked", "Name", "ParentID", "Permissions",
		"Properties", "Rank", "ShowPropertyBar", "Tags", "UpdatedAt")
	check("ResourceWithAncestors", gen.ResourceWithAncestors{}, append(fields(gen.Resource{}), "Ancestors")...)
	check("ResourceListItem", gen.ResourceListItem{}, "Aliases", "CreatedAt", "Documents", "HasChildren", "IconColor",
		"IconGlyph", "IconShape", "IconSize", "ID", "IsHidden", "Name", "ParentID", "Rank", "Tags", "UpdatedAt")
	// MapImage and Sources belong to maps and timelines (phase 4).
	check("Document", gen.Document{}, "CalendarID", "CreatedAt", "DeletedAt", "ID", "IsFullWidth", "IsHidden",
		"MapImage", "Name", "Rank", "Sources", "Type", "UpdatedAt")
	check("ResourceCreateParams", gen.ResourceCreateParams{}, fieldsOfPublic(ResourceCreate{})...)
	check("ResourceUpdateParams", gen.ResourceUpdateParams{}, fieldsOfPublic(ResourceUpdate{})...)
	check("ResourceMoveParams", gen.ResourceMoveParams{}, fieldsOfPublic(ResourceMove{})...)
	check("ResourceDocumentParams", gen.ResourceDocumentParams{}, fieldsOfPublic(ResourceDocument{})...)
	check("ProjectDetail", gen.ProjectDetail{}, "CoverGradient", "CoverImageURL", "CreatedAt", "HomeResourceID", "ID",
		"IsPublic", "LastActivityAt", "Name")
}

func fieldsOfPublic(v any) []string {
	var out []string
	rt := reflect.TypeOf(v)
	for i := range rt.NumField() {
		out = append(out, rt.Field(i).Name)
	}
	return out
}

func TestRes_C_RES_03_C_WIRE_01_CreateExactBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ResourceCreate
		want string
	}{
		{"C-WIRE-01 name only", ResourceCreate{Name: "The Tree Fort"}, `{"name":"The Tree Fort"}`},
		{"C-RES-03 all fields", ResourceCreate{
			ID: Some("r0123456789abcdefghijklm"), Name: "Ice Kingdom", ParentID: Some("ooo"), Before: Some("fire"),
			Tags: Some([]string{"Kingdom", "Cold"}), Aliases: Some([]string{"Land of Ice"}),
			IconGlyph: Some("globe"), IconColor: Some("#4caf50"), IconShape: Some(IconShapePin), IconSize: Some(IconSizeSmall),
			IsHidden: Some(false), IsLocked: Some(true), Banner: Some(Banner{Enabled: true, URL: "https://x/b.png"}),
			Documents: Some([]ResourceDocument{{Name: Some("Overview")}, {Name: Some("Map"), Type: Some(DocumentMap)}}),
		}, mustJSON(t, `{"aliases":["Land of Ice"],"banner":{"enabled":true,"url":"https://x/b.png","yPosition":0},
			"before":"fire","documents":[{"name":"Overview"},{"name":"Map","type":"map"}],"iconColor":"#4caf50",
			"iconGlyph":"globe","iconShape":"pin","iconSize":"small","id":"r0123456789abcdefghijklm","isHidden":false,
			"isLocked":true,"name":"Ice Kingdom","parentId":"ooo","tags":["Kingdom","Cold"]}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t, testutil.JSON(201, resourceJSON))
			c, _ := newTestClient(t, srv)
			r, err := c.Resources.Create(context.Background(), tc.in)
			if err != nil {
				t.Fatal(err)
			}
			got := lastRequest(t, srv)
			if got.method != "POST" || got.path != "/api/v3/projects/p1/resources" {
				t.Errorf("request = %+v", got)
			}
			assertJSON(t, []byte(got.body), tc.want)
			if r.ID != "r1" || r.Documents[0].Type != DocumentBlank {
				t.Errorf("result = %+v", r)
			}
		})
	}
}

func TestRes_C_RES_04_CreateMany(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(201, `{"resources":[`+okResource+`,`+strings.Replace(okResource, `"r1"`, `"r2"`, 1)+`]}`))
	c, _ := newTestClient(t, srv)
	out, err := c.Resources.CreateMany(context.Background(), []ResourceCreate{
		{ID: Some("ooo00000"), Name: "The Land of Ooo"},
		{ID: Some("candy000"), Name: "Candy Kingdom", ParentID: Some("ooo00000")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lastRequest(t, srv)
	if got.method != "POST" || got.path != "/api/v3/projects/p1/resources/batch" {
		t.Errorf("request = %+v", got)
	}
	assertJSON(t, []byte(got.body), `{"resources":[{"id":"ooo00000","name":"The Land of Ooo"},{"id":"candy000","name":"Candy Kingdom","parentId":"ooo00000"}]}`)
	if len(out) != 2 || out[0].ID != "r1" || out[1].ID != "r2" {
		t.Fatalf("out = %+v", out)
	}
}

func TestRes_C_RES_05_C_WIRE_01_C_WIRE_04_UpdateExactBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ResourceUpdate
		want string
	}{
		{"C-WIRE-01 nothing set", ResourceUpdate{}, `{}`},
		{"C-WIRE-01 name only", ResourceUpdate{Name: Some("The Ice Kingdom")}, `{"name":"The Ice Kingdom"}`},
		{"C-WIRE-01 false is sent when set", ResourceUpdate{IsHidden: Some(false)}, `{"isHidden":false}`},
		{"C-WIRE-04 empty list clears", ResourceUpdate{Tags: Some([]string{}), Aliases: Some[[]string](nil)}, `{"aliases":[],"tags":[]}`},
		{"C-WIRE-04 list replaces", ResourceUpdate{Tags: Some([]string{"Kingdom"})}, `{"tags":["Kingdom"]}`},
		{"C-RES-05 permissions", ResourceUpdate{Permissions: Some(map[string]PermissionLevel{"MEMBER": PermissionNoAccess})}, `{"permissions":{"MEMBER":0}}`},
		{"C-RES-05 icon", ResourceUpdate{IconShape: Some(IconShapeNone), IconSize: Some(IconSizeMedium)}, `{"iconShape":"none","iconSize":"medium"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
			c, _ := newTestClient(t, srv)
			if _, err := c.Resources.Update(context.Background(), "r1", tc.in); err != nil {
				t.Fatal(err)
			}
			got := lastRequest(t, srv)
			if got.method != "PATCH" || got.path != "/api/v3/projects/p1/resources/r1" {
				t.Errorf("request = %+v", got)
			}
			assertJSON(t, []byte(got.body), tc.want)
		})
	}
}

func TestRes_C_RES_06_C_WIRE_02_C_WIRE_03_MoveExactBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ResourceMove
		want string
	}{
		{"C-WIRE-03 to root", ResourceMove{ParentID: Null[string]()}, `{"parentId":null}`},
		{"C-WIRE-02 to parent", ResourceMove{ParentID: Value("ooo")}, `{"parentId":"ooo"}`},
		{"C-WIRE-02 reorder only", ResourceMove{After: Some("candy")}, `{"after":"candy"}`},
		{"C-WIRE-02 nothing", ResourceMove{}, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
			c, _ := newTestClient(t, srv)
			if _, err := c.Resources.Move(context.Background(), "ice", tc.in); err != nil {
				t.Fatal(err)
			}
			got := lastRequest(t, srv)
			if got.method != "POST" || got.path != "/api/v3/projects/p1/resources/ice/move" {
				t.Errorf("request = %+v", got)
			}
			assertJSON(t, []byte(got.body), tc.want)
		})
	}
}

func TestRes_C_RES_07_DeleteAndCascade(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(409, "HAS_CHILDREN", "r"), testutil.Reply{Status: 204})
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	err := c.Resources.Delete(ctx, "ooo", ResourceDeleteParams{})
	if !errors.Is(err, ErrHasChildren) {
		t.Fatalf("err = %v", err)
	}
	if got := lastRequest(t, srv); got != (sent{"DELETE", "/api/v3/projects/p1/resources/ooo", "", ""}) {
		t.Errorf("delete = %+v", got)
	}
	if err := c.Resources.Delete(ctx, "ooo", ResourceDeleteParams{Cascade: true}); err != nil {
		t.Fatal(err)
	}
	if got := lastRequest(t, srv); got != (sent{"DELETE", "/api/v3/projects/p1/resources/ooo", "cascade=true", ""}) {
		t.Errorf("cascade delete = %+v", got)
	}
}

func TestRes_C_RES_08_Restore(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	r, err := c.Resources.Restore(context.Background(), "ooo")
	if err != nil || r.ID != "r1" {
		t.Fatalf("%v %+v", err, r)
	}
	got := srv.Requests()[0]
	if got.Method != "POST" || got.Path != "/api/v3/projects/p1/resources/ooo/restore" || len(got.Body) != 0 || got.Header.Get("Content-Type") != "" {
		t.Errorf("restore = %s %s body %q", got.Method, got.Path, got.Body)
	}
}

func TestRes_C_RES_10_PathParamsEscapedAndRequired(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	_, _ = c.Resources.Get(ctx, "a/b?c")
	if got := srv.Requests()[0].Path; got != "/api/v3/projects/p1/resources/a%2Fb%3Fc" {
		t.Errorf("path = %s", got)
	}
	// An empty ID never turns into a call on another endpoint.
	if _, err := c.Resources.Get(ctx, ""); err == nil {
		t.Error("Get(\"\") sent")
	}
	if err := c.Resources.Delete(ctx, "", ResourceDeleteParams{Cascade: true}); err == nil {
		t.Error("Delete(\"\") sent")
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("%d requests", n)
	}
}

func TestWire_C_WIRE_05_Headers(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(201, okResource), testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv, WithUserAgent("my-importer/1.0"))
	ctx := context.Background()
	_, _ = c.Resources.Create(ctx, ResourceCreate{Name: "x"})
	_, _ = c.Resources.Get(ctx, "r1")
	r := srv.Requests()
	for _, rq := range r {
		if rq.Header.Get("Accept") != "application/json" || rq.Header.Get("User-Agent") != "my-importer/1.0 golk/"+Version {
			t.Errorf("%s headers: %v", rq.Method, rq.Header)
		}
	}
	if r[0].Header.Get("Content-Type") != "application/json" {
		t.Error("POST lacks Content-Type")
	}
	if r[1].Header.Get("Content-Type") != "" {
		t.Error("GET has Content-Type")
	}
}

func TestWire_C_WIRE_07_ToleratesUnknownResponseFields(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, strings.Replace(okResource, `{"id"`, `{"futureField":{"x":1},"id"`, 1)))
	c, _ := newTestClient(t, srv)
	if _, err := c.Resources.Get(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
}

var cuid2 = regexp.MustCompile(`^[a-z][a-z0-9]{23}$`)

func TestContent_C_CONTENT_01_NewID(t *testing.T) {
	seen := map[string]bool{}
	for range 2000 {
		id := NewID()
		if !cuid2.MatchString(id) || seen[id] {
			t.Fatalf("bad or repeated id %q", id)
		}
		seen[id] = true
	}
}

func TestContent_C_CONTENT_05_SortByRank(t *testing.T) {
	items := []ResourceListItem{{ID: "c", Rank: "a1"}, {ID: "a", Rank: "Zz"}, {ID: "b", Rank: "a0"}, {ID: "d", Rank: "a0V"}}
	SortByRank(items)
	var got []string
	for _, it := range items {
		got = append(got, it.ID)
	}
	// Byte order: "Zz" < "a0" < "a0V" < "a1" (a locale sort would put Zz last).
	if !slices.Equal(got, []string{"a", "b", "d", "c"}) {
		t.Fatalf("order = %v", got)
	}
}

func FuzzDecodeResource(f *testing.F) {
	f.Add([]byte(fullResource))
	f.Add([]byte(okResource))
	f.Add([]byte(`{"id":"r","properties":[{"kind":"brandNew","x":[1,2]}],"documents":[{"type":"hologram"}]}`))
	f.Add([]byte(`{"parentId":null,"banner":null,"createdAt":"not a time"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var w gen.ResourceWithAncestors
		if json.Unmarshal(data, &w) != nil {
			return
		}
		var r gen.Resource
		if json.Unmarshal(data, &r) != nil {
			return
		}
		res := resourceFromWire(r)
		// Every property is kept, typed or raw, never dropped.
		if len(res.Properties) != len(r.Properties) {
			t.Fatalf("properties dropped: %d -> %d", len(r.Properties), len(res.Properties))
		}
		for i, p := range res.Properties {
			if string(p.Kind()) != r.Properties[i].Kind {
				t.Fatalf("property %d kind changed", i)
			}
		}
		// Re-encoding the wire type and decoding again is stable.
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var again gen.Resource
		if err := json.Unmarshal(b, &again); err != nil {
			t.Fatalf("re-decode: %v\n%s", err, b)
		}
		b2, _ := json.Marshal(again)
		if string(b) != string(b2) {
			t.Fatalf("unstable round trip\n%s\n%s", b, b2)
		}
	})
}
