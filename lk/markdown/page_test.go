package markdown_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

const pageJSON = `{"id":"d1","version":"v1","schema":1,"text":"Candy Kingdom\nRuled by PB.",
"blocks":[{"kind":"heading","id":"h1","level":1,"content":[{"text":"Candy Kingdom"}]},
{"kind":"paragraph","id":"p1","content":[{"text":"Ruled by PB."}]},
{"kind":"unsupported","id":"u1"}]}`

var doc = lk.DocRef{ResourceID: "r1", DocumentID: "d1"}

func newClient(t *testing.T, srv *testutil.FakeServer) *lk.Client {
	t.Helper()
	c, err := lk.New("test-key", lk.WithBaseURL(srv.URL), lk.WithProjectID("p1"),
		lk.WithRateLimiter(nil), lk.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func getPage(t *testing.T) (*testutil.FakeServer, *lk.Client, *markdown.Page) {
	t.Helper()
	srv := testutil.NewFakeServer(t, testutil.JSON(http.StatusOK, pageJSON))
	c := newClient(t, srv)
	p, err := markdown.Get(context.Background(), c, doc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "v1" || len(p.Blocks) != 3 || p.Markdown == "" {
		t.Fatalf("Get = %+v", p)
	}
	if r := srv.Requests()[0]; r.Method != http.MethodGet || r.Path != "/api/v3/projects/p1/resources/r1/docs/d1/blocks" {
		t.Fatalf("Get sent %s %s", r.Method, r.Path)
	}
	return srv, c, p
}

func TestReplace_M_API_04_AlwaysSendsIfMatch(t *testing.T) {
	srv, c, p := getPage(t)
	srv.Enqueue(testutil.JSON(http.StatusOK, pageJSON))
	// A caller's own IfMatch can't replace the base version.
	_, rep, err := markdown.Replace(context.Background(), c, doc, p, p.Markdown, markdown.ReplaceOptions{}, lk.IfMatch("other"))
	if err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()[1]
	if r.Method != http.MethodPut || r.Path != "/api/v3/projects/p1/resources/r1/docs/d1/blocks" {
		t.Fatalf("Replace sent %s %s", r.Method, r.Path)
	}
	if got := r.Header.Get("If-Match"); got != "v1" {
		t.Errorf("If-Match = %q, want v1", got)
	}
	var body struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(p.Blocks)
	if string(body.Blocks) != string(want) {
		t.Errorf("unchanged Markdown wrote\n%s\nwant\n%s", body.Blocks, want)
	}
	if len(rep.Kept) != 3 || len(rep.Changed)+len(rep.Added)+len(rep.Removed) != 0 {
		t.Errorf("report = %+v", rep)
	}

	t.Run("M-API-04 version mismatch surfaces", func(t *testing.T) {
		srv.Enqueue(testutil.APIError(http.StatusPreconditionFailed, "VERSION_MISMATCH", "req1"))
		_, _, err := markdown.Replace(context.Background(), c, doc, p, p.Markdown, markdown.ReplaceOptions{})
		if !errors.Is(err, lk.ErrVersionMismatch) {
			t.Errorf("err = %v, want ErrVersionMismatch", err)
		}
	})
	t.Run("M-API-04 no base", func(t *testing.T) {
		for _, b := range []*markdown.Page{nil, {Markdown: p.Markdown, Blocks: p.Blocks}} {
			if _, _, err := markdown.Replace(context.Background(), c, doc, b, "x", markdown.ReplaceOptions{}); !errors.Is(err, markdown.ErrNoBase) {
				t.Errorf("err = %v, want ErrNoBase", err)
			}
		}
	})
	if n := len(srv.Requests()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

func TestReplace_M_API_05_DryRunSendsNothing(t *testing.T) {
	srv, c, p := getPage(t)
	md := p.Markdown + "\n\nA new paragraph.\n"
	got, rep, err := markdown.Replace(context.Background(), c, doc, p, md, markdown.ReplaceOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("dry run sent %d requests after Get", n-1)
	}
	if got.Version != "v1" || len(got.Blocks) != 4 || len(rep.Added) != 1 || len(rep.Kept) != 3 {
		t.Errorf("dry run = %+v, report %+v", got, rep)
	}
	if got.Text != "Candy Kingdom\nRuled by PB.\nA new paragraph." {
		t.Errorf("dry run text = %q", got.Text)
	}
}
