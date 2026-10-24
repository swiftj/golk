//go:build integration

package mcpserver_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestIntegration_MCPMapsAndTemplates drives the map and template tools
// against the sandbox through MCP, with the real spoiler guard.
func TestIntegration_MCPMapsAndTemplates(t *testing.T) {
	cs, c, ctx := session(t), sandbox.Client(t), sandbox.Ctx(t)

	cr := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Land of Ooo")})
	rid, _ := cr.out["resource_id"].(string)
	if rid == "" {
		t.Fatalf("create_resource output %v", cr.out)
	}
	sandbox.Cleanup(t, rid)
	r, err := c.Resources.Get(ctx, rid)
	if err != nil || len(r.Documents) == 0 {
		t.Fatalf("resource %+v, %v", r, err)
	}
	doc := r.Documents[0].ID

	// A 512x256 image: bounds [[-128,0],[0,256]] (D-014).
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 512, 256))
	img.Set(10, 10, color.RGBA{R: 0x33, G: 0x99, B: 0x33, A: 0xff})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(t.TempDir(), "ooo.png")
	if err := os.WriteFile(pngPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	up := mustOK(t, cs, "upload_image", map[string]any{"path": pngPath, "name": sandbox.Name("Ooo map")})
	aid, _ := up.out["asset_id"].(string)
	if aid == "" {
		t.Fatalf("upload_image %v", up.out)
	}
	sandbox.CleanupAsset(t, aid)

	// X-MAP-02: set the image (a dry run first), then add a pin by pixel.
	mustOK(t, cs, "set_map_image", map[string]any{"resource_id": rid, "document_id": doc, "asset_id": aid, "dry_run": true})
	si := mustOK(t, cs, "set_map_image", map[string]any{"resource_id": rid, "document_id": doc, "asset_id": aid})
	if im, _ := si.out["image"].(map[string]any); im["width"] != float64(512) || im["height"] != float64(256) {
		t.Fatalf("set_map_image %v: %s", si.out, si.text)
	}
	pin := mustOK(t, cs, "set_map_feature", map[string]any{"resource_id": rid, "document_id": doc, "type": "pin",
		"name": "Candy Castle", "pixel": []float64{256, 128}, "style": map[string]any{"icon_shape": "shield"}})
	fid, _ := pin.out["feature_id"].(string)
	if fid == "" {
		t.Fatalf("set_map_feature %v", pin.out)
	}
	mustOK(t, cs, "set_map_feature", map[string]any{"resource_id": rid, "feature_id": fid, "style": map[string]any{"icon_color": "#336633"}})

	// X-MAP-01.
	rm := mustOK(t, cs, "read_map", map[string]any{"resource_id": rid})
	if !strings.Contains(rm.text, `- pin "Candy Castle" (`+fid+`) pos [-64, 128]`) || rm.out["features"] != float64(1) ||
		!strings.Contains(rm.text, "512x256, bounds [[-128, 0], [0, 256]]") {
		t.Fatalf("read_map %v:\n%s", rm.out, rm.text)
	}

	// X-GUARD-08: a public pin named with a term is refused; X-GUARD-11:
	// hiding it doesn't help until hidden pins are confirmed invisible.
	mustRefuse(t, cs, "set_map_feature", map[string]any{"resource_id": rid, "document_id": doc, "type": "pin", "name": "Zorblax Spire", "pos": []float64{-10, 10}}, "spoiler guard")
	mustRefuse(t, cs, "set_map_feature", map[string]any{"resource_id": rid, "document_id": doc, "type": "pin", "name": "Zorblax Spire", "pos": []float64{-10, 10}, "is_hidden": true}, "spoiler guard")
	if fs, err := c.Features.List(ctx, lk.DocRef{ResourceID: rid, DocumentID: doc}, lk.FeatureListParams{}); err != nil || len(fs.Items) != 1 {
		t.Fatalf("features after the refusals: %+v, %v", fs, err)
	}

	// A folder: its delete dry run warns; then delete it for real.
	fo := mustOK(t, cs, "set_map_feature", map[string]any{"resource_id": rid, "document_id": doc, "type": "folder", "name": "Homes"})
	foid, _ := fo.out["feature_id"].(string)
	dd := mustOK(t, cs, "delete_map_feature", map[string]any{"resource_id": rid, "feature_id": foid, "dry_run": true})
	if !strings.Contains(dd.text, "parentId") {
		t.Errorf("folder dry run: %s", dd.text)
	}
	mustOK(t, cs, "delete_map_feature", map[string]any{"resource_id": rid, "document_id": doc, "feature_id": foid})

	// A hidden child may hold a term.
	kid := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Zorblax Crypt"), "parent_id": rid, "is_hidden": true,
		"page_markdown": "Zorblax waits below.\n"})
	kidID, _ := kid.out["resource_id"].(string)
	if kidID == "" {
		t.Fatalf("create_resource %v", kid.out)
	}

	// X-TPL-01, X-GUARD-10: save, list, create from it.
	mustRefuse(t, cs, "save_as_template", map[string]any{"resource_id": rid, "name": sandbox.Name("Zorblax Template")}, "spoiler guard")
	st := mustOK(t, cs, "save_as_template", map[string]any{"resource_id": rid, "name": sandbox.Name("Ooo Template"), "include_children": true})
	tid, _ := st.out["resource_id"].(string)
	if tid == "" {
		t.Fatalf("save_as_template %v", st.out)
	}
	sandbox.Cleanup(t, tid)
	if st.out["copied"] != float64(2) || st.out["checked"] != float64(1) {
		t.Errorf("save_as_template %v", st.out)
	}

	found, cursor := false, ""
	for i := 0; i < 50 && !found; i++ {
		args := map[string]any{"limit": 100}
		if cursor != "" {
			args["cursor"] = cursor
		}
		lt := mustOK(t, cs, "list_templates", args)
		found = strings.Contains(lt.text, "("+tid+")")
		cursor, _ = lt.out["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if !found {
		t.Fatalf("list_templates lacks %s", tid)
	}

	cf := mustOK(t, cs, "create_from_template", map[string]any{"template_id": tid, "name": sandbox.Name("Ooo Copy"), "include_children": true})
	nid, _ := cf.out["resource_id"].(string)
	if nid == "" {
		t.Fatalf("create_from_template %v", cf.out)
	}
	sandbox.Cleanup(t, nid)
	if cf.out["copied"] != float64(2) || cf.out["checked"] != float64(1) {
		t.Errorf("create_from_template %v", cf.out)
	}
	// D-014 A-35: the copy keeps the child's hidden flag.
	var kids []lk.ResourceListItem
	for k, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: nid}) {
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, k)
	}
	if len(kids) != 1 || !kids[0].IsHidden {
		t.Errorf("copy's children %+v", kids)
	}
	mustRefuse(t, cs, "create_from_template", map[string]any{"template_id": tid, "name": sandbox.Name("Zorblax Copy")}, "spoiler guard")
}
