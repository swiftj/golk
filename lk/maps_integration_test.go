//go:build integration

package lk_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestIntegration_Features ports the guide's §features example and checks
// C-FEAT-01..04 against the sandbox.
func TestIntegration_Features(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := create(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	fort := create(t, c, lk.ResourceCreate{Name: "The Tree Fort"})
	m := firstDoc(ooo)

	homes, err := c.Features.Create(ctx, m, lk.FeatureCreate{Type: lk.Some(lk.FeatureFolder), Name: lk.Some("Homes")})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := c.Features.Create(ctx, m, lk.FeatureCreate{
		Type: lk.Some(lk.FeaturePin), Name: lk.Some("The Tree Fort"), Pos: lk.Some([2]float64{12.5, -40.25}),
		ParentID: lk.Some(homes.ID), URI: lk.Some("lk://resources/" + fort.ID), IconShape: lk.Some(lk.IconShapeNone),
	})
	if err != nil {
		t.Fatal(err)
	}
	ice, err := c.Features.Create(ctx, m, lk.FeatureCreate{
		Type: lk.Some(lk.FeatureRegion), Name: lk.Some("Ice Kingdom"),
		Polygon: lk.Some([][2]float64{{40, -60}, {40, -20}, {30, -20}, {30, -60}}),
		Color:   lk.Some("#a8d8ff"), FillOpacity: lk.Some(0.3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ice.Polygon) != 4 || ice.Polygon[1] != [2]float64{40, -20} || ice.FillOpacity.Or(-1) != 0.3 {
		t.Errorf("region = %+v", ice)
	}
	d, err := c.Documents.Get(ctx, m)
	if err != nil || d.Type != lk.DocumentMap || d.MapImage != nil {
		t.Errorf("blank document after the first feature = %+v, %v; want a map without an image", d, err)
	}

	if _, err := c.Features.Update(ctx, m, pin.ID, lk.FeatureUpdate{Pos: lk.Some([2]float64{12.75, -40.5}), IconGlyph: lk.Some("tree")}); err != nil {
		t.Fatal(err)
	}
	moved, err := c.Features.Get(ctx, m, pin.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(moved.Name, moved.Pos.Or([2]float64{}), moved.ParentID == homes.ID, moved.URI, moved.IconShape)
	if moved.Pos.Or([2]float64{}) != [2]float64{12.75, -40.5} || moved.ParentID != homes.ID || moved.IconGlyph != "tree" ||
		moved.URI != "lk://resources/"+fort.ID || moved.IconShape != lk.IconShapeNone || moved.Name != "The Tree Fort" {
		t.Errorf("moved pin = %+v", moved)
	}

	fortPage := "lk://documents/" + fort.Documents[0].ID
	if _, err := c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some(fortPage)}); err != nil {
		t.Fatal(err)
	}
	_, err = c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some("https://example.com/ice-kingdom")})
	var apiErr *lk.APIError
	if !errors.Is(err, lk.ErrValidation) || !errors.As(err, &apiErr) {
		t.Fatalf("https uri: err = %v, want VALIDATION_ERROR", err)
	}
	for _, issue := range apiErr.Errors {
		t.Logf("%s at %s", apiErr.Code, issue.Path)
	}
	if len(apiErr.Errors) == 0 || apiErr.Errors[0].Path != "uri" {
		t.Errorf("field errors = %+v, want one at uri", apiErr.Errors)
	}
	if got, err := c.Features.Get(ctx, m, ice.ID); err != nil || got.URI != fortPage {
		t.Errorf("uri after the refused update = %q, %v; want %q", got.URI, err, fortPage)
	}
	if _, err := c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some("")}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Features.Get(ctx, m, ice.ID); err != nil || got.URI != "" {
		t.Errorf("uri after clearing = %q, %v", got.URI, err)
	}

	all, err := c.Features.List(ctx, m, lk.FeatureListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all.Items {
		t.Log(f.Type, f.Name)
	}
	if len(all.Items) != 3 {
		t.Errorf("listed %d features, want 3", len(all.Items))
	}
	pins, err := c.Features.List(ctx, m, lk.FeatureListParams{Type: lk.FeaturePin})
	if err != nil || len(pins.Items) != 1 || pins.Items[0].ID != pin.ID {
		t.Errorf("pins = %+v, %v", pins, err)
	}

	if err := c.Features.Delete(ctx, m, ice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Features.Get(ctx, m, ice.ID); !errors.Is(err, lk.ErrNotFound) {
		t.Errorf("get after delete: err = %v, want NOT_FOUND", err)
	}
	// C-FEAT-04: deleting a folder leaves its features naming it.
	if err := c.Features.Delete(ctx, m, homes.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Features.Get(ctx, m, pin.ID); err != nil || got.ParentID != homes.ID {
		t.Errorf("pin after its folder was deleted = %+v, %v", got, err)
	}
}

// gridPNG draws a w by h PNG.
func gridPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{R: uint8(x), G: 0xc8, B: uint8(y), A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestIntegration_MapImage checks C-DOC-10's mapImage and C-MAP-01 against
// the sandbox: the bounds have the spec's shape, and pins placed with
// PixelToPos read back to the same pixels with PosToPixel.
func TestIntegration_MapImage(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := create(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	art, err := c.Assets.UploadImage(ctx, lk.UploadImageParams{Data: gridPNG(t, 512, 256), Name: sandbox.Name("Map of Ooo")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, art.ID)
	m := firstDoc(ooo)

	updated, err := c.Documents.Update(ctx, m, lk.DocumentUpdate{MapImage: lk.Some(lk.MapImageRef{AssetID: art.ID})})
	if err != nil {
		t.Fatal(err)
	}
	img := updated.MapImage
	if img == nil {
		t.Fatal("the updated document has no mapImage")
	}
	t.Logf("mapImage %dx%d bounds %v asset %v tiles %+v", img.Width, img.Height, img.Bounds, img.AssetID == art.ID, img.Tiles)
	if updated.Type != lk.DocumentMap || img.AssetID != art.ID || img.URL == "" || img.Width != 512 || img.Height != 256 {
		t.Errorf("document = %+v, mapImage = %+v", updated, img)
	}
	south, west, north, east := img.Bounds[0][0], img.Bounds[0][1], img.Bounds[1][0], img.Bounds[1][1]
	if north != 0 || west != 0 || south >= 0 || east <= 0 {
		t.Errorf("bounds = %v, want north and west 0, south negative, east positive", img.Bounds)
	}
	// Both the spec's formula and ours give a frame with the image's aspect.
	if math.Abs(east/(-south)-float64(img.Width)/float64(img.Height)) > 1e-9 {
		t.Errorf("bounds %v do not have the image's aspect %dx%d", img.Bounds, img.Width, img.Height)
	}
	if img.Tiles == nil || img.Tiles.URLTemplate == "" {
		t.Errorf("tiles = %+v", img.Tiles)
	}
	got, err := c.Documents.Get(ctx, m)
	if err != nil || got.MapImage == nil || got.MapImage.Bounds != img.Bounds {
		t.Errorf("get mapImage = %+v, %v", got, err)
	}

	pixels := map[string][2]float64{"Candy Kingdom": {260, 210}, "Ice Kingdom": {110, 85}, "Fire Kingdom": {415, 0}}
	for name, px := range pixels {
		if _, err := c.Features.Create(ctx, m, lk.FeatureCreate{
			Type: lk.Some(lk.FeaturePin), Name: lk.Some(name), Pos: lk.Some(lk.PixelToPos(*img, px[0], px[1])),
		}); err != nil {
			t.Fatal(err)
		}
	}
	pins, err := c.Features.List(ctx, m, lk.FeatureListParams{Type: lk.FeaturePin})
	if err != nil {
		t.Fatal(err)
	}
	if len(pins.Items) != len(pixels) {
		t.Fatalf("listed %d pins, want %d", len(pins.Items), len(pixels))
	}
	for _, p := range pins.Items {
		pos, ok := p.Pos.Get()
		if !ok {
			t.Errorf("pin %s has no pos", p.Name)
			continue
		}
		x, y := lk.PosToPixel(*img, pos)
		t.Log(p.Name, pos, x, y)
		want := pixels[p.Name]
		if math.Abs(x-want[0]) > 1e-6 || math.Abs(y-want[1]) > 1e-6 {
			t.Errorf("%s: pixel %v read back as (%v, %v)", p.Name, want, x, y)
		}
	}
}
