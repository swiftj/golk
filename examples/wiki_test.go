//go:build integration

package examples

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"slices"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// Ports of the guide's Writing content examples and the wiki part of its
// Worked example. Each uses the guarded sandbox client and made-up names.

func createResource(t *testing.T, c *lk.Client, r lk.ResourceCreate) *lk.Resource {
	t.Helper()
	r.Name = sandbox.Name(r.Name)
	res, err := c.Resources.Create(sandbox.Ctx(t), r)
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, res.ID)
	return res
}

func page(r *lk.Resource) lk.DocRef {
	return lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}
}

// TestTextStyles ports "Text styles".
func TestTextStyles(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	candy := createResource(t, c, lk.ResourceCreate{Name: "Candy Kingdom"})
	pb := createResource(t, c, lk.ResourceCreate{Name: "Princess Bubblegum"})
	written, err := c.Documents.ReplaceBlocks(ctx, page(candy), blocks.List{
		blocks.Styled(blocks.H(1, "Candy Kingdom"), blocks.Style{Align: lk.Some(blocks.AlignCenter)}),
		blocks.Styled(blocks.P("Ruled by ", blocks.Color("#EC4899", blocks.MentionOf(pb.ID, "Princess Bubblegum")), "."), blocks.Style{Background: lk.Some("#14B8A6")}),
		blocks.P("The castle is ", blocks.Highlight("#F59E0B", "made of candy"), "."),
		blocks.Styled(blocks.Quote("Science is the best kind of magic."), blocks.Style{Color: lk.Some("#A855F7")}),
		blocks.Layout(blocks.Styled(blocks.Column(50, "Candy Castle"), blocks.Style{Background: lk.Some("#3B82F6")}), blocks.Column(50, "Candy Village")),
	})
	if err != nil {
		t.Fatal(err)
	}
	heading := written.Blocks[0].(*blocks.HeadingBlock)
	paragraph := written.Blocks[1].(*blocks.ParagraphBlock)
	pull := written.Blocks[3].(*blocks.QuoteBlock)
	castle := written.Blocks[4].(*blocks.LayoutBlock).Children[0].(*blocks.LayoutColumnBlock)
	t.Log(heading.Align.Or(""), paragraph.Background.Or(""), pull.Color.Or(""), castle.Background.Or(""))
	if heading.Align.Or("") != blocks.AlignCenter || !strings.EqualFold(pull.Color.Or(""), "#A855F7") || castle.Background.Or("") == "" {
		t.Errorf("styles read back: %q %q %q %q", heading.Align.Or(""), paragraph.Background.Or(""), pull.Color.Or(""), castle.Background.Or(""))
	}
}

