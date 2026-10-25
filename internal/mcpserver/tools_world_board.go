package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addBoardTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name: "read_board",
		Description: "Read a board document (a tldraw 3.9 canvas): its pages, shapes (id, type, position, text, parent), arrows with the shapes they connect, " +
			"assets and version. raw=true adds every record as JSON, as tldraw stores it. Omit document_id for the resource's first board.",
		Annotations: readOnly("Read board"),
	}, func(in ReadBoardIn) string { return in.Project }, s.readBoard)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "add_board_shapes",
		Description: "Add tldraw shapes, and optionally arrow bindings and assets, to a board. Each shape needs only type (geo, text, note, arrow, frame, ...); " +
			"x, y and missing props take the app's defaults. An arrow attaches to shapes with two bindings {type:\"arrow\", fromId: arrow, toId: shape, props:{terminal:\"start\"|\"end\"}}. " +
			"version (from read_board) is required. A blank document becomes a board. Every text is checked by the spoiler guard unless the board or its resource is hidden.",
		Annotations: write("Add board shapes", false, false),
	}, func(in AddBoardShapesIn) string { return in.Project }, serial(s, s.addBoardShapes))
	addTool(s, ms, &mcp.Tool{
		Name: "update_board_shapes",
		Description: "Change board shapes. Each entry names a shape by id; the fields given replace the stored ones, and props and meta merge one level deep. " +
			"version (from read_board) is required. Every text is checked by the spoiler guard unless the board or its resource is hidden.",
		Annotations: write("Update board shapes", false, true),
	}, func(in UpdateBoardShapesIn) string { return in.Project }, serial(s, s.updateBoardShapes))
	addTool(s, ms, &mcp.Tool{
		Name: "delete_board_shapes",
		Description: "Delete board shapes by id, with the shapes inside them (frames, groups) and the arrow bindings that lose an end. " +
			"version (from read_board) is required. Use dry_run to see what would go.",
		Annotations: write("Delete board shapes", true, true),
	}, func(in DeleteBoardShapesIn) string { return in.Project }, serial(s, s.deleteBoardShapes))
	addTool(s, ms, &mcp.Tool{
		Name: "replace_board",
		Description: "Replace every shape, binding and asset of a board with the ones given (pages are kept). Records read with read_board raw=true can be sent back as they are. " +
			"version (from read_board) is required. Use dry_run to see how much would be removed. Every text is checked by the spoiler guard unless the board or its resource is hidden.",
		Annotations: write("Replace board", true, true),
	}, func(in ReplaceBoardIn) string { return in.Project }, serial(s, s.replaceBoard))
}

// boardDoc finds the board document id of r, or its first board when id is
// empty. A blank document is accepted only when named and blankOK is set (a
// write makes it a board).
func boardDoc(r *lk.Resource, id string, blankOK bool) (*lk.Document, error) {
	if id == "" {
		for i := range r.Documents {
			if r.Documents[i].Type == lk.DocumentBoard {
				return &r.Documents[i], nil
			}
		}
		return nil, fmt.Errorf("resource %q (%s) has no board document. Hint: pass document_id; documents: %s", r.Name, r.ID, docList(r.Documents))
	}
	d, err := findDoc(r, id)
	if err != nil {
		return nil, err
	}
	if d.Type == lk.DocumentBoard || blankOK && d.Type == lk.DocumentBlank {
		return d, nil
	}
	return nil, fmt.Errorf("document %q (%s) of %q is a %s document, not a board", d.Name, d.ID, r.Name, d.Type)
}

// boardTarget resolves a write's resource and board document.
func boardTarget(ctx context.Context, c *lk.Client, resourceID, documentID, version string) (*lk.Resource, *lk.Document, error) {
	if version == "" {
		return nil, nil, errors.New("version is required: the board version from read_board")
	}
	r, err := c.Resources.Get(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	d, err := boardDoc(r, documentID, true)
	if err != nil {
		return nil, nil, err
	}
	return r, d, nil
}

// strictContent is v for the guard with nothing exempt. The guard exempts
// "secret" blocks in "blocks"/"children" lists and hidden properties in
// "rows"/"properties"/"property"; a board can hold such keys in shape props
// (a content-block shape), and whether the app hides them there is not
// documented, so they are renamed and checked like any other content.
func strictContent(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			out := make(map[string]any, len(t))
			for k, e := range t {
				switch k {
				case "blocks", "children", "rows", "properties", "property":
					k = "board:" + k
				}
				out[k] = walk(e)
			}
			return out
		case []any:
			for i, e := range t {
				t[i] = walk(e)
			}
			return t
		}
		return x
	}
	return walk(x), nil
}

