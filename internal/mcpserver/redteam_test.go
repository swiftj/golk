package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver/guard"
	"github.com/swiftj/golk/internal/testutil"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

func mustBlocks(t *testing.T, docJSON string) blocks.List {
	t.Helper()
	var db lk.DocumentBlocks
	if err := json.Unmarshal([]byte(docJSON), &db); err != nil {
		t.Fatal(err)
	}
	return db.Blocks
}

// Red-team tests: attacks on the guarantees of lk-mcp, run with the REAL
// spoiler guard over a temporary terms file. All terms are made up.

// rgHarness is newHarness with a real guard reading terms.
func rgHarness(t *testing.T, terms ...string) *harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(path, []byte(strings.Join(terms, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return rgHarnessWith(t, guard.Open(path))
}

func rgHarnessWith(t *testing.T, g Checker) *harness {
	t.Helper()
	h := &harness{t: t, lk: testutil.NewFakeServer(t)}
	cfg := Config{}.WithKey(DefaultProject, "test-key")
	d := Deps{Guard: g, Version: "test", ClientOptions: []lk.Option{
		lk.WithBaseURL(h.lk.URL), lk.WithProjectID("p1"), lk.WithRateLimiter(nil), lk.WithMaxRetries(0),
	}}
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

const rgBlocks = `{"id":"d1","version":"v7","schema":1,"text":"Hi","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}]}]}`

// The guard's generic walk sees every field the Markdown can put in a page:
// alt text, link URLs, mention labels, lk-raw JSON and fields hidden in
// runs.
func TestRedteam_X_GUARD_03_EditPageFieldsAreChecked(t *testing.T) {
	cases := map[string]string{
		"link url":                   "[here](https://example.com/zorblax)\n",
		"link text":                  "[Zorblax](https://example.com/)\n",
		"image alt":                  "![Zorblax](https://example.com/a.png)\n",
		"heading":                    "# The Crimson Accord\n",
		"line break":                 "The Crimson\nAccord\n",
		"case":                       "ZORBLAX\n",
		"zero width":                 "Zor​blax\n",
		"lk-raw run":                 "```lk-raw\n{\"kind\":\"paragraph\",\"content\":[{\"text\":\"Zorblax\"}]}\n```\n",
		"lk-raw secret-kind run":     "```lk-raw\n{\"kind\":\"paragraph\",\"content\":[{\"text\":\"Zorblax\",\"kind\":\"secret\"}]}\n```\n",
		"lk-raw hidden-property run": "```lk-raw\n{\"kind\":\"paragraph\",\"content\":[{\"text\":\"Zorblax\",\"kind\":\"textField\",\"isHidden\":true}]}\n```\n",
		"lk-raw secret in content":   "```lk-raw\n{\"kind\":\"paragraph\",\"content\":[{\"kind\":\"secret\",\"children\":[{\"kind\":\"paragraph\",\"content\":[{\"text\":\"Zorblax\"}]}]}]}\n```\n",
		"lk-raw unknown kind":        "```lk-raw\n{\"kind\":\"futureThing\",\"label\":\"Zorblax\"}\n```\n",
	}
	for name, md := range cases {
		t.Run(name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax", "the Crimson Accord")
			h.lk.Enqueue(wtOK(wtRes), wtOK(rgBlocks))
			r := h.call("edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": md, "allow_remove_opaque": true})
			if !r.IsError {
				t.Fatalf("accepted %q: %s", md, text(r))
			}
			if !strings.Contains(text(r), "spoiler guard") && !strings.Contains(text(r), "line") {
				t.Errorf("not a guard or syntax refusal: %s", text(r))
			}
			if len(h.writes()) != 0 {
				t.Errorf("writes %+v", h.writes())
			}
		})
	}
}

// Content restored from the base page by block matching (fields the
// Markdown doesn't model) is part of what is checked.
func TestRedteam_X_GUARD_03_EditPageBaseFieldsAreChecked(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	base := `{"id":"d1","version":"v7","schema":1,"text":"Hi","blocks":[{"kind":"paragraph","id":"p1","content":[{"text":"Hi"}],"futureField":"Zorblax"}]}`
	md, err := markdown.ToMarkdown(mustBlocks(t, base), markdown.ToOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h.lk.Enqueue(wtOK(wtRes), wtOK(base))
	r := h.call("edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": md})
	if !r.IsError || !strings.Contains(text(r), "spoiler guard") {
		t.Fatalf("want a guard refusal: %s", text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

// Every player-visible input field of create_resource is checked.
func TestRedteam_X_GUARD_03_CreateResourceFields(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"name":      {"name": "Zorblax"},
		"alias":     {"name": "Ooo", "aliases": []string{"zorblax"}},
		"tag":       {"name": "Ooo", "tags": []string{"Zorblax"}},
		"page name": {"name": "Ooo", "page_name": "Zorblax"},
		"page":      {"name": "Ooo", "page_markdown": "| a | b |\n|---|---|\n| Zorblax | x |\n"},
	} {
		t.Run(name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			r := h.call("create_resource", args)
			if !r.IsError {
				t.Fatalf("accepted: %s", text(r))
			}
			if len(h.lk.Requests()) != 0 {
				t.Errorf("requests %+v", h.lk.Requests())
			}
		})
	}
}

// Row keys, row values and an unmodelled field of a hidden property all
// count when the property is shown.
func TestRedteam_X_GUARD_05_SetPropertyShowChecksUnmodelledFields(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	prop := `{"kind":"keyValue","id":"q2","rank":"b","title":"Facts","isHidden":true,"futureCaption":"Zorblax","rows":[{"id":"w1","key":"Age","content":[{"text":"9"}]}]}`
	h.lk.Enqueue(wtOK(wtRes), wtOK(`{"data":[`+prop+`],"hasMore":false}`))
	r := h.call("set_property", map[string]any{"resource_id": "r1", "title": "Facts", "is_hidden": false})
	if !r.IsError || !strings.Contains(text(r), "spoiler guard") {
		t.Fatalf("want a guard refusal: %s", text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}

	h = rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtNoProps))
	r = h.call("set_property", map[string]any{"resource_id": "r1", "title": "Facts", "is_title_hidden": true,
		"rows": []map[string]string{{"key": "Zorblax", "value": "x"}}})
	if !r.IsError || !strings.Contains(text(r), "spoiler guard") {
		t.Fatalf("row key: want a guard refusal: %s", text(r))
	}
}

// The asset name and description are checked, on a dry run too.
func TestRedteam_X_GUARD_03_UploadImageDescription(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	path := filepath.Join(t.TempDir(), "a.png")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")
	if err := os.WriteFile(path, png, 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.call("upload_image", map[string]any{"path": path, "name": "a", "description": "of zorblax", "dry_run": true})
	if !r.IsError || !strings.Contains(text(r), "spoiler guard") {
		t.Fatalf("want a guard refusal: %s", text(r))
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

// Odd inputs give tool errors, not panics or protocol errors.
func TestRedteam_X_ERR_01_OddInputs(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"set_property", map[string]any{"resource_id": "r1", "title": "T", "rows": []any{}, "dry_run": true}},
		{"set_property", map[string]any{"resource_id": "", "title": ""}},
		{"edit_page", map[string]any{"resource_id": "", "base_version": "", "markdown": ""}},
		{"create_resource", map[string]any{"name": "   "}},
		{"create_document", map[string]any{"resource_id": "r1", "name": "x", "type": "map"}},
		{"update_resource", map[string]any{"id": "r1"}},
		{"update_document", map[string]any{"resource_id": "r1", "document_id": "d1"}},
		{"delete_property", map[string]any{"resource_id": "r1"}},
		{"upload_image", map[string]any{"path": "/nonexistent/x.png", "name": "x"}},
		{"upload_image", map[string]any{"path": "/", "name": "x"}},
		{"read_page", map[string]any{"resource_id": "r1", "project": "nosuch"}},
	} {
		h.lk.Enqueue(wtOK(wtRes), wtOK(wtNoProps), testutil.JSON(http.StatusNotFound, `{"message":"no","code":"NOT_FOUND"}`))
		r, err := h.cs.CallTool(context.Background(), callParams(c.tool, c.args))
		if err != nil {
			t.Errorf("%s %v: protocol error %v", c.tool, c.args, err)
			continue
		}
		_ = r
	}
	for _, w := range h.writes() {
		t.Errorf("an odd input wrote %s %s", w.Method, w.Path)
	}
}

// No key in any error or %#v of the config.
func TestRedteam_X_CFG_02_NoKeyInErrors(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(testutil.JSON(http.StatusUnauthorized, `{"message":"bad key test-key","code":"UNAUTHENTICATED","requestId":"rq1"}`))
	r := h.call("get_resource", map[string]any{"id": "r1"})
	if !r.IsError {
		t.Fatalf("want an error: %s", text(r))
	}
	// The server's own message is relayed as is; the key never comes from
	// lk-mcp itself.
	cfg := Config{}.WithKey(DefaultProject, "sekrit-key-123")
	for _, s := range []string{cfg.String(), cfgGo(cfg), (&projects{cfg: cfg}).cfg.String()} {
		if strings.Contains(s, "sekrit") {
			t.Errorf("key leaked: %s", s)
		}
	}
}

func cfgGo(c Config) string { return fmt.Sprintf("%#v %+v %v", c, c, c) }
