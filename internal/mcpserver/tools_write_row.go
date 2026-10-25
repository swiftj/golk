package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// RowTargetArg picks a key-value list of rows: a sidebar keyValue property
// (resource_id, property_id), a keyValue block on a page (document_id,
// block_id), or a keyValue property of an infobox (document_id, block_id,
// property_id).
type RowTargetArg struct {
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the rows"`
	PropertyID string `json:"property_id,omitempty" jsonschema:"a keyValue property: of the sidebar, or with document_id and block_id of an infobox"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"with block_id: the page holding a keyValue block or an infobox"`
	BlockID    string `json:"block_id,omitempty" jsonschema:"with document_id: a keyValue block, or (with property_id) an infobox block"`
}

func (a RowTargetArg) check() error {
	switch {
	case a.ResourceID == "":
		return errors.New("resource_id is required")
	case (a.DocumentID == "") != (a.BlockID == ""):
		return errors.New("give document_id and block_id together")
	case a.BlockID == "" && a.PropertyID == "":
		return errors.New("name the rows: property_id (a sidebar property), document_id+block_id (a keyValue block), or all three (an infobox property)")
	}
	return nil
}

func (a RowTargetArg) doc() lk.DocRef {
	return lk.DocRef{ResourceID: a.ResourceID, DocumentID: a.DocumentID}
}

func (a RowTargetArg) where() string {
	switch {
	case a.BlockID == "":
		return fmt.Sprintf("sidebar property %s of resource %s", a.PropertyID, a.ResourceID)
	case a.PropertyID == "":
		return fmt.Sprintf("keyValue block %s on page %s", a.BlockID, a.DocumentID)
	}
	return fmt.Sprintf("property %s of infobox %s on page %s", a.PropertyID, a.BlockID, a.DocumentID)
}

// rowTarget is a resolved RowTargetArg. Every row write carries the
// version read with the rows (If-Match).
type rowTarget struct {
	c       *lk.Client
	arg     RowTargetArg
	version string
}

func (t *rowTarget) list(ctx context.Context) ([]blocks.KVRow, error) {
	var l *lk.VersionedList[blocks.KVRow]
	var err error
	switch a := t.arg; {
	case a.BlockID == "":
		l, err = t.c.Properties.ListRows(ctx, a.ResourceID, a.PropertyID)
	case a.PropertyID == "":
		l, err = t.c.Blocks.ListRows(ctx, a.doc(), a.BlockID)
	default:
		l, err = t.c.Blocks.ListPropertyRows(ctx, a.doc(), a.BlockID, a.PropertyID)
	}
	if err != nil {
		return nil, err
	}
	t.version = l.Version
	return l.Items, nil
}

func (t *rowTarget) opts() []lk.CallOption { return []lk.CallOption{lk.IfMatch(t.version)} }

func (t *rowTarget) create(ctx context.Context, r lk.RowCreate) (*lk.Versioned[blocks.KVRow], error) {
	switch a := t.arg; {
	case a.BlockID == "":
		return t.c.Properties.CreateRow(ctx, a.ResourceID, a.PropertyID, r, t.opts()...)
	case a.PropertyID == "":
		return t.c.Blocks.CreateRow(ctx, a.doc(), a.BlockID, r, t.opts()...)
	default:
		return t.c.Blocks.CreatePropertyRow(ctx, a.doc(), a.BlockID, a.PropertyID, r, t.opts()...)
	}
}

func (t *rowTarget) update(ctx context.Context, id string, p lk.RowPatch) (*lk.Versioned[blocks.KVRow], error) {
	switch a := t.arg; {
	case a.BlockID == "":
		return t.c.Properties.UpdateRow(ctx, a.ResourceID, a.PropertyID, id, p, t.opts()...)
	case a.PropertyID == "":
		return t.c.Blocks.UpdateRow(ctx, a.doc(), a.BlockID, id, p, t.opts()...)
	default:
		return t.c.Blocks.UpdatePropertyRow(ctx, a.doc(), a.BlockID, a.PropertyID, id, p, t.opts()...)
	}
}