// checkBoard runs the guard over records about to be written to board d of
// r, unless players can't see the board (hidden document or resource;
// hidden isn't inherited, D-012). Nothing on a board is hidden by itself.
func (s *server) checkBoard(r *lk.Resource, d *lk.Document, records any) error {
	if r.IsHidden || d.IsHidden {
		return nil
	}
	v, err := strictContent(map[string]any{"board": d.Name, "records": records})
	if err != nil {
		return err
	}
	return s.check(v)
}

// decodeRecords turns a tool's JSON records into lk input records, refusing
// fields the API doesn't take.
func decodeRecords[T any](field string, raw []map[string]any) ([]T, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var out []T
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%s: %w. Hint: records take only the fields read_board raw=true shows (typeName may be left out)", field, err)
	}
	return out, nil
}

// boardInputs decodes a create or replace call's records.
func boardInputs(shapes, bindings, assets []map[string]any) (lk.BoardRecords, error) {
	var r lk.BoardRecords
	var err error
	if r.Shapes, err = decodeRecords[lk.BoardShapeInput]("shapes", stripTypeName(shapes)); err != nil {
		return r, err
	}
	if r.Bindings, err = decodeRecords[lk.BoardBindingInput]("bindings", stripTypeName(bindings)); err != nil {
		return r, err
	}
	if r.Assets, err = decodeRecords[lk.BoardAssetInput]("assets", stripTypeName(assets)); err != nil {
		return r, err
	}
	return r, nil
}

// stripTypeName drops typeName, which raw records carry and inputs may
// omit (it can only be the record's own kind).
func stripTypeName(rs []map[string]any) []map[string]any {
	if rs == nil {
		return nil
	}
	out := make([]map[string]any, len(rs))
	for i, r := range rs {
		c := make(map[string]any, len(r))
		for k, v := range r {
			if k != "typeName" {
				c[k] = v
			}
		}
		out[i] = c
	}
	return out
}

// ReadBoardIn is read_board's input.
type ReadBoardIn struct {
	ProjectArg
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the board"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"the board document; omit for the resource's first board"`
	Raw        bool   `json:"raw,omitempty" jsonschema:"also return every page, shape, binding and asset record as JSON"`
	PageID     string `json:"page_id,omitempty" jsonschema:"list only the shapes on this page (page:...), in drawing order; bindings and assets are left out"`
}

// BoardOut is the board tools' structured output.
type BoardOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	Name       string `json:"name"`
	Hidden     bool   `json:"hidden,omitempty"`
	Version    string `json:"version,omitempty"`
	Shapes     int    `json:"shapes"`
	Bindings   int    `json:"bindings"`
	Assets     int    `json:"assets"`
	// ShapeIDs and BindingIDs are what delete_board_shapes deleted (or
	// would delete).
	ShapeIDs   []string `json:"shape_ids,omitempty"`
	BindingIDs []string `json:"binding_ids,omitempty"`
	DryRun     bool     `json:"dry_run,omitempty"`
}

