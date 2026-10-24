//go:build integration

package lk_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

func firstDoc(r *lk.Resource) lk.DocRef {
	return lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}
}

func solidPNG(t *testing.T, rgb color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, rgb)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func upload(t *testing.T, c *lk.Client, name string) *lk.Asset {
	t.Helper()
	a, err := c.Assets.UploadImage(sandbox.Ctx(t), lk.UploadImageParams{
		Data: solidPNG(t, color.RGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff}), Name: sandbox.Name(name),
	})
	if err != nil {
		t.Fatalf("upload %s: %v", name, err)
	}
	sandbox.CleanupAsset(t, a.ID)
	return a
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(x)
	return string(b)
}

func TestIntegration_C_CONTENT_02_BlankConvertsOnFirstWrite(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	bmo := create(t, c, lk.ResourceCreate{Name: "BMO"})
	doc := firstDoc(bmo)

	empty, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil || len(empty.Blocks) != 0 || empty.Version == "" {
		t.Fatalf("blank page = %+v, %v", empty, err)
	}
	if d, err := c.Documents.Get(ctx, doc); err != nil || d.Type != lk.DocumentBlank {
		t.Fatalf("a read converted the document: %+v %v", d, err)
	}
	written, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.P("Who wants to play video games?")}, lk.IfMatch(empty.Version))
	if err != nil {
		t.Fatal(err)
	}
	if d, err := c.Documents.Get(ctx, doc); err != nil || d.Type != lk.DocumentPage {
		t.Fatalf("after the first write: %+v %v", d, err)
	}
	if written.Text != "Who wants to play video games?" || written.Blocks[0].NodeID() == "" {
		t.Errorf("written = %+v", written)
	}
}

func TestIntegration_C_CONTENT_03_ImageURLs(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	fort := create(t, c, lk.ResourceCreate{Name: "The Tree Fort"})
	doc := firstDoc(fort)
	art := upload(t, c, "Tree Fort portrait")
	if art.Type != lk.AssetImage || art.Width != 8 || art.Height != 8 || art.URL == "" {
		t.Fatalf("asset = %+v", art)
	}
	if _, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.Image(art.URL, "A green square")}); err != nil {
		t.Fatalf("own asset URL refused: %v", err)
	}
	for _, url := range []string{"data:image/png;base64,AAAA", "https://images.example.com/fort.png"} {
		_, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.Image(url, "")})
		var e *lk.APIError
		if !errors.As(err, &e) || e.Code != lk.CodeValidationError || len(e.Errors) == 0 || e.Errors[0].Path != "blocks[0].url" {
			t.Errorf("%s: %v", url[:10], err)
		}
	}
}

