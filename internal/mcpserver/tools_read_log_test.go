package mcpserver

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk"
)

// rtSyncBuffer is a log sink safe for the server's goroutines.
type rtSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *rtSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *rtSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// rtHarness is newHarness with a chosen config and a captured debug log.
func rtHarness(t *testing.T, cfg Config) (*harness, *rtSyncBuffer) {
	t.Helper()
	logs := &rtSyncBuffer{}
	h := &harness{t: t, lk: testutil.NewFakeServer(t), guard: &fakeGuard{}}
	d := Deps{
		Guard: h.guard, Version: "test",
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		ClientOptions: []lk.Option{
			lk.WithBaseURL(h.lk.URL), lk.WithProjectID("p1"), lk.WithRateLimiter(nil), lk.WithMaxRetries(0),
		},
	}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(cfg, d).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	h.cs = cs
	return h, logs
}

const (
	rtKey1 = "made-up-key-9d1e44"
	rtKey2 = "made-up-key-c06b3f"
	// rtSentence is page and property text that must never be logged.
	rtSentence = "The Gumball Guardians wake at moonrise"
)

func TestLogs_X_LOG_02_OneLinePerCallNoContent(t *testing.T) {
	h, logs := rtHarness(t, Config{}.WithKey(DefaultProject, rtKey1))
	h.lk.Enqueue(
		testutil.JSON(http.StatusOK, resourceJSON),
		testutil.JSON(http.StatusOK, `{"id":"d1","version":"v7","schema":1,"text":"x","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"`+rtSentence+`"}]}]}`),
		testutil.JSON(http.StatusOK, `{"id":"r5","name":"Gate","tags":[],"aliases":[],"rank":"a","isHidden":false,"permissions":{},"documents":[],`+
			`"properties":[{"kind":"textField","id":"pr1","title":"Lore","children":[{"kind":"paragraph","content":[{"text":"`+rtSentence+`"}]}]}]}`),
		testutil.JSON(http.StatusOK, `{"data":[]}`),
		testutil.APIError(http.StatusNotFound, "NOT_FOUND", "req-9"),
	)
	calls := []struct {
		tool string
		args map[string]any
		ok   bool
	}{
		{"read_page", map[string]any{"resource_id": "r1"}, true},
		{"get_resource", map[string]any{"resource_id": "r5"}, true},
		{"search", map[string]any{"query": "wake at moonrise"}, true},
		{"get_resource", map[string]any{"resource_id": "gone"}, false},
		{"search", map[string]any{"query": ""}, false},
	}
	for _, c := range calls {
		r := h.call(c.tool, c.args)
		if r.IsError == c.ok {
			t.Fatalf("%s: IsError %v: %s", c.tool, r.IsError, text(r))
		}
		if c.ok && !strings.Contains(text(r), rtSentence) && c.tool != "search" {
			t.Fatalf("%s: output lacks the content: %s", c.tool, text(r))
		}
	}
	got := logs.String()
	var lines []string
	for l := range strings.Lines(got) {
		if strings.Contains(l, `msg="tool call"`) {
			lines = append(lines, l)
		}
	}
	if len(lines) != len(calls) {
		t.Fatalf("%d tool call lines, want %d:\n%s", len(lines), len(calls), got)
	}
	for i, c := range calls {
		l := lines[i]
		for _, want := range []string{"tool=" + c.tool, "project=default", "duration=", "ok=" + map[bool]string{true: "true", false: "false"}[c.ok]} {
			if !strings.Contains(l, want) {
				t.Errorf("line %d lacks %q: %s", i, want, l)
			}
		}
	}
	if !strings.Contains(lines[3], "error=NOT_FOUND") {
		t.Errorf("failed call line lacks its code: %s", lines[3])
	}
	for _, secret := range []string{rtSentence, "moonrise", rtKey1, "Candy Kingdom", "Lore"} {
		if strings.Contains(got, secret) {
			t.Errorf("logs hold %q:\n%s", secret, got)
		}
	}
}

func TestLogs_X_LOG_02_ListProjects(t *testing.T) {
	h, logs := rtHarness(t, Config{}.WithKey(DefaultProject, rtKey1))
	h.lk.Enqueue(testutil.JSON(http.StatusOK, `{"id":"p1","name":"Ooo"}`))
	if r := h.call("list_projects", map[string]any{}); r.IsError {
		t.Fatal(text(r))
	}
	got := logs.String()
	for _, want := range []string{"tool=list_projects", "project=all", "duration=", "ok=true"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, rtKey1) {
		t.Error("log holds the key")
	}
}

// X-CFG-02: no key in tool output, tool errors or logs, including errors
// from the API and from project resolution.
func TestKeys_X_CFG_02_NotInOutputErrorsOrLogs(t *testing.T) {
	h, logs := rtHarness(t, Config{}.WithKey("first", rtKey1).WithKey("second", rtKey2))
	h.lk.Enqueue(
		testutil.JSON(http.StatusOK, `{"id":"p1","name":"Ooo"}`),
		testutil.APIError(http.StatusUnauthorized, "UNAUTHENTICATED", "req-1"),
		testutil.APIError(http.StatusUnauthorized, "UNAUTHENTICATED", "req-2"),
	)
	var all []string
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"list_projects", map[string]any{}},
		{"list_tags", map[string]any{"project": "second"}},
		{"list_tags", map[string]any{}},
		{"list_tags", map[string]any{"project": "nope"}},
	} {
		r := h.call(c.tool, c.args)
		all = append(all, text(r), string(mustJSON(r.StructuredContent)))
	}
	all = append(all, logs.String())
	for _, s := range all {
		if strings.Contains(s, rtKey1) || strings.Contains(s, rtKey2) {
			t.Errorf("a key leaked: %s", s)
		}
	}
}

// X-PROJ-01 through the tools: the project argument picks the key, and a
// call that names no project with several configured fails without a
// request.
func TestTools_X_PROJ_01_ProjectArgument(t *testing.T) {
	h, _ := rtHarness(t, Config{}.WithKey("first", rtKey1).WithKey("second", rtKey2))
	r := h.call("list_tags", map[string]any{})
	if !r.IsError || !strings.Contains(text(r), "first, second") {
		t.Errorf("no project: %v %s", r.IsError, text(r))
	}
	r = h.call("search", map[string]any{"project": "Third", "query": "x"})
	if !r.IsError || !strings.Contains(text(r), `unknown project "third"`) {
		t.Errorf("unknown project: %v %s", r.IsError, text(r))
	}
	if n := len(h.lk.Requests()); n != 0 {
		t.Fatalf("%d requests sent for unresolved projects", n)
	}
	h.lk.Enqueue(testutil.JSON(http.StatusOK, rtTagsJSON), testutil.JSON(http.StatusOK, rtTagsJSON), testutil.JSON(http.StatusOK, rtTagsJSON))
	for _, p := range []string{"SECOND", "first", "second"} {
		if r := h.call("list_tags", map[string]any{"project": p}); r.IsError {
			t.Fatal(text(r))
		}
	}
	reqs := h.lk.Requests()
	for i, want := range []string{rtKey2, rtKey1, rtKey2} {
		if got := reqs[i].Header.Get("Authorization"); got != "Bearer "+want {
			t.Errorf("request %d Authorization = %q", i, got)
		}
	}
}