func (t *rowTarget) delete(ctx context.Context, id string) error {
	switch a := t.arg; {
	case a.BlockID == "":
		return t.c.Properties.DeleteRow(ctx, a.ResourceID, a.PropertyID, id, t.opts()...)
	case a.PropertyID == "":
		return t.c.Blocks.DeleteRow(ctx, a.doc(), a.BlockID, id, t.opts()...)
	default:
		return t.c.Blocks.DeletePropertyRow(ctx, a.doc(), a.BlockID, a.PropertyID, id, t.opts()...)
	}
}

// shown reports whether players can see the rows: not when the resource,
// the page or the property holding them is hidden.
func (t *rowTarget) shown(ctx context.Context) (bool, error) {
	a := t.arg
	r, err := t.c.Resources.Get(ctx, a.ResourceID)
	if err != nil {
		return false, err
	}
	if r.IsHidden {
		return false, nil
	}
	if a.DocumentID != "" {
		d, err := findDoc(r, a.DocumentID)
		if err != nil {
			return false, err
		}
		if d.IsHidden {
			return false, nil
		}
	}
	var p blocks.Property
	switch {
	case a.PropertyID == "":
		return true, nil
	case a.BlockID == "":
		if p, err = t.c.Properties.Get(ctx, a.ResourceID, a.PropertyID); err != nil {
			return false, err
		}
	default:
		v, err := t.c.Blocks.GetProperty(ctx, a.doc(), a.BlockID, a.PropertyID)
		if err != nil {
			return false, err
		}
		p = v.Value
	}
	if env := p.PropertyEnvelope(); env != nil {
		if h, _ := env.IsHidden.Get(); h {
			return false, nil
		}
	}
	return true, nil
}

// findRow finds a row by id, or else by exact key.
func findRow(rows []blocks.KVRow, id, key string) (*blocks.KVRow, error) {
	var hits []*blocks.KVRow
	for i := range rows {
		if id != "" && rows[i].ID == id || id == "" && rows[i].Key == key {
			hits = append(hits, &rows[i])
		}
	}
	switch {
	case len(hits) == 1:
		return hits[0], nil
	case len(hits) > 1:
		return nil, fmt.Errorf("%d rows have the key %q; pass row_id", len(hits), key)
	case id != "":
		return nil, fmt.Errorf("no row %s", id)
	}
	return nil, nil
}

// SetRowIn is set_row's input.
type SetRowIn struct {
	ProjectArg
	DryRunArg
	RowTargetArg
	RowID  string  `json:"row_id,omitempty" jsonschema:"the row to change; omit to match by key"`
	Key    string  `json:"key,omitempty" jsonschema:"the row's key: matches an existing row (when row_id is omitted), or names a new one"`
	NewKey *string `json:"new_key,omitempty" jsonschema:"rename the row's key"`
	Value  *string `json:"value,omitempty" jsonschema:"the row's value as inline LK Markdown (one paragraph); empty for none"`
	Text   *string `json:"text,omitempty" jsonschema:"the row's plain text, when it should differ from the value's text"`
	After  string  `json:"after,omitempty" jsonschema:"place a new row right after this row id; not with before"`
	Before string  `json:"before,omitempty" jsonschema:"place a new row right before this row id; not with after"`
}