// propString returns a shape prop that is a JSON string.
func propString(props map[string]json.RawMessage, key string) string {
	var s string
	if raw, ok := props[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func (s *server) readBoard(ctx context.Context, c *lk.Client, in ReadBoardIn) (toolResult[BoardOut], error) {
	var zero toolResult[BoardOut]
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := boardDoc(r, in.DocumentID, false)
	if err != nil {
		return zero, err
	}
	if in.PageID != "" {
		l, err := c.Boards.ListShapes(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, lk.BoardShapeListParams{PageID: in.PageID})
		if err != nil {
			return zero, err
		}
		out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, Version: l.Version, Shapes: len(l.Items)}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Shapes on page %s of board %q (%s), version %s:\n", in.PageID, d.Name, d.ID, l.Version)
		for _, sh := range l.Items {
			fmt.Fprintf(&sb, "- %s %s at (%g, %g)", sh.Type, sh.ID, sh.X, sh.Y)
			if t := propString(sh.Props, "text"); t != "" {
				fmt.Fprintf(&sb, " text %q", t)
			}
			sb.WriteString("\n")
		}
		if in.Raw {
			data, err := json.Marshal(l.Items)
			if err != nil {
				return zero, err
			}
			sb.WriteString("Records:\n")
			sb.Write(data)
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%d shape(s).", len(l.Items))
		return toolResult[BoardOut]{out: out, text: sb.String()}, nil
	}
	b, err := c.Boards.GetContent(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID})
	if err != nil {
		return zero, err
	}
	out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, Version: b.Version,
		Shapes: len(b.Shapes), Bindings: len(b.Bindings), Assets: len(b.Assets)}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Board %q (%s) of %q (%s)%s, version %s: %d shape(s), %d binding(s), %d asset(s).\n", d.Name, d.ID, r.Name, r.ID,
		listFlags(flagIf(d.IsHidden, "hidden document"), flagIf(r.IsHidden, "hidden resource")), b.Version, len(b.Shapes), len(b.Bindings), len(b.Assets))
	for _, p := range b.Pages {
		fmt.Fprintf(&sb, "Page %q (%s)\n", p.Name, p.ID)
	}
	ends := map[string][2]string{}
	for _, x := range b.Bindings {
		e := ends[x.FromID]
		switch propString(x.Props, "terminal") {
		case "start":
			e[0] = x.ToID
		case "end":
			e[1] = x.ToID
		}
		ends[x.FromID] = e
	}
	onlyPage := len(b.Pages) == 1
	for _, sh := range b.Shapes {
		fmt.Fprintf(&sb, "- %s %s at (%g, %g)", sh.Type, sh.ID, sh.X, sh.Y)
		for _, k := range []string{"text", "name"} {
			if t := propString(sh.Props, k); t != "" {
				fmt.Fprintf(&sb, " %s %q", k, t)
				break
			}
		}
		if e, ok := ends[sh.ID]; ok {
			fmt.Fprintf(&sb, " from %s to %s", orDash(e[0]), orDash(e[1]))
		}
		if !onlyPage || sh.ParentID != b.Pages[0].ID {
			fmt.Fprintf(&sb, " in %s", sh.ParentID)
		}
		sb.WriteString(listFlags(flagIf(sh.IsLocked, "locked")))
		sb.WriteString("\n")
	}
	for _, a := range b.Assets {
		fmt.Fprintf(&sb, "- asset %s %s %s\n", a.Type, a.ID, propString(a.Props, "src"))
	}
	if in.Raw {
		data, err := json.Marshal(struct {
			Pages    []lk.BoardPage    `json:"pages"`
			Shapes   []lk.BoardShape   `json:"shapes"`
			Bindings []lk.BoardBinding `json:"bindings"`
			Assets   []lk.BoardAsset   `json:"assets"`
		}{b.Pages, b.Shapes, b.Bindings, b.Assets})
		if err != nil {
			return zero, err
		}
		sb.WriteString("Records:\n")
		sb.Write(data)
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "Pass version %s to the next board write.", b.Version)
	return toolResult[BoardOut]{out: out, text: sb.String()}, nil
}

// AddBoardShapesIn is add_board_shapes's input.
type AddBoardShapesIn struct {
	ProjectArg
	DryRunArg
	ResourceID string           `json:"resource_id" jsonschema:"the resource holding the board"`
	DocumentID string           `json:"document_id,omitempty" jsonschema:"the board document (a blank one becomes a board); omit for the resource's first board"`
	Version    string           `json:"version" jsonschema:"the board version from read_board, sent as If-Match"`
	Shapes     []map[string]any `json:"shapes" jsonschema:"tldraw shapes to create: {id?, type, x?, y?, rotation?, index?, parentId?, isLocked?, opacity?, props?, meta?}"`
	Bindings   []map[string]any `json:"bindings,omitempty" jsonschema:"bindings to create: {id?, type, fromId, toId, props?, meta?}"`
	Assets     []map[string]any `json:"assets,omitempty" jsonschema:"assets to create: {id?, type (image, video or bookmark), props, meta?}"`
}

func (s *server) addBoardShapes(ctx context.Context, c *lk.Client, in AddBoardShapesIn) (toolResult[BoardOut], error) {
	var zero toolResult[BoardOut]
	recs, err := boardInputs(in.Shapes, in.Bindings, in.Assets)
	if err != nil {
		return zero, err
	}
	if len(recs.Shapes)+len(recs.Bindings)+len(recs.Assets) == 0 {
		return zero, errors.New("nothing to add: give shapes, bindings or assets")
	}
	r, d, err := boardTarget(ctx, c, in.ResourceID, in.DocumentID, in.Version)
	if err != nil {
		return zero, err
	}
	if err := s.checkBoard(r, d, recs); err != nil {
		return zero, err
	}
	out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, DryRun: in.DryRun,
		Shapes: len(recs.Shapes), Bindings: len(recs.Bindings), Assets: len(recs.Assets)}
	what := fmt.Sprintf("%d shape(s), %d binding(s) and %d asset(s) to board %q (%s) of %q", out.Shapes, out.Bindings, out.Assets, d.Name, d.ID, r.Name)
	if in.DryRun {
		return toolResult[BoardOut]{out: out, text: "Dry run: would add " + what + ". Nothing was written."}, nil
	}
	res, err := c.Boards.CreateShapes(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, recs, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version = res.Version
	var ids []string
	for _, sh := range res.Shapes {
		ids = append(ids, sh.ID)
	}
	return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Added %s. Shape ids: %s. New version %s.", what, orDash(strings.Join(ids, ", ")), res.Version)}, nil
}

