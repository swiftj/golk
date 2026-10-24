package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver/guard"
	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk"
)

// harness is an MCP client session with an lk-mcp server whose single
// project ("default", id p1) talks to a fake LegendKeeper.
type harness struct {
	t     *testing.T
	lk    *testutil.FakeServer
	guard *fakeGuard
	cs    *mcp.ClientSession
}

// fakeGuard flags every string containing one of its terms, and records
// what it was asked to check.
type fakeGuard struct {
	mu      sync.Mutex
	terms   []string
	err     error
	checked []string
}

func (g *fakeGuard) Check(v any) ([]guard.Violation, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checked = append(g.checked, string(data))
	if g.err != nil {
		return nil, g.err
	}
	var out []guard.Violation
	for _, term := range g.terms {
		if strings.Contains(string(data), term) {
			out = append(out, guard.Violation{Term: term, Path: "$"})
		}
	}
	return out, nil
}

type harnessOpt func(*Config, *Deps)

func readOnlyMode() harnessOpt { return func(c *Config, _ *Deps) { c.ReadOnly = true } }
func noGuard() harnessOpt      { return func(_ *Config, d *Deps) { d.Guard = nil } }

func newHarness(t *testing.T, opts ...harnessOpt) *harness {
	t.Helper()
	h := &harness{t: t, lk: testutil.NewFakeServer(t), guard: &fakeGuard{}}
	cfg := Config{}.WithKey(DefaultProject, "test-key")
	d := Deps{Guard: h.guard, Version: "test", ClientOptions: []lk.Option{
		lk.WithBaseURL(h.lk.URL), lk.WithProjectID("p1"), lk.WithRateLimiter(nil), lk.WithMaxRetries(0),
	}}
	for _, o := range opts {
		o(&cfg, &d)
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
	return h
}

// call calls a tool and returns its result. A protocol error fails the
// test; a tool error is returned with IsError set.
func (h *harness) call(name string, args any) *mcp.CallToolResult {
	h.t.Helper()
	r, err := h.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("%s: %v", name, err)
	}
	return r
}

// text is a result's text content.
func text(r *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// structured decodes a result's structured content into v.
func structured(t *testing.T, r *mcp.CallToolResult, v any) {
	t.Helper()
	data, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
}

// writes are the recorded requests that change something.
func (h *harness) writes() []testutil.Recorded {
	var out []testutil.Recorded
	for _, r := range h.lk.Requests() {
		if r.Method != "GET" {
			out = append(out, r)
		}
	}
	return out
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func callParams(name string, args any) *mcp.CallToolParams {
	return &mcp.CallToolParams{Name: name, Arguments: args}
}
