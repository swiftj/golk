package lk

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

func item(id string) string {
	return `{"id":"` + id + `","name":"` + id + `","tags":[],"aliases":[],"documents":[],"hasChildren":false}`
}

func listPage(next string, hasMore *bool, ids ...string) testutil.Reply {
	body := `{"data":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += item(id)
	}
	body += `]`
	if hasMore != nil {
		if *hasMore {
			body += `,"hasMore":true`
		} else {
			body += `,"hasMore":false`
		}
	}
	if next != "" {
		body += `,"nextCursor":"` + next + `"`
	} else {
		body += `,"nextCursor":null`
	}
	return testutil.JSON(200, body+"}")
}

var (
	yes = func() *bool { b := true; return &b }()
	no  = func() *bool { b := false; return &b }()
)

func collect(t *testing.T, c *Client, p ResourceListParams) ([]string, error) {
	t.Helper()
	var ids []string
	for r, err := range c.Resources.All(context.Background(), p) {
		if err != nil {
			return ids, err
		}
		ids = append(ids, r.ID)
	}
	return ids, nil
}

func TestPage_C_PAGE_01_C_PAGE_02_C_PAGE_07_CursorAndFilters(t *testing.T) {
	cursor := "opaque+/=cursor%20" // returned verbatim, never parsed
	srv := testutil.NewFakeServer(t, listPage(cursor, yes, "a", "b"), listPage("", no, "c"))
	c, _ := newTestClient(t, srv)
	ids, err := collect(t, c, ResourceListParams{ParentID: "ooo", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("ids = %v", ids)
	}
	r := srv.Requests()
	q0, _ := url.ParseQuery(r[0].RawQuery)
	q1, _ := url.ParseQuery(r[1].RawQuery)
	if q0.Get("parentId") != "ooo" || q0.Get("limit") != "2" || q0.Has("cursor") {
		t.Errorf("first page query = %s", r[0].RawQuery)
	}
	if q1.Get("cursor") != cursor || q1.Get("parentId") != "ooo" || q1.Get("limit") != "2" {
		t.Errorf("second page query = %s", r[1].RawQuery)
	}
	if r[0].Path != "/api/v3/projects/p1/resources" {
		t.Errorf("path = %s", r[0].Path)
	}
}

func TestPage_C_PAGE_01_ProjectsPaged(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(200, `{"data":[{"id":"p1","name":"A"}],"hasMore":true,"nextCursor":"n1"}`),
		testutil.JSON(200, `{"data":[{"id":"p2","name":"B"}],"hasMore":false,"nextCursor":null}`))
	c, _ := newTestClient(t, srv)
	var ids []string
	for p, err := range c.Projects.All(context.Background(), ProjectListParams{Limit: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if len(ids) != 2 || srv.Requests()[1].RawQuery != "cursor=n1&limit=1" {
		t.Fatalf("ids = %v, second query %q", ids, srv.Requests()[1].RawQuery)
	}
}

func TestPage_C_PAGE_03_C_PAGE_04_IteratorAndManualNext(t *testing.T) {
	srv := testutil.NewFakeServer(t, listPage("c1", yes, "a", "b"), listPage("", no, "c"))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	p, err := c.Resources.List(ctx, ResourceListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || !p.HasMore || p.NextCursor != "c1" {
		t.Fatalf("page 1 = %+v", p)
	}
	p2, err := p.Next(ctx)
	if err != nil || len(p2.Items) != 1 || p2.HasMore || p2.NextCursor != "" {
		t.Fatalf("page 2 = %+v, %v", p2, err)
	}
	p3, err := p2.Next(ctx)
	if p3 != nil || err != nil {
		t.Fatalf("after last page: %v, %v", p3, err)
	}
	if n := len(srv.Requests()); n != 2 {
		t.Fatalf("%d requests", n)
	}
}

func TestPage_C_PAGE_06_EarlyBreakAndError(t *testing.T) {
	srv := testutil.NewFakeServer(t, listPage("c1", yes, "a", "b"))
	c, _ := newTestClient(t, srv)
	for _, err := range c.Resources.All(context.Background(), ResourceListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("early break sent %d requests", n)
	}

	srv = testutil.NewFakeServer(t, listPage("c1", yes, "a"), testutil.APIError(400, "VALIDATION_ERROR", "r"))
	c, _ = newTestClient(t, srv)
	var errs int
	var ids []string
	for r, err := range c.Resources.All(context.Background(), ResourceListParams{}) {
		if err != nil {
			errs++
			continue
		}
		ids = append(ids, r.ID)
	}
	if errs != 1 || len(ids) != 1 {
		t.Fatalf("errs = %d, ids = %v", errs, ids)
	}
}

func TestPage_C_PAGE_08_Termination(t *testing.T) {
	// hasMore true but no cursor: an error, not a silent truncation.
	srv := testutil.NewFakeServer(t, listPage("", yes, "a"))
	c, _ := newTestClient(t, srv)
	ids, err := collect(t, c, ResourceListParams{})
	if !errors.Is(err, ErrPaginationInconsistent) || len(ids) != 1 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	// hasMore false wins over a stray cursor.
	srv = testutil.NewFakeServer(t, listPage("stray", no, "a"))
	c, _ = newTestClient(t, srv)
	if ids, err := collect(t, c, ResourceListParams{}); err != nil || len(ids) != 1 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	// hasMore absent: the cursor decides.
	srv = testutil.NewFakeServer(t, listPage("c1", nil, "a"), listPage("", nil, "b"))
	c, _ = newTestClient(t, srv)
	if ids, err := collect(t, c, ResourceListParams{}); err != nil || len(ids) != 2 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
}

func TestPage_C_PAGE_09_LimitNotClamped(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(400, "VALIDATION_ERROR", "r"))
	c, _ := newTestClient(t, srv)
	_, err := c.Resources.List(context.Background(), ResourceListParams{Limit: 500})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v", err)
	}
	if q := srv.Requests()[0].RawQuery; q != "limit=500" {
		t.Fatalf("query = %q", q)
	}
}