// UpdateBoardShapesIn is update_board_shapes's input.
type UpdateBoardShapesIn struct {
	ProjectArg
	DryRunArg
	ResourceID string           `json:"resource_id" jsonschema:"the resource holding the board"`
	DocumentID string           `json:"document_id,omitempty" jsonschema:"the board document; omit for the resource's first board"`
	Version    string           `json:"version" jsonschema:"the board version from read_board, sent as If-Match"`
	Shapes     []map[string]any `json:"shapes" jsonschema:"partial shapes: {id, type?, x?, y?, rotation?, index?, parentId?, isLocked?, opacity?, props?, meta?}; props and meta merge one level deep"`
}

func (s *server) updateBoardShapes(ctx context.Context, c *lk.Client, in UpdateBoardShapesIn) (toolResult[BoardOut], error) {
	var zero toolResult[BoardOut]
	patches, err := decodeRecords[lk.BoardShapePatch]("shapes", stripTypeName(in.Shapes))
	if err != nil {
		return zero, err
	}
	if len(patches) == 0 {
		return zero, errors.New("nothing to change: give shapes, each with an id")
	}
	for i, p := range patches {
		if p.ID == "" {
			return zero, fmt.Errorf("shapes[%d] has no id", i)
		}
	}
	r, d, err := boardTarget(ctx, c, in.ResourceID, in.DocumentID, in.Version)
	if err != nil {
		return zero, err
	}
	if err := s.checkBoard(r, d, patches); err != nil {
		return zero, err
	}
	out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, DryRun: in.DryRun, Shapes: len(patches)}
	what := fmt.Sprintf("%d shape(s) on board %q (%s) of %q", len(patches), d.Name, d.ID, r.Name)
	if in.DryRun {
		return toolResult[BoardOut]{out: out, text: "Dry run: would update " + what + ". Nothing was written."}, nil
	}
	res, err := c.Boards.UpdateShapes(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, patches, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version = res.Version
	return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Updated %s. New version %s.", what, res.Version)}, nil
}

// DeleteBoardShapesIn is delete_board_shapes's input.
type DeleteBoardShapesIn struct {
	ProjectArg
	DryRunArg
	ResourceID string   `json:"resource_id" jsonschema:"the resource holding the board"`
	DocumentID string   `json:"document_id,omitempty" jsonschema:"the board document; omit for the resource's first board"`
	Version    string   `json:"version" jsonschema:"the board version from read_board, sent as If-Match"`
	IDs        []string `json:"ids" jsonschema:"the shape ids to delete"`
}

// staleNote warns a dry run that its version is no longer the board's.
func staleNote(sent, current string) string {
	if sent == current {
		return ""
	}
	return fmt.Sprintf(" Warning: the board is now at version %s, not %s, so the write would fail; read_board again.", current, sent)
}

func (s *server) deleteBoardShapes(ctx context.Context, c *lk.Client, in DeleteBoardShapesIn) (toolResult[BoardOut], error) {
	var zero toolResult[BoardOut]
	if len(in.IDs) == 0 {
		return zero, errors.New("ids is required: the shape ids to delete")
	}
	// No guard: deleting takes content away from players, it can't show
	// them anything.
	r, d, err := boardTarget(ctx, c, in.ResourceID, in.DocumentID, in.Version)
	if err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, DryRun: in.DryRun}
	if in.DryRun {
		b, err := c.Boards.GetContent(ctx, ref)
		if err != nil {
			return zero, err
		}
		out.ShapeIDs, out.BindingIDs = deletedBy(b, in.IDs)
		out.Shapes, out.Bindings = len(out.ShapeIDs), len(out.BindingIDs)
		return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Dry run: would delete %d shape(s) (%s) and %d binding(s) from board %q (%s) of %q.%s Nothing was written.",
			out.Shapes, orDash(strings.Join(out.ShapeIDs, ", ")), out.Bindings, d.Name, d.ID, r.Name, staleNote(in.Version, b.Version))}, nil
	}
	res, err := c.Boards.DeleteShapes(ctx, ref, in.IDs, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version, out.ShapeIDs, out.BindingIDs = res.Version, res.ShapeIDs, res.BindingIDs
	out.Shapes, out.Bindings = len(res.ShapeIDs), len(res.BindingIDs)
	return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Deleted %d shape(s) (%s) and %d binding(s) from board %q (%s) of %q. New version %s.",
		out.Shapes, orDash(strings.Join(res.ShapeIDs, ", ")), out.Bindings, d.Name, d.ID, r.Name, res.Version)}, nil
}