// TestTitlesAndFill ports "Block titles and image fill".
func TestTitlesAndFill(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	portrait, err := c.Assets.UploadImage(ctx, lk.UploadImageParams{Data: solidPNG(1, 1, [3]byte{0, 0, 0}), ContentType: "image/png", Name: sandbox.Name("The Tree Fort")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, portrait.ID)
	fort := createResource(t, c, lk.ResourceCreate{Name: "The Tree Fort"})
	finn := createResource(t, c, lk.ResourceCreate{Name: "Finn the Human"})
	jake := createResource(t, c, lk.ResourceCreate{Name: "Jake the Dog"})

	img := blocks.Image(portrait.URL, "The Tree Fort")
	img.Fill = lk.Some(true)
	hidden := blocks.Collection(finn.ID, jake.ID)
	hidden.IsTitleHidden = lk.Some(true)
	stretch := blocks.Collection(jake.ID)
	stretch.Title, stretch.IsTitleHidden = lk.Some("Can stretch"), lk.Some(false)
	written, err := c.Documents.ReplaceBlocks(ctx, page(fort), blocks.List{
		blocks.Infobox(blocks.AsProperty(img, blocks.Envelope{Title: lk.Some("Portrait"), IsTitleHidden: lk.Some(true)})),
		blocks.P("A hollow tree in the Grasslands, home to two heroes."),
		blocks.H(2, "Residents"),
		hidden,
		stretch,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range written.Blocks {
		if rc, ok := b.(*blocks.ResourceCollectionBlock); ok {
			t.Log(rc.Title.Or(""), rc.IsTitleHidden.Or(false))
		}
	}
	residents := written.Blocks[3]
	shown, err := c.Blocks.Update(ctx, page(fort), residents.NodeID(), lk.BlockPatch{Title: lk.Some("Everyone"), IsTitleHidden: lk.Some(false)}, lk.IfMatch(written.Version))
	if err != nil {
		t.Fatal(err)
	}
	rc := shown.Value.(*blocks.ResourceCollectionBlock)
	if rc.Title.Or("") != "Everyone" || rc.IsTitleHidden.Or(true) {
		t.Errorf("shown = %s %v", rc.Title.Or(""), rc.IsTitleHidden.Or(true))
	}
}

// TestKeyValueRows ports "Key-value rows".
func TestKeyValueRows(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	bmo := createResource(t, c, lk.ResourceCreate{Name: "BMO"})
	written, err := c.Documents.ReplaceBlocks(ctx, page(bmo), blocks.List{
		blocks.KeyValue(blocks.KV("Lives in", "The Tree Fort"), blocks.KV("Plays", "Video games")),
	})
	if err != nil {
		t.Fatal(err)
	}
	specs := written.Blocks[0].NodeID()
	rows, err := c.Blocks.ListRows(ctx, page(bmo), specs)
	if err != nil {
		t.Fatal(err)
	}
	plays := rows.Items[1]
	if _, err := c.Blocks.CreateRow(ctx, page(bmo), specs, lk.RowCreate{
		Key: "Best friend", Content: blocks.R(blocks.Bold("Football"), blocks.BR(), "the reflection in the bathroom mirror"), After: lk.Some(plays.ID),
	}, lk.IfMatch(rows.Version)); err != nil {
		t.Fatal(err)
	}
	rows, err = c.Blocks.ListRows(ctx, page(bmo), specs)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, r := range rows.Items {
		lines = append(lines, r.Key+": "+r.Text.Or(""))
	}
	t.Log(strings.Join(lines, "\n"))
	if want := "Best friend: Football\nthe reflection in the bathroom mirror"; lines[2] != want {
		t.Errorf("row 3 = %q, want %q", lines[2], want)
	}
}

// TestInfoboxes ports "Infoboxes".
func TestInfoboxes(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	jake := createResource(t, c, lk.ResourceCreate{Name: "Jake the Dog"})
	written, err := c.Documents.ReplaceBlocks(ctx, page(jake), blocks.List{
		blocks.Infobox(blocks.AsProperty(blocks.KeyValue(blocks.KV("Species", "Magic dog"), blocks.KV("Age", "28")), blocks.Envelope{Title: lk.Some("Stats")})),
		blocks.P("Finn's brother. Stretches into anything, including a boat, a key and a very large hand."),
	})
	if err != nil {
		t.Fatal(err)
	}
	sheet := written.Blocks[0].NodeID()
	stats := written.Blocks[0].(*blocks.InfoboxBlock).Rows[0].NodeID()
	if _, err := c.Blocks.CreateProperty(ctx, page(jake), sheet,
		blocks.AsProperty(blocks.TextField("Got his powers from rolling in a magic mud puddle."), blocks.Envelope{Title: lk.Some("Origin")}),
		lk.Place{Before: stats}, lk.IfMatch(written.Version)); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Blocks.ListPropertyRows(ctx, page(jake), sheet, stats)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows.Items {
		t.Logf("%s: %s", r.Key, r.Text.Or(""))
	}
	items, err := c.Blocks.ListProperties(ctx, page(jake), sheet)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, p := range items.Items {
		titles = append(titles, p.PropertyEnvelope().Title.Or(""))
	}
	if !slices.Equal(titles, []string{"Origin", "Stats"}) {
		t.Errorf("infobox titles = %v", titles)
	}
}

// TestWorkedExampleWiki ports the wiki part of the guide's Worked example:
// a world with three kingdoms, each with a sidebar property and a page, an
// overview page that mentions every kingdom, and the uploaded map image.
// The map pins and the timeline are phase 4 (maps and timelines).
func TestWorkedExampleWiki(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	type kingdom struct {
		name, ruler, summary string
		landmarks            []string
		color                [3]byte
	}
	kingdoms := []kingdom{
		{"Candy Kingdom", "Princess Bubblegum", "Candy people, a castle, and a princess who runs it like a lab.", []string{"Candy Castle", "Candy Village"}, [3]byte{255, 160, 200}},
		{"Ice Kingdom", "The Ice King", "A lonely king, a mountain of ice and far too many penguins.", []string{"Ice Castle", "The fan fiction shelf"}, [3]byte{220, 240, 255}},
		{"Fire Kingdom", "Flame Princess", "Hot enough that Finn needed a fire shield to visit.", []string{"Fire Palace"}, [3]byte{255, 110, 40}},
	}
	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo", Tags: lk.Some([]string{"World"})})
	art, err := c.Assets.UploadImage(ctx, lk.UploadImageParams{Data: solidPNG(64, 48, [3]byte{120, 200, 90}), ContentType: "image/png", Name: sandbox.Name("Map of Ooo")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, art.ID)

	var created []*lk.Resource
	for _, k := range kingdoms {
		res, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name(k.name), ParentID: lk.Some(ooo.ID), Tags: lk.Some([]string{"Kingdom"})})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, res)
		if _, err := c.Properties.Create(ctx, res.ID, blocks.AsProperty(blocks.KeyValue(blocks.KV("Ruler", k.ruler)), blocks.Envelope{Title: lk.Some("Rule")}), lk.Place{}); err != nil {
			t.Fatal(err)
		}
		landmarks := make([]any, len(k.landmarks))
		for i, l := range k.landmarks {
			landmarks[i] = l
		}
		written, err := c.Documents.ReplaceBlocks(ctx, page(res), blocks.List{
			blocks.H(2, k.name), blocks.P(k.summary, " Part of ", blocks.MentionOf(ooo.ID, ""), "."), blocks.UL(landmarks[0], landmarks[1:]...),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := blocks.PlainText(written.Blocks); got != written.Text {
			t.Errorf("PlainText = %q, server %q", got, written.Text)
		}
		t.Log(blocks.PlainText(written.Blocks))
	}

	overview := blocks.List{blocks.H(2, "Kingdoms")}
	for _, k := range created {
		overview = append(overview, blocks.P(blocks.MentionOf(k.ID, "")))
	}
	overview = append(overview, blocks.Callout(blocks.ToneNote, "Everything here grew back after the Mushroom War."))
	written, err := c.Documents.ReplaceBlocks(ctx, page(ooo), overview)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range created {
		if !strings.Contains(written.Text, k.Name) {
			t.Errorf("overview text lacks %s: %q", k.Name, written.Text)
		}
	}
}

// solidPNG draws a w by h PNG in one color, the way the guide's example
// builds its image in code.
func solidPNG(w, h int, rgb [3]byte) []byte {
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
		return b.Bytes()
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:], uint32(w))
	binary.BigEndian.PutUint32(header[4:], uint32(h))
	copy(header[8:], []byte{8, 2, 0, 0, 0})
	var rows bytes.Buffer
	zw := zlib.NewWriter(&rows)
	for range h {
		_, _ = zw.Write([]byte{0})
		for range w {
			_, _ = zw.Write(rgb[:])
		}
	}
	_ = zw.Close()
	var out bytes.Buffer
	out.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	out.Write(chunk("IHDR", header))
	out.Write(chunk("IDAT", rows.Bytes()))
	out.Write(chunk("IEND", nil))
	return out.Bytes()
}
