//go:build integration

package mcpserver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/mcpserver"
	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestIntegration_MCPFullCoverage drives the Phase 7 tools against the
// sandbox through MCP (D-021): sidebar order, documents of every type,
// block and row tools, every property kind, document delete and restore,
// asset folders, and members and invites.
func TestIntegration_MCPFullCoverage(t *testing.T) {
	cs, c, ctx := session(t), sandbox.Client(t), sandbox.Ctx(t)

	// X-RES-04, X-RES-01, X-RES-05: a batch of children, then reorder one.
	parent := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Atlas")})
	pid, _ := parent.out["resource_id"].(string)
	sandbox.Cleanup(t, pid)
	batch := mustOK(t, cs, "create_resources", map[string]any{"resources": []any{
		map[string]any{"name": "Candy Kingdom", "parent_id": pid},
		map[string]any{"name": "Ice Kingdom", "parent_id": pid},
		map[string]any{"name": "Fire Kingdom", "parent_id": pid},
	}})
	created, _ := batch.out["resources"].([]any)
	if len(created) != 3 {
		t.Fatalf("create_resources %v", batch.out)
	}
	id := func(i int) string { m, _ := created[i].(map[string]any); s, _ := m["resource_id"].(string); return s }
	mustOK(t, cs, "update_resource", map[string]any{"id": id(2), "before": id(0)})
	lr := mustOK(t, cs, "list_resources", map[string]any{"parent_id": pid})
	fire, candy := strings.Index(lr.text, "Fire Kingdom"), strings.Index(lr.text, "Candy Kingdom")
	if fire < 0 || candy < 0 || fire > candy || !strings.Contains(lr.text, " rank ") {
		t.Fatalf("list_resources after the move:\n%s", lr.text)
	}

	// X-RES-03: documents of every type at create.
	multi := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Ooo"), "parent_id": pid, "documents": []any{
		map[string]any{"name": "Lore", "markdown": "Candy people.\n"},
		map[string]any{"name": "Atlas", "type": "map"},
		map[string]any{"name": "History", "type": "time", "calendar_id": "gregorian"},
		map[string]any{"name": "Rivalries", "type": "board"},
	}})
	rid, _ := multi.out["resource_id"].(string)
	r, err := c.Resources.Get(ctx, rid)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, d := range r.Documents {
		types = append(types, string(d.Type))
	}
	if strings.Join(types, ",") != "page,map,time,board" {
		t.Fatalf("document types %v", types)
	}
	page := r.Documents[0].ID

	// X-DOC-04, X-BLOCK-*: assign ids, then insert, update, move and delete
	// blocks with the page version.
	mustOK(t, cs, "assign_block_ids", map[string]any{"resource_id": rid, "document_id": page})
	rp := mustOK(t, cs, "read_page", map[string]any{"resource_id": rid, "document_id": page})
	v, _ := rp.out["version"].(string)
	ins := mustOK(t, cs, "insert_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "markdown": "## Rulers"})
	hid, _ := ins.out["block_id"].(string)
	v, _ = ins.out["version"].(string)
	up := mustOK(t, cs, "update_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "block_id": hid, "patch": map[string]any{"level": 3}})
	v, _ = up.out["version"].(string)
	db, err := c.Documents.GetBlocks(ctx, lk.DocRef{ResourceID: rid, DocumentID: page})
	if err != nil {
		t.Fatal(err)
	}
	mv := mustOK(t, cs, "move_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "block_id": hid, "before": db.Blocks[0].NodeID()})
	v, _ = mv.out["version"].(string)
	rb := mustOK(t, cs, "read_block", map[string]any{"resource_id": rid, "document_id": page, "block_id": hid})
	if !strings.Contains(rb.text, "### Rulers") {
		t.Fatalf("read_block:\n%s", rb.text)
	}
	mustOK(t, cs, "delete_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "block_id": hid})

	// X-PROP-*, X-ROW-*: a meter property, a key-value property and its rows,
	// and an infobox on the page with a property and a row.
	mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "property_markdown": ":::meter mode=bar title=Vitals\n- HP :: 3/5 icon=heart\n:::"})
	kv := mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "title": "Facts", "rows": []any{map[string]any{"key": "Ruler", "value": "Princess Bubblegum"}}})
	kvID, _ := kv.out["property_id"].(string)
	mustOK(t, cs, "set_row", map[string]any{"resource_id": rid, "property_id": kvID, "key": "Capital", "value": "Candy Castle"})
	mustOK(t, cs, "set_row", map[string]any{"resource_id": rid, "property_id": kvID, "key": "Ruler", "value": "**PB**"})
	mustOK(t, cs, "delete_row", map[string]any{"resource_id": rid, "property_id": kvID, "key": "Capital"})
	rows, err := c.Properties.ListRows(ctx, rid, kvID)
	if err != nil || len(rows.Items) != 1 || rows.Items[0].Key != "Ruler" {
		t.Fatalf("rows %+v, %v", rows, err)
	}
	rp = mustOK(t, cs, "read_page", map[string]any{"resource_id": rid, "document_id": page})
	v, _ = rp.out["version"].(string)
	ib := mustOK(t, cs, "insert_block", map[string]any{"resource_id": rid, "document_id": page, "version": v,
		"markdown": ":::infobox\n:::keyvalue title=Stats\n- Age :: 827\n:::\n:::"})
	ibID, _ := ib.out["block_id"].(string)
	ip := mustOK(t, cs, "set_property", map[string]any{"resource_id": rid, "document_id": page, "block_id": ibID, "title": "Origin", "markdown": "Grown from gum."})
	if !strings.Contains(ip.text, "infobox "+ibID) {
		t.Fatalf("infobox set_property: %s", ip.text)
	}
	props, err := c.Blocks.ListProperties(ctx, lk.DocRef{ResourceID: rid, DocumentID: page}, ibID)
	if err != nil || len(props.Items) != 2 {
		t.Fatalf("infobox properties %+v, %v", props, err)
	}
	statsID := props.Items[0].PropertyEnvelope().ID
	mustOK(t, cs, "set_row", map[string]any{"resource_id": rid, "document_id": page, "block_id": ibID, "property_id": statsID, "key": "Species", "value": "Candy person"})

	// X-GUARD-15: a term in the open is refused; in a secret it is allowed.
	rp = mustOK(t, cs, "read_page", map[string]any{"resource_id": rid, "document_id": page})
	v, _ = rp.out["version"].(string)
	mustRefuse(t, cs, "insert_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "markdown": "Zorblax returns."}, "spoiler guard")
	mustOK(t, cs, "insert_block", map[string]any{"resource_id": rid, "document_id": page, "version": v, "markdown": ":::secret\nZorblax returns.\n:::"})

	// X-DOC-01..03: a full-width document, reordered, deleted and restored.
	nd := mustOK(t, cs, "create_document", map[string]any{"resource_id": rid, "name": "Notes", "is_full_width": true})
	ndID, _ := nd.out["document_id"].(string)
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": ndID, "rank": "0"})
	mustOK(t, cs, "delete_document", map[string]any{"resource_id": rid, "document_id": ndID})
	rs := mustOK(t, cs, "restore_document", map[string]any{"resource_id": rid, "document_id": ndID})
	if !strings.Contains(rs.text, "visible to players (checked") {
		t.Fatalf("restore_document: %s", rs.text)
	}

	// X-ASSET-03: an asset folder, renamed and described.
	f := mustOK(t, cs, "create_asset_folder", map[string]any{"name": sandbox.Name("Art")})
	fid, _ := f.out["id"].(string)
	sandbox.CleanupAsset(t, fid)
	mustOK(t, cs, "update_asset", map[string]any{"asset_id": fid, "name": sandbox.Name("Old art"), "description": "Retired sketches"})
	ga := mustOK(t, cs, "get_asset", map[string]any{"asset_id": fid})
	if !strings.Contains(ga.text, "Retired sketches") {
		t.Fatalf("get_asset: %s", ga.text)
	}

	// X-MEM-*: members, then an invite to a reserved, undeliverable address
	// (example.com) that is revoked at once.
	lm := mustOK(t, cs, "list_members", map[string]any{})
	if !strings.Contains(lm.text, "member(s)") {
		t.Fatalf("list_members: %s", lm.text)
	}
	email := sandbox.RunPrefix + "@example.com"
	mustOK(t, cs, "invite_member", map[string]any{"email": email})
	t.Cleanup(func() {
		for inv, err := range c.Invites.All(sandbox.Ctx(t)) {
			if err == nil && inv.Email == email {
				if err := c.Invites.Revoke(sandbox.Ctx(t), inv.ID); err != nil && !errors.Is(err, lk.ErrNotFound) {
					t.Errorf("cleanup: revoking invite %s: %v", inv.ID, err)
				}
			}
		}
	})
	li := mustOK(t, cs, "list_invites", map[string]any{})
	var inviteID string
	for _, line := range strings.Split(li.text, "\n") {
		if strings.Contains(line, email) {
			inviteID = line[strings.Index(line, "(")+1 : strings.Index(line, ")")]
		}
	}
	if inviteID == "" {
		t.Fatalf("list_invites lacks %s:\n%s", email, li.text)
	}
	mustOK(t, cs, "revoke_invite", map[string]any{"invite_id": inviteID})
}

// TestIntegration_Glyphs_X_ICON_03 checks that LegendKeeper accepts every glyph
// lk-mcp suggests or renames to (X-ICON-03), so the list can't drift.
func TestIntegration_Glyphs_X_ICON_03(t *testing.T) {
	cs := session(t)
	r := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Icons")})
	id, _ := r.out["resource_id"].(string)
	sandbox.Cleanup(t, id)
	for _, g := range mcpserver.GlyphsToCheck() {
		if res := call(t, cs, "update_resource", map[string]any{"id": id, "icon_glyph": g}); res.err {
			t.Errorf("glyph %q rejected: %s", g, res.text)
		}
	}
}
