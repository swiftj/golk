package lk

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

const oneProject = `{"data":[{"id":"proj1","name":"Ooo","isPublic":false,"createdAt":"2026-01-02T03:04:05Z"}],"hasMore":false,"nextCursor":null}`

func unresolved(t *testing.T, srv *testutil.FakeServer, opts ...Option) *Client {
	t.Helper()
	c, _ := newTestClient(t, srv, append([]Option{WithProjectID("")}, opts...)...)
	return c
}

func TestProj_C_PROJ_01_ResolvedOnceAndCached(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, oneProject), testutil.JSON(200, okResource), testutil.JSON(200, okResource))
	c := unresolved(t, srv)
	ctx := context.Background()
	for range 2 {
		if _, err := c.Resources.Get(ctx, "r1"); err != nil {
			t.Fatal(err)
		}
	}
	r := srv.Requests()
	if len(r) != 3 || r[0].Path != "/api/v3/projects" || r[1].Path != "/api/v3/projects/proj1/resources/r1" || r[2].Path != r[1].Path {
		t.Fatalf("requests = %+v", r)
	}
}

func TestProj_C_PROJ_02_ExplicitProjectSkipsResolution(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource), testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv, WithProjectID("given"))
	_, _ = c.Resources.Get(context.Background(), "r1")
	u := unresolved(t, srv)
	_, _ = u.Resources.Get(context.Background(), "r1", ForProject("per-call"))
	r := srv.Requests()
	if r[0].Path != "/api/v3/projects/given/resources/r1" || r[1].Path != "/api/v3/projects/per-call/resources/r1" {
		t.Fatalf("paths %s %s", r[0].Path, r[1].Path)
	}
	// A per-call project does not populate the cache (D-004, A-14).
	if u.projectID != "" {
		t.Fatal("ForProject populated the cache")
	}
}

func TestProj_C_PROJ_03_FailureNotCached(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(401, "UNAUTHENTICATED", "a"),
		testutil.JSON(200, oneProject), testutil.JSON(200, okResource))
	c := unresolved(t, srv)
	ctx := context.Background()
	if _, err := c.Resources.Get(ctx, "r1"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("first call err = %v", err)
	}
	if _, err := c.Resources.Get(ctx, "r1"); err != nil {
		t.Fatalf("second call should resolve again: %v", err)
	}
}

func TestProj_C_PROJ_04_Ambiguous(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"a"},{"id":"b"}],"hasMore":false,"nextCursor":null}`,
		`{"data":[{"id":"a"}],"hasMore":true,"nextCursor":"n"}`,
	} {
		srv := testutil.NewFakeServer(t, testutil.JSON(200, body))
		c := unresolved(t, srv)
		if _, err := c.Resources.Get(context.Background(), "r1"); !errors.Is(err, ErrAmbiguousProject) {
			t.Fatalf("%s: err = %v", body, err)
		}
	}
}

// blockedProjects answers projects.list only after release is closed, and
// signals arrived when the request is in.
func blockedProjects(arrived chan<- struct{}, release <-chan struct{}) testutil.Reply {
	return testutil.Reply{Handler: func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oneProject))
	}}
}

func TestProj_C_PROJ_05_ConcurrentCallersShareOneResolution(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	srv := testutil.NewFakeServer(t, blockedProjects(arrived, release))
	const n = 10
	for range n {
		srv.Enqueue(testutil.JSON(200, okResource))
	}
	c := unresolved(t, srv)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			_, err := c.Resources.Get(context.Background(), "r1")
			errs <- err
		})
	}
	<-arrived // one resolution is in flight; the rest wait on it or the cache
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lists := 0
	for _, r := range srv.Requests() {
		if r.Path == "/api/v3/projects" {
			lists++
		}
	}
	if lists != 1 {
		t.Fatalf("%d projects.list calls, want 1", lists)
	}
}

func TestProj_C_PROJ_05_WaiterCancelReleasesOnlyItself(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	srv := testutil.NewFakeServer(t, blockedProjects(arrived, release), testutil.JSON(200, okResource))
	c := unresolved(t, srv)
	leaderDone := make(chan error, 1)
	go func() {
		_, err := c.Resources.Get(context.Background(), "r1")
		leaderDone <- err
	}()
	<-arrived // the leader's resolution is in flight
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.resolveProjectID(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter err = %v", err)
	}
	close(release)
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader failed after a waiter gave up: %v", err)
	}
}

func TestProj_C_PROJ_06_ProjectReturnsDetail(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, oneProject),
		testutil.JSON(200, `{"id":"proj1","name":"Ooo","isPublic":true,"createdAt":"2026-01-02T04:04:05+01:00","coverImageUrl":null,"coverGradient":null,"homeResourceId":"home1"}`))
	c := unresolved(t, srv)
	p, err := c.Project(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Project{ID: "proj1", Name: "Ooo", IsPublic: true, HomeResourceID: "home1", CreatedAt: epoch}
	if *p != want || p.CreatedAt.Location() != time.UTC {
		t.Fatalf("got %+v, want %+v", *p, want)
	}
	if r := srv.Requests(); r[1].Path != "/api/v3/projects/proj1" {
		t.Fatalf("get path = %s", r[1].Path)
	}
	if _, err := c.Projects.Get(context.Background(), ""); err == nil {
		t.Fatal("Projects.Get(\"\") read a project")
	}
}

func TestProj_C_PROJ_07_NoProject(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"data":[],"hasMore":false,"nextCursor":null}`))
	c := unresolved(t, srv)
	if _, err := c.Project(context.Background()); !errors.Is(err, ErrNoProject) {
		t.Fatalf("err = %v", err)
	}
}