// everyKind is a page with one of every writable block kind.
func everyKind(art *lk.Asset, other string) blocks.List {
	cell := blocks.TH("Name")
	cell.Colspan = lk.Some(2)
	ol := blocks.OL("First", "Second")
	ol.Start = lk.Some(3)
	custom := blocks.Callout(blocks.ToneCustom, "Here be dragons")
	custom.Icon, custom.Color = lk.Some("dragon"), lk.Some("#aa0000")
	img := blocks.Image(art.URL, "A green square")
	img.Fill, img.Layout, img.Width = lk.Some(true), lk.Some(blocks.LayoutWrapLeft), lk.Some(30)
	return blocks.List{
		blocks.Styled(blocks.H(1, "Candy Kingdom"), blocks.Style{Align: lk.Some(blocks.AlignCenter)}),
		blocks.Styled(blocks.P("Ruled by ", blocks.Color("#EC4899", blocks.MentionOf(other, "Princess Bubblegum")), ", ",
			blocks.Bold(blocks.Italic("Mathematical!")), blocks.BR(), blocks.Mono("x := 1"), " ", blocks.Link("https://example.com", "link"),
			" H", blocks.Sub("2"), "O ", blocks.Highlight("#F59E0B", "made of candy")), blocks.Style{Background: lk.Some("#14B8A6")}),
		blocks.Styled(blocks.Quote("Science is the best kind of magic."), blocks.Style{Color: lk.Some("#A855F7")}),
		blocks.Code("print('candy')", "python"),
		blocks.Rule(),
		blocks.UL("Treasure room", blocks.LI("Outer", blocks.UL("Inner"))),
		ol,
		blocks.Tasks(blocks.Task(true, "Done"), blocks.Task(false, "Not yet")),
		blocks.Table(blocks.TR(cell), blocks.TR(blocks.TD("Finn"), blocks.TD("Jake"))),
		blocks.Callout(blocks.ToneWarning, "Beware the Lich"),
		custom,
		blocks.Expand("More", "Hidden details"),
		blocks.Layout(blocks.Styled(blocks.Column(50, "Candy Castle"), blocks.Style{Background: lk.Some("#3B82F6")}), blocks.Column(50, "Candy Village")),
		img,
		blocks.Embed(blocks.EmbedYouTube, "https://www.youtube.com/watch?v=dQw4w9WgXcQ"),
		blocks.KeyValue(blocks.KV("Lives in", "The Tree Fort"), blocks.KV("Plays", blocks.Bold("Video games"))),
		blocks.Infobox(
			blocks.AsProperty(blocks.KeyValue(blocks.KV("Species", "Magic dog")), blocks.Envelope{Title: lk.Some("Stats")}),
			blocks.AsProperty(blocks.TextField("Got his powers from a magic mud puddle."), blocks.Envelope{Title: lk.Some("Origin")}),
		),
		blocks.Meter(blocks.MeterBar, blocks.MeterItem{Name: "HP", Icon: "heart", Current: 3, Max: 5}),
		blocks.Collection(other),
		blocks.CollectionWhere(blocks.Filter{ID: "f1", FilterType: "tag", Operator: lk.Some("has_any"), Tags: []string{"Kingdom"}}),
		blocks.Links(other),
		blocks.EmptyMap(),
		blocks.Secret("The Lich is not really gone."),
		blocks.TextField("A rich-text container."),
		blocks.P("Last"),
	}
}

