//go:build integration

package mcpserver_test

import (
	"bytes"
	"errors"
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

// TestIntegration_MCPDeleteAsset uploads an image and deletes it again
// through MCP (X-ASSET-01): a dry run changes nothing, the delete removes it.
func TestIntegration_MCPDeleteAsset(t *testing.T) {
	cs, c, ctx := session(t), sandbox.Client(t), sandbox.Ctx(t)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: 0x99, A: 0xff})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	up := mustOK(t, cs, "upload_image", map[string]any{"path": path, "name": sandbox.Name("Old image")})
	id, _ := up.out["asset_id"].(string)
	if id == "" {
		t.Fatalf("upload_image %v", up.out)
	}
	sandbox.CleanupAsset(t, id)

	dry := mustOK(t, cs, "delete_asset", map[string]any{"asset_id": id, "dry_run": true})
	if !strings.Contains(dry.text, "Dry run: would delete image") {
		t.Errorf("dry run: %s", dry.text)
	}
	if _, err := c.Assets.Get(ctx, id); err != nil {
		t.Fatalf("the dry run deleted it: %v", err)
	}
	mustOK(t, cs, "delete_asset", map[string]any{"asset_id": id})
	if _, err := c.Assets.Get(ctx, id); !errors.Is(err, lk.ErrNotFound) {
		t.Fatalf("after delete_asset: err = %v, want ErrNotFound", err)
	}
}
