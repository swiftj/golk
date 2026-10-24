//go:build integration

package examples

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

// Ports of the guide's §features and §Map images examples.

// TestFeatures ports "features".
func TestFeatures(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	fort := createResource(t, c, lk.ResourceCreate{Name: "The Tree Fort"})
	m := page(ooo)

	homes, err := c.Features.Create(ctx, m, lk.FeatureCreate{Type: lk.Some(lk.FeatureFolder), Name: lk.Some("Homes")})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := c.Features.Create(ctx, m, lk.FeatureCreate{
		Type:      lk.Some(lk.FeaturePin),
		Name:      lk.Some("The Tree Fort"),
		Pos:       lk.Some([2]float64{12.5, -40.25}),
		ParentID:  lk.Some(homes.ID),
		URI:       lk.Some("lk://resources/" + fort.ID),
		IconShape: lk.Some(lk.IconShapeNone),
	})
	if err != nil {
		t.Fatal(err)
	}
	ice, err := c.Features.Create(ctx, m, lk.FeatureCreate{
		Type:        lk.Some(lk.FeatureRegion),
		Name:        lk.Some("Ice Kingdom"),
		Polygon:     lk.Some([][2]float64{{40, -60}, {40, -20}, {30, -20}, {30, -60}}),
		Color:       lk.Some("#a8d8ff"),
		FillOpacity: lk.Some(0.3),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Features.Update(ctx, m, pin.ID, lk.FeatureUpdate{Pos: lk.Some([2]float64{12.75, -40.5}), IconGlyph: lk.Some("tree")}); err != nil {
		t.Fatal(err)
	}
	moved, err := c.Features.Get(ctx, m, pin.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(moved.Name, moved.Pos.Or([2]float64{}), moved.ParentID == homes.ID, moved.URI, moved.IconShape)
	if moved.ParentID != homes.ID || moved.IconShape != lk.IconShapeNone || moved.Pos.Or([2]float64{}) != [2]float64{12.75, -40.5} {
		t.Errorf("moved = %+v", moved)
	}

	fortPage := fort.Documents[0]
	if _, err := c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some("lk://documents/" + fortPage.ID)}); err != nil {
		t.Fatal(err)
	}
	_, err = c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some("https://example.com/ice-kingdom")})
	var apiErr *lk.APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, lk.ErrValidation) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	for _, issue := range apiErr.Errors {
		t.Logf("%s at %s", apiErr.Code, issue.Path)
	}
	got, err := c.Features.Get(ctx, m, ice.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(got.URI)
	if got.URI != "lk://documents/"+fortPage.ID {
		t.Errorf("uri = %q", got.URI)
	}
	if _, err := c.Features.Update(ctx, m, ice.ID, lk.FeatureUpdate{URI: lk.Some("")}); err != nil {
		t.Fatal(err)
	}

	for f, err := range c.Features.All(ctx, m, lk.FeatureListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(f.Type, f.Name)
	}
	pins, err := c.Features.List(ctx, m, lk.FeatureListParams{Type: lk.FeaturePin})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(pins.Items), "pins")
	if len(pins.Items) != 1 {
		t.Errorf("%d pins, want 1", len(pins.Items))
	}

	if err := c.Features.Delete(ctx, m, ice.ID); err != nil {
		t.Fatal(err)
	}
}

type kingdom struct {
	name  string
	pixel [2]float64
	color color.RGBA
}

var kingdoms = []kingdom{
	{"Candy Kingdom", [2]float64{520, 420}, color.RGBA{255, 160, 200, 255}},
	{"Ice Kingdom", [2]float64{220, 170}, color.RGBA{220, 240, 255, 255}},
	{"Fire Kingdom", [2]float64{830, 610}, color.RGBA{255, 110, 40, 255}},
}

// oooPNG draws the guide's map: grass with a disc of colour per kingdom.
func oooPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	grass := color.RGBA{120, 200, 90, 255}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := grass
			for _, k := range kingdoms {
				if math.Hypot(float64(x)-k.pixel[0], float64(y)-k.pixel[1]) < 110 {
					c = k.color
					break
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestMapImages ports "Map images".
func TestMapImages(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	art, err := c.Assets.UploadImage(ctx, lk.UploadImageParams{
		Data: oooPNG(t, 1024, 768), ContentType: "image/png", Name: sandbox.Name("Map of Ooo"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, art.ID)
	m := page(ooo)

	updated, err := c.Documents.Update(ctx, m, lk.DocumentUpdate{MapImage: lk.Some(lk.MapImageRef{AssetID: art.ID})})
	if err != nil {
		t.Fatal(err)
	}
	img := updated.MapImage
	if img == nil {
		t.Fatal("the map has no image")
	}
	t.Log(img.Width, img.Height, img.Bounds, img.AssetID == art.ID)
	if img.Width != 1024 || img.Height != 768 || img.AssetID != art.ID {
		t.Errorf("mapImage = %+v", img)
	}

	for _, k := range kingdoms {
		if _, err := c.Features.Create(ctx, m, lk.FeatureCreate{
			Type: lk.Some(lk.FeaturePin), Name: lk.Some(k.name), Pos: lk.Some(lk.PixelToPos(*img, k.pixel[0], k.pixel[1])),
		}); err != nil {
			t.Fatal(err)
		}
	}

	pins, err := c.Features.List(ctx, m, lk.FeatureListParams{Type: lk.FeaturePin})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]float64{}
	for _, k := range kingdoms {
		want[k.name] = k.pixel
	}
	for _, p := range pins.Items {
		pos, ok := p.Pos.Get()
		if !ok {
			continue
		}
		x, y := lk.PosToPixel(*img, pos)
		t.Log(p.Name, pos, [2]float64{x, y})
		if w := want[p.Name]; math.Abs(x-w[0]) > 1e-6 || math.Abs(y-w[1]) > 1e-6 {
			t.Errorf("%s: pixel %v read back as (%v, %v)", p.Name, w, x, y)
		}
	}
	if len(pins.Items) != len(kingdoms) {
		t.Errorf("%d pins, want %d", len(pins.Items), len(kingdoms))
	}
}
