//go:build integration

package lk_test

// Asset-deletion probe (SDK_CONTRACT.md A-41). Run with
//
//	make test-integration ARGS='-run TestProbe6 -v ./lk/'
//
// and copy the observations into DECISIONS.md D-020.

import (
	"net/http"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// TestProbe6_DeleteUsedAsset covers A-41: what a page that shows an image
// is left with when its asset is deleted.
func TestProbe6_DeleteUsedAsset(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	a, err := c.Assets.UploadImage(ctx, lk.UploadImageParams{Data: tinyPNG(t), Name: sandbox.Name("Probe image")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, a.ID)
	r := create(t, c, lk.ResourceCreate{Name: sandbox.Name("Probe page")})
	doc := lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}
	if _, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.Image(a.URL, "probe")}); err != nil {
		t.Fatal(err)
	}
	status := func() int {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("GET url: %v", err)
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	t.Logf("A-41 before delete: GET url -> %d", status())
	if err := c.Assets.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	t.Logf("A-41 after delete: GET url -> %d", status())
	_, err = c.Assets.Get(ctx, a.ID)
	t.Logf("A-41 get deleted asset: %v", err)
	db, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("A-41 page after delete: %d block(s), first %T", len(db.Blocks), db.Blocks[0])
	if img, ok := db.Blocks[0].(*blocks.ImageBlock); ok {
		t.Logf("A-41 image url kept: %v", img.URL == a.URL)
	}
	_, err = c.Documents.ReplaceBlocks(ctx, doc, db.Blocks, lk.IfMatch(db.Version))
	t.Logf("A-41 rewrite page unchanged after delete: err=%v", err)
	r2 := create(t, c, lk.ResourceCreate{Name: sandbox.Name("Probe page 2")})
	_, err = c.Documents.ReplaceBlocks(ctx, lk.DocRef{ResourceID: r2.ID, DocumentID: r2.Documents[0].ID}, blocks.List{blocks.Image(a.URL, "probe")})
	t.Logf("A-41 write deleted asset's url to another page: err=%v", err)
	err = c.Assets.Delete(ctx, a.ID)
	t.Logf("A-41 delete again: %v", err)
}
