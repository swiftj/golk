//go:build integration

package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver"
	"github.com/swiftj/golk/internal/mcpserver/guard"
	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

func TestMain(m *testing.M) { os.Exit(sandbox.Main(m)) }

// session is an MCP client connected to an in-process lk-mcp whose default
// project is the sandbox, with a guard file of made-up terms.
func session(t *testing.T) *mcp.ClientSession {
	t.Helper()
	terms := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(terms, []byte("# made-up test terms\nZorblax\nthe Crimson Accord\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := mcpserver.Config{GuardFile: terms}.WithKey(mcpserver.DefaultProject, sandbox.APIKey(t))
	srv := mcpserver.New(cfg, mcpserver.Deps{Guard: guard.Open(terms), Version: "it", ClientOptions: sandbox.ClientOptions(t)})
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "it", Version: "it"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type result struct {
	text string
	out  map[string]any
	err  bool
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) result {
	t.Helper()
	r, err := cs.CallTool(sandbox.Ctx(t), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var res result
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			res.text += tc.Text
		}
	}
	res.err = r.IsError
	if r.StructuredContent != nil {
		data, _ := json.Marshal(r.StructuredContent)
		_ = json.Unmarshal(data, &res.out)
	}
	return res
}

func mustOK(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) result {
	t.Helper()
	r := call(t, cs, name, args)
	if r.err {
		t.Fatalf("%s: %s", name, r.text)
	}
	return r
}

func mustRefuse(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	r := call(t, cs, name, args)
	if !r.err || !strings.Contains(r.text, want) {
		t.Fatalf("%s: want a refusal containing %q, got %v: %s", name, want, r.err, r.text)
	}
}

func blockIDs(l blocks.List) []string {
	var out []string
	blocks.Walk(l, func(_ string, n blocks.Node) bool {
		out = append(out, n.NodeID())
		return true
	})
	return out
}

// TestIntegration_MCPWiki drives every write tool against the sandbox
// through MCP, with the real spoiler guard.
func TestIntegration_MCPWiki(t *testing.T) {
	cs, c, ctx := session(t), sandbox.Client(t), sandbox.Ctx(t)
	page := "Candy Kingdom lore.\n\nThe castle is pink.\n\n:::secret\nZorblax rules from the shadows.\n:::\n"

	// X-GUARD-07: a protected term in a public name is refused and nothing
	// is created.
	mustRefuse(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Zorblax Keep")}, "spoiler guard")

	// X-GUARD-03: the same term inside a secret is fine.
	cr := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Candy Kingdom"), "page_markdown": page, "tags": []string{"Kingdom"}})
	rid, _ := cr.out["resource_id"].(string)
	if rid == "" {
		t.Fatalf("create_resource output %v", cr.out)
	}
	sandbox.Cleanup(t, rid)

	rp := mustOK(t, cs, "read_page", map[string]any{"resource_id": rid})
	version, _ := rp.out["version"].(string)
	docID, _ := rp.out["document_id"].(string)
	if !strings.Contains(rp.text, ":::secret\nZorblax rules from the shadows.\n:::") {
		t.Fatalf("read_page text:\n%s", rp.text)
	}
	md := rp.text[strings.Index(rp.text, "\n\n")+2:]
	doc := lk.DocRef{ResourceID: rid, DocumentID: docID}
	before, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}

	// X-TOOL-04: a dry run writes nothing.
	edited := strings.Replace(md, "The castle is pink.", "The castle is very pink.", 1)
	dr := mustOK(t, cs, "edit_page", map[string]any{"resource_id": rid, "base_version": version, "markdown": edited, "dry_run": true})
	if dr.out["changed"] != float64(1) {
		t.Errorf("dry run report %v", dr.out)
	}
	if now, _ := c.Documents.GetBlocks(ctx, doc); now.Version != before.Version {
		t.Fatalf("dry run changed the page: %s → %s", before.Version, now.Version)
	}

	// X-TOOL-05: the edit keeps every id and changes one paragraph.
	er := mustOK(t, cs, "edit_page", map[string]any{"resource_id": rid, "base_version": version, "markdown": edited})
	after, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(blockIDs(before.Blocks), blockIDs(after.Blocks)) || er.out["version"] != after.Version {
		t.Errorf("ids before %v after %v; report %v", blockIDs(before.Blocks), blockIDs(after.Blocks), er.out)
	}

	// A stale base version is refused.
	mustRefuse(t, cs, "edit_page", map[string]any{"resource_id": rid, "base_version": version, "markdown": edited}, "changed")

	// X-GUARD-03: a protected term in public text is refused, and accepted
	// inside a secret.
	v2 := after.Version
	mustRefuse(t, cs, "edit_page", map[string]any{"resource_id": rid, "base_version": v2,
		"markdown": edited + "\nThe Crimson Accord meets at dawn.\n"}, "the Crimson Accord")
	mustOK(t, cs, "edit_page", map[string]any{"resource_id": rid, "base_version": v2,
		"markdown": edited + "\n:::secret\nThe Crimson Accord meets at dawn.\n:::\n"})

	// X-TOOL-06: sidebar property upsert and delete.
	sp := mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Stats",
		"rows": []map[string]string{{"key": "Ruler", "value": "**Princess** Bubblegum"}}})
	if sp.out["created"] != true {
		t.Errorf("set_property %v", sp.out)
	}
	mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Stats",
		"rows": []map[string]string{{"key": "Ruler", "value": "Princess Bubblegum"}, {"key": "Capital", "value": "Candy Castle"}}})
	mustRefuse(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Notes", "markdown": "Zorblax is near."}, "spoiler guard")
	mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Notes", "markdown": "Zorblax is near.", "is_hidden": true})
	// X-GUARD-05: showing that property is refused.
	mustRefuse(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Notes", "is_hidden": false}, "spoiler guard")
	mustOK(t, cs, "delete_property", map[string]any{"resource_id": rid, "title": "Stats"})

	// X-GUARD-04/05: a hidden document may hold the term; showing it may not.
	cd := mustOK(t, cs, "create_document", map[string]any{"resource_id": rid, "name": "DM notes", "is_hidden": true,
		"markdown": "Zorblax lives under the castle.\n"})
	dm, _ := cd.out["document_id"].(string)
	mustRefuse(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": dm, "is_hidden": false}, "spoiler guard")
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": dm, "name": "Notes for the DM"})

	// X-TOOL-07: an image upload.
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 0xec, G: 0x48, B: 0x99, A: 0xff})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(t.TempDir(), "flag.png")
	if err := os.WriteFile(pngPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	up := mustOK(t, cs, "upload_image", map[string]any{"path": pngPath, "name": sandbox.Name("Candy flag")})
	aid, _ := up.out["asset_id"].(string)
	if aid == "" || up.out["url"] == "" {
		t.Fatalf("upload_image %v", up.out)
	}
	sandbox.CleanupAsset(t, aid)

	// X-TOOL-08: delete (dry run, then real) and restore.
	mustOK(t, cs, "delete_resource", map[string]any{"id": rid, "dry_run": true})
	if _, err := c.Resources.Get(ctx, rid); err != nil {
		t.Fatalf("dry run deleted it: %v", err)
	}
	mustOK(t, cs, "delete_resource", map[string]any{"id": rid})
	rr := mustOK(t, cs, "restore_resource", map[string]any{"id": rid})
	if rr.out["hidden"] != false {
		t.Errorf("a clean restore left it hidden: %v %s", rr.out, rr.text)
	}
	if r, err := c.Resources.Get(ctx, rid); err != nil || r.IsHidden {
		t.Errorf("after restore: %+v, %v", r, err)
	}
}