// TestIntegration_EveryKindRoundTrips writes one of every kind, reads the
// page back, and writes what it read: nothing may change (C-BLOCK-01,
// C-BLOCK-04, C-CONTENT-04), and PlainText matches the server (C-READ-01).
func TestIntegration_EveryKindRoundTrips(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	pb := create(t, c, lk.ResourceCreate{Name: "Princess Bubblegum"})
	candy := create(t, c, lk.ResourceCreate{Name: "Candy Kingdom"})
	art := upload(t, c, "Candy portrait")
	doc := firstDoc(candy)

	written, err := c.Documents.ReplaceBlocks(ctx, doc, everyKind(art, pb.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range written.Blocks {
		if _, raw := b.(*blocks.RawBlock); raw {
			t.Errorf("%s came back as a raw block: %s", b.Kind(), b.(*blocks.RawBlock).JSON)
		}
	}
	if got := blocks.PlainText(written.Blocks); got != written.Text {
		t.Errorf("PlainText\n got %q\nwant %q", got, written.Text)
	}

	read, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Documents.ReplaceBlocks(ctx, doc, read.Blocks, lk.IfMatch(read.Version))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := canonical(t, read.Blocks), canonical(t, again.Blocks); a != b {
		t.Errorf("read-modify-write changed the page:\nread  %s\nwrote %s", a, b)
	}

	// Walk paths match what the server reports for a bad nested write.
	var cellPath string
	blocks.Walk(read.Blocks, func(path string, n blocks.Node) bool {
		if n.Kind() == blocks.KindTableCell {
			cellPath = path
			return false
		}
		return true
	})
	tbl := read.Blocks[8].(*blocks.TableBlock)
	tbl.Children[0].(*blocks.TableRowBlock).Children[0].(*blocks.TableCellBlock).Colspan = lk.Some(0)
	_, err = c.Documents.ReplaceBlocks(ctx, doc, read.Blocks)
	var e *lk.APIError
	if !errors.As(err, &e) || len(e.Errors) == 0 || !strings.HasPrefix(e.Errors[0].Path, cellPath) {
		t.Errorf("error path for %s: %v", cellPath, err)
	}
}

// TestIntegration_BlockOperations ports the guide's Block operations
// example, plus a stale If-Match (C-VER-02, C-ERR-12).
func TestIntegration_BlockOperations(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	fort := create(t, c, lk.ResourceCreate{Name: "The Tree Fort"})
	doc := firstDoc(fort)

	written, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{blocks.H(1, "The Tree Fort"), blocks.UL("Treasure room", "BMO's charging nook")})
	if err != nil {
		t.Fatal(err)
	}
	titleID, roomsID := written.Blocks[0].NodeID(), written.Blocks[1].NodeID()
	version := written.Version

	intro, err := c.Blocks.Insert(ctx, doc, blocks.P("Where Finn and Jake live."), lk.At{After: titleID}, lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	introID, version := intro.Value.NodeID(), intro.Version

	updated, err := c.Blocks.Update(ctx, doc, introID, lk.BlockPatch{
		Content: lk.Some(blocks.R("Where Finn and Jake live, with ", blocks.Bold("BMO"), ".")),
	}, lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	version = updated.Version

	replaced, err := c.Blocks.Replace(ctx, doc, titleID, blocks.H(2, "The Tree Fort, home of heroes"), lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Value.NodeID() != titleID {
		t.Errorf("replace changed the id: %s", replaced.Value.NodeID())
	}
	version = replaced.Version

	moved, err := c.Blocks.Move(ctx, doc, roomsID, lk.At{Before: introID}, lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	version = moved.Version

	rooms, err := c.Blocks.Get(ctx, doc, roomsID)
	if err != nil || rooms.Value.Kind() != blocks.KindBulletList || rooms.Version != version {
		t.Fatalf("get = %+v, %v (want version %s)", rooms, err, version)
	}

	// A write at an old version is refused and reports the current one.
	err = c.Blocks.Delete(ctx, doc, roomsID, lk.IfMatch(written.Version))
	var e *lk.APIError
	if !errors.Is(err, lk.ErrVersionMismatch) || !errors.As(err, &e) || e.CurrentVersion != version {
		t.Fatalf("stale delete: %v (current %q, want %q)", err, e.CurrentVersion, version)
	}
	if err := c.Blocks.Delete(ctx, doc, roomsID, lk.IfMatch(version)); err != nil {
		t.Fatal(err)
	}
	ids, err := c.Documents.AssignBlockIDs(ctx, doc)
	if err != nil || ids.Assigned != 0 || ids.Version == "" {
		t.Fatalf("assignBlockIds = %+v, %v", ids, err)
	}
	final, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := final.Text; got != "The Tree Fort, home of heroes\nWhere Finn and Jake live, with BMO." {
		t.Errorf("final text = %q", got)
	}
}

func TestIntegration_RowsAndProperties(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	jake := create(t, c, lk.ResourceCreate{Name: "Jake the Dog"})
	doc := firstDoc(jake)

	// Sidebar properties are unversioned; their rows are versioned.
	stats, err := c.Properties.Create(ctx, jake.ID, blocks.AsProperty(blocks.KeyValue(blocks.KV("Species", "Magic dog")),
		blocks.Envelope{Title: lk.Some("Stats")}), lk.Place{})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := c.Properties.Create(ctx, jake.ID, blocks.AsProperty(blocks.TextField("Rolled in a magic mud puddle."),
		blocks.Envelope{Title: lk.Some("Origin")}), lk.Place{Before: stats.NodeID()})
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.Properties.List(ctx, jake.ID)
	if err != nil || len(list.Items) != 2 || list.Items[0].NodeID() != origin.NodeID() {
		t.Fatalf("properties = %+v, %v", list, err)
	}
	rows, err := c.Properties.ListRows(ctx, jake.ID, stats.NodeID())
	if err != nil || len(rows.Items) != 1 || rows.Version == "" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	row, err := c.Properties.CreateRow(ctx, jake.ID, stats.NodeID(), lk.RowCreate{Key: "Age", Text: lk.Some("28")}, lk.IfMatch(rows.Version))
	if err != nil || row.Version == "" || row.Value.Text.Or("") != "28" {
		t.Fatalf("createRow = %+v, %v", row, err)
	}
	cleared, err := c.Properties.UpdateRow(ctx, jake.ID, stats.NodeID(), row.Value.ID, lk.RowPatch{Content: lk.Null[blocks.Runs]()}, lk.IfMatch(row.Version))
	if err != nil || cleared.Value.Text.Or("x") != "" {
		t.Fatalf("clear row = %+v, %v", cleared, err)
	}
	if err := c.Properties.DeleteRow(ctx, jake.ID, stats.NodeID(), row.Value.ID, lk.IfMatch(cleared.Version)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Properties.Update(ctx, jake.ID, stats.NodeID(), lk.PropertyPatch{Title: lk.Some("Facts"), IsHidden: lk.Some(true)}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Properties.Get(ctx, jake.ID, stats.NodeID()); err != nil || got.PropertyEnvelope().Title.Or("") != "Facts" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := c.Properties.Delete(ctx, jake.ID, origin.NodeID()); err != nil {
		t.Fatal(err)
	}

	// Infobox properties and their rows, and keyValue block rows.
	written, err := c.Documents.ReplaceBlocks(ctx, doc, blocks.List{
		blocks.Infobox(blocks.AsProperty(blocks.KeyValue(blocks.KV("Species", "Magic dog")), blocks.Envelope{Title: lk.Some("Stats")})),
		blocks.KeyValue(blocks.KV("Lives in", "The Tree Fort")),
	})
	if err != nil {
		t.Fatal(err)
	}
	box, kv := written.Blocks[0].NodeID(), written.Blocks[1].NodeID()
	props, err := c.Blocks.ListProperties(ctx, doc, box)
	if err != nil || len(props.Items) != 1 {
		t.Fatalf("infobox properties = %+v, %v", props, err)
	}
	added, err := c.Blocks.CreateProperty(ctx, doc, box, blocks.AsProperty(blocks.TextField("Stretches into anything."), blocks.Envelope{Title: lk.Some("Powers")}),
		lk.Place{Before: props.Items[0].NodeID()}, lk.IfMatch(props.Version))
	if err != nil {
		t.Fatal(err)
	}
	ibRows, err := c.Blocks.ListPropertyRows(ctx, doc, box, props.Items[0].NodeID())
	if err != nil || len(ibRows.Items) != 1 {
		t.Fatalf("infobox rows = %+v, %v", ibRows, err)
	}
	ibRow, err := c.Blocks.CreatePropertyRow(ctx, doc, box, props.Items[0].NodeID(), lk.RowCreate{Key: "Age", Text: lk.Some("28")}, lk.IfMatch(ibRows.Version))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Blocks.UpdateProperty(ctx, doc, box, added.Value.NodeID(), lk.PropertyPatch{IsTitleHidden: lk.Some(true)}, lk.IfMatch(ibRow.Version)); err != nil {
		t.Fatal(err)
	}
	kvRows, err := c.Blocks.ListRows(ctx, doc, kv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Blocks.CreateRow(ctx, doc, kv, lk.RowCreate{Key: "Plays", Content: blocks.R("Viola"), After: lk.Some(kvRows.Items[0].ID)}, lk.IfMatch(kvRows.Version)); err != nil {
		t.Fatal(err)
	}
	final, err := c.Documents.GetBlocks(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	ib := final.Blocks[0].(*blocks.InfoboxBlock)
	if len(ib.Rows) != 2 || ib.Rows[0].PropertyEnvelope().Title.Or("") != "Powers" || len(final.Blocks[1].(*blocks.KeyValueBlock).Rows) != 2 {
		t.Errorf("final page = %s", canonical(t, final.Blocks))
	}
}

func TestIntegration_DocumentsLifecycle(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ice := create(t, c, lk.ResourceCreate{Name: "Ice Kingdom", Documents: lk.Some([]lk.ResourceDocument{
		{Name: lk.Some("Main"), Blocks: blocks.List{blocks.P("A lonely king and far too many penguins.")}},
	})})
	main := firstDoc(ice)
	if ice.Documents[0].Type != lk.DocumentPage {
		t.Errorf("created with content: %+v", ice.Documents[0])
	}
	if got, err := c.Documents.GetBlocks(ctx, main); err != nil || got.Text != "A lonely king and far too many penguins." {
		t.Fatalf("content = %+v, %v", got, err)
	}
	lore, err := c.Documents.Create(ctx, ice.ID, lk.DocumentCreate{Name: "Lore", Type: lk.DocumentPage, IsHidden: lk.Some(true)})
	if err != nil || !lore.IsHidden {
		t.Fatalf("create = %+v, %v", lore, err)
	}
	ref := lk.DocRef{ResourceID: ice.ID, DocumentID: lore.ID}
	if d, err := c.Documents.Update(ctx, ref, lk.DocumentUpdate{Name: lk.Some("Penguin lore"), IsFullWidth: lk.Some(true)}); err != nil || d.Name != "Penguin lore" || !d.IsFullWidth || !d.IsHidden {
		t.Fatalf("update = %+v, %v", d, err)
	}
	var names []string
	for d, err := range c.Documents.All(ctx, ice.ID) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"Main", "Penguin lore"}) {
		t.Errorf("documents = %v", names)
	}
	if err := c.Documents.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Documents.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Documents.Restore(ctx, ref); !errors.Is(err, lk.ErrNotDeleted) {
		t.Fatalf("restoring a live document: %v", err)
	}
}

func TestIntegration_SearchAndTags(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	// A made-up word, so the only match is this run's resource.
	word := "zorblax" + strings.TrimPrefix(sandbox.RunPrefix, "golk-it-")
	gunter := create(t, c, lk.ResourceCreate{Name: "Gunter", Tags: lk.Some([]string{"Penguin"}), Documents: lk.Some([]lk.ResourceDocument{
		{Blocks: blocks.List{blocks.P("Gunter knows the " + word + " ritual.")}},
	})})

	// The index may lag a write; poll for a short while.
	var hits []lk.SearchHit
	for range 15 {
		page, err := c.Search.Search(ctx, lk.SearchParams{Query: word, Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if hits = page.Items; len(hits) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if len(hits) == 0 || hits[0].Resource.ID != gunter.ID || hits[0].Document == nil {
		t.Fatalf("search %s = %+v", word, hits)
	}
	if empty, err := c.Search.Search(ctx, lk.SearchParams{}); err != nil || len(empty.Items) != 0 {
		t.Errorf("empty query = %+v, %v", empty, err)
	}

	tags, err := c.Tags.List(ctx)
	if errors.Is(err, lk.ErrForbidden) {
		t.Skip("the sandbox key is not an admin; tags.list needs ADMIN")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(tags.Items, func(tg lk.Tag) bool { return tg.Tag == "Penguin" && tg.Count >= 1 }) {
		t.Errorf("tags = %+v", tags.Items)
	}
}

func TestIntegration_AssetFolders(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	folder, err := c.Assets.CreateFolder(ctx, lk.AssetFolderCreate{Name: sandbox.Name("Treasure")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.CleanupAsset(t, folder.ID)
	if folder.Type != lk.AssetFolder {
		t.Errorf("folder = %+v", folder)
	}
	art := upload(t, c, "Gold")
	if err := c.Assets.Update(ctx, art.ID, lk.AssetUpdate{FolderID: lk.Value(folder.ID), Description: lk.Some("Shiny")}); err != nil {
		t.Fatal(err)
	}
	var inFolder []string
	for a, err := range c.Assets.All(ctx, lk.AssetListParams{FolderID: folder.ID}) {
		if err != nil {
			t.Fatal(err)
		}
		inFolder = append(inFolder, a.ID)
	}
	if !slices.Equal(inFolder, []string{art.ID}) {
		t.Errorf("folder contents = %v", inFolder)
	}
	if err := c.Assets.Update(ctx, art.ID, lk.AssetUpdate{FolderID: lk.Null[string]()}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Assets.Get(ctx, art.ID)
	if err != nil || got.FolderID != "" || got.Description != "Shiny" {
		t.Errorf("after moving to root: %+v, %v", got, err)
	}
}