// RowOut is set_row's and delete_row's output.
type RowOut struct {
	RowID   string `json:"row_id,omitempty"`
	Key     string `json:"key"`
	Version string `json:"version,omitempty"`
	Created bool   `json:"created,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

func (s *server) setRow(ctx context.Context, c *lk.Client, in SetRowIn) (toolResult[RowOut], error) {
	var zero toolResult[RowOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if in.RowID == "" && in.Key == "" {
		return zero, errors.New("give row_id or key")
	}
	if in.After != "" && in.Before != "" {
		return zero, errors.New("give after or before, not both")
	}
	var runs blocks.Runs
	if in.Value != nil {
		var err error
		if runs, err = inlineRuns(in.Key, *in.Value); err != nil {
			return zero, err
		}
	}
	t := &rowTarget{c: c, arg: in.RowTargetArg}
	rows, err := t.list(ctx)
	if err != nil {
		return zero, err
	}
	cur, err := findRow(rows, in.RowID, in.Key)
	if err != nil {
		return zero, err
	}
	out := RowOut{Key: in.Key, DryRun: in.DryRun}
	shownRow := map[string]any{"key": in.Key, "content": runs}
	var did string
	if cur == nil {
		if in.NewKey != nil {
			return zero, errors.New("new_key renames an existing row; name a new row with key")
		}
		out.Created, did = true, fmt.Sprintf("add row %q", in.Key)
	} else {
		if in.After != "" || in.Before != "" {
			return zero, errors.New("after and before place a new row")
		}
		out.RowID, out.Key = cur.ID, cur.Key
		shownRow["key"] = cur.Key
		if in.Value == nil {
			shownRow["content"] = cur.Content
		}
		if in.NewKey != nil {
			shownRow["key"], out.Key = *in.NewKey, *in.NewKey
		}
		did = fmt.Sprintf("update row %q (%s)", cur.Key, cur.ID)
	}
	if in.Text != nil {
		shownRow["text"] = *in.Text
	}
	shown, err := t.shown(ctx)
	if err != nil {
		return zero, err
	}
	if shown {
		if err := s.check(map[string]any{"row": shownRow}); err != nil {
			return zero, err
		}
	}
	if in.DryRun {
		return toolResult[RowOut]{out: out, text: fmt.Sprintf("Dry run: would %s in %s. Nothing was written.", did, in.where())}, nil
	}
	var res *lk.Versioned[blocks.KVRow]
	if cur == nil {
		res, err = t.create(ctx, lk.RowCreate{Key: in.Key, Content: runs, Text: optPtr(in.Text), After: optStr(in.After), Before: optStr(in.Before)})
	} else {
		p := lk.RowPatch{Key: optPtr(in.NewKey), Text: optPtr(in.Text)}
		if in.Value != nil {
			p.Content = lk.Value(runs)
		}
		res, err = t.update(ctx, cur.ID, p)
	}
	if err != nil {
		return zero, err
	}
	out.RowID, out.Key, out.Version = res.Value.ID, res.Value.Key, res.Version
	verb := "Updated"
	if cur == nil {
		verb = "Added"
	}
	return toolResult[RowOut]{out: out, text: fmt.Sprintf("%s row %q (%s) in %s; version %s.", verb, out.Key, out.RowID, in.where(), orDash(res.Version))}, nil
}

// DeleteRowIn is delete_row's input.
type DeleteRowIn struct {
	ProjectArg
	DryRunArg
	RowTargetArg
	RowID string `json:"row_id,omitempty" jsonschema:"the row to delete; or give key"`
	Key   string `json:"key,omitempty" jsonschema:"the exact key of the row to delete; or give row_id"`
}

func (s *server) deleteRow(ctx context.Context, c *lk.Client, in DeleteRowIn) (toolResult[RowOut], error) {
	var zero toolResult[RowOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	if (in.RowID == "") == (in.Key == "") {
		return zero, errors.New("give row_id or key (one of them)")
	}
	// No guard: removing a row shows players nothing new.
	t := &rowTarget{c: c, arg: in.RowTargetArg}
	rows, err := t.list(ctx)
	if err != nil {
		return zero, err
	}
	cur, err := findRow(rows, in.RowID, in.Key)
	if err != nil {
		return zero, err
	}
	if cur == nil {
		return zero, fmt.Errorf("%s has no row with key %q", in.where(), in.Key)
	}
	out := RowOut{RowID: cur.ID, Key: cur.Key, DryRun: in.DryRun}
	what := fmt.Sprintf("row %q (%s) of %s", cur.Key, cur.ID, in.where())
	if in.DryRun {
		return toolResult[RowOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := t.delete(ctx, cur.ID); err != nil {
		return zero, err
	}
	return toolResult[RowOut]{out: out, text: "Deleted " + what + "."}, nil
}