// deletedBy is what deleting ids from b removes: the named shapes that
// exist, every shape inside them, and the bindings with an end on any of
// those.
func deletedBy(b *lk.Board, ids []string) (shapes, bindings []string) {
	gone := map[string]bool{}
	for _, id := range ids {
		for _, sh := range b.Shapes {
			if sh.ID == id {
				gone[id] = true
			}
		}
	}
	for grew := true; grew; {
		grew = false
		for _, sh := range b.Shapes {
			if !gone[sh.ID] && gone[sh.ParentID] {
				gone[sh.ID], grew = true, true
			}
		}
	}
	for _, sh := range b.Shapes {
		if gone[sh.ID] {
			shapes = append(shapes, sh.ID)
		}
	}
	for _, x := range b.Bindings {
		if gone[x.FromID] || gone[x.ToID] {
			bindings = append(bindings, x.ID)
		}
	}
	return shapes, bindings
}

// ReplaceBoardIn is replace_board's input.
type ReplaceBoardIn struct {
	ProjectArg
	DryRunArg
	ResourceID string           `json:"resource_id" jsonschema:"the resource holding the board"`
	DocumentID string           `json:"document_id,omitempty" jsonschema:"the board document (a blank one becomes a board); omit for the resource's first board"`
	Version    string           `json:"version" jsonschema:"the board version from read_board, sent as If-Match"`
	Shapes     []map[string]any `json:"shapes" jsonschema:"every shape the board should hold ([] clears it)"`
	Bindings   []map[string]any `json:"bindings,omitempty" jsonschema:"every binding the board should hold"`
	Assets     []map[string]any `json:"assets,omitempty" jsonschema:"every asset the board should hold"`
}

func (s *server) replaceBoard(ctx context.Context, c *lk.Client, in ReplaceBoardIn) (toolResult[BoardOut], error) {
	var zero toolResult[BoardOut]
	if in.Shapes == nil {
		return zero, errors.New("shapes is required: every shape the board should hold ([] clears it)")
	}
	recs, err := boardInputs(in.Shapes, in.Bindings, in.Assets)
	if err != nil {
		return zero, err
	}
	r, d, err := boardTarget(ctx, c, in.ResourceID, in.DocumentID, in.Version)
	if err != nil {
		return zero, err
	}
	if err := s.checkBoard(r, d, recs); err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	out := BoardOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Hidden: d.IsHidden, DryRun: in.DryRun,
		Shapes: len(recs.Shapes), Bindings: len(recs.Bindings), Assets: len(recs.Assets)}
	what := fmt.Sprintf("%d shape(s), %d binding(s) and %d asset(s)", out.Shapes, out.Bindings, out.Assets)
	if in.DryRun {
		b, err := c.Boards.GetContent(ctx, ref)
		if err != nil {
			return zero, err
		}
		return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Dry run: would replace the %d shape(s), %d binding(s) and %d asset(s) of board %q (%s) of %q with %s.%s Nothing was written.",
			len(b.Shapes), len(b.Bindings), len(b.Assets), d.Name, d.ID, r.Name, what, staleNote(in.Version, b.Version))}, nil
	}
	b, err := c.Boards.ReplaceContent(ctx, ref, recs, lk.IfMatch(in.Version))
	if err != nil {
		return zero, err
	}
	out.Version, out.Shapes, out.Bindings, out.Assets = b.Version, len(b.Shapes), len(b.Bindings), len(b.Assets)
	return toolResult[BoardOut]{out: out, text: fmt.Sprintf("Replaced the content of board %q (%s) of %q: it now holds %d shape(s), %d binding(s) and %d asset(s). New version %s.",
		d.Name, d.ID, r.Name, out.Shapes, out.Bindings, out.Assets, b.Version)}, nil
}
