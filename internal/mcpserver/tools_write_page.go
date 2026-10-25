package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

// EditPageIn is edit_page's input.
type EditPageIn struct {
	ProjectArg
	DryRunArg
	ResourceID        string `json:"resource_id" jsonschema:"the resource (wiki entry) id"`
	DocumentID        string `json:"document_id,omitempty" jsonschema:"the page document; omit for the resource's first page"`
	BaseVersion       string `json:"base_version" jsonschema:"the version read_page returned for the page this edit starts from"`
	Markdown          string `json:"markdown" jsonschema:"the whole edited page as LK Markdown; blocks left out are deleted"`
	AllowRemoveOpaque bool   `json:"allow_remove_opaque,omitempty" jsonschema:"permit deleting ::unsupported or lk-raw content; only when deleting it is intended"`
}

// EditPageOut is edit_page's output.
type EditPageOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	// Version is the page's new version (with dry_run, its current one).
	Version        string `json:"version"`
	Kept           int    `json:"kept"`
	Changed        int    `json:"changed"`
	Added          int    `json:"added"`
	Removed        int    `json:"removed"`
	RemovedSecrets int    `json:"removed_secrets,omitempty"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

func (s *server) editPage(ctx context.Context, c *lk.Client, in EditPageIn) (toolResult[EditPageOut], error) {
	var zero toolResult[EditPageOut]
	if in.BaseVersion == "" {
		return zero, errors.New("base_version is required: pass the version read_page returned")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := pageDoc(r, in.DocumentID)
	if err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	page, err := markdown.Get(ctx, c, ref)
	if err != nil {
		return zero, err
	}
	if page.Version != in.BaseVersion {
		return zero, fmt.Errorf("the page %q (document %s) changed since it was read: its version is %s, not base_version %s; nothing was written. Hint: call read_page again and redo the edit on the new version",
			d.Name, d.ID, page.Version, in.BaseVersion)
	}
	ro := markdown.ReplaceOptions{DryRun: true, AllowRemoveOpaque: in.AllowRemoveOpaque}
	preview, rep, err := markdown.Replace(ctx, c, ref, page, in.Markdown, ro)
	if err != nil {
		return zero, err
	}
	// Nothing on a hidden resource or document reaches players
	// (X-GUARD-04); otherwise the whole page is checked, secrets exempt.
	if !r.IsHidden && !d.IsHidden {
		if err := s.check(map[string]any{"blocks": preview.Blocks}); err != nil {
			return zero, err
		}
	}
	out := EditPageOut{ResourceID: r.ID, DocumentID: d.ID, Version: page.Version, DryRun: in.DryRun,
		Kept: len(rep.Kept), Changed: len(rep.Changed), Added: len(rep.Added), Removed: len(rep.Removed)}
	for _, rb := range rep.Removed {
		if rb.Secret {
			out.RemovedSecrets++
		}
	}
	report := reportText(rep)
	if in.DryRun {
		return toolResult[EditPageOut]{out: out, text: fmt.Sprintf("Dry run: would write page %q (document %s of %q, version %s): %s Nothing was written.",
			d.Name, d.ID, r.Name, page.Version, report)}, nil
	}
	db, _, err := markdown.Replace(ctx, c, ref, page, in.Markdown, markdown.ReplaceOptions{AllowRemoveOpaque: in.AllowRemoveOpaque})
	if err != nil {
		return zero, err
	}
	out.Version = db.Version
	return toolResult[EditPageOut]{out: out, text: fmt.Sprintf("Wrote page %q (document %s of %q): %s New version %s.",
		d.Name, d.ID, r.Name, report, db.Version)}, nil
}

// reportText is a Report as one or two sentences: counts, then each
// removed block, with removed secrets called out first.
func reportText(rep *markdown.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d block(s) kept, %d changed, %d added, %d removed.", len(rep.Kept), len(rep.Changed), len(rep.Added), len(rep.Removed))
	var secrets, all []string
	for _, rb := range rep.Removed {
		label := string(rb.Kind)
		if rb.ID != "" {
			label += " " + rb.ID
		}
		if rb.Opaque {
			label += " (unsupported/lk-raw content)"
		}
		all = append(all, label)
		if rb.Secret {
			secrets = append(secrets, label)
		}
	}
	if len(secrets) > 0 {
		fmt.Fprintf(&b, " WARNING: this removes %d secret block(s) (DM-only content): %s.", len(secrets), strings.Join(secrets, ", "))
	}
	if len(all) > 0 {
		fmt.Fprintf(&b, " Removed: %s.", strings.Join(all, ", "))
	}
	return b.String()
}

// CreateDocumentIn is create_document's input.
type CreateDocumentIn struct {
	ProjectArg
	DryRunArg
	ResourceID  string  `json:"resource_id" jsonschema:"the resource to add the document (tab) to"`
	Name        string  `json:"name" jsonschema:"the document's name"`
	Type        string  `json:"type,omitempty" jsonschema:"page (default), map, time (a timeline), board or blank"`
	IsHidden    *bool   `json:"is_hidden,omitempty" jsonschema:"true creates it hidden from players (DM-only)"`
	Markdown    *string `json:"markdown,omitempty" jsonschema:"the page's content as LK Markdown; page documents only"`
	IsFullWidth *bool   `json:"is_full_width,omitempty" jsonschema:"true lays the document out full width"`
	CalendarID  string  `json:"calendar_id,omitempty" jsonschema:"a time document's calendar (list_calendars)"`
}

func (s *server) createDocument(ctx context.Context, c *lk.Client, in CreateDocumentIn) (toolResult[DocumentOut], error) {
	var zero toolResult[DocumentOut]
	if strings.TrimSpace(in.Name) == "" {
		return zero, errors.New("name is required")
	}
	typ := lk.DocumentPage
	switch in.Type {
	case "", "page":
	case "blank", "map", "time", "board":
		typ = lk.DocumentType(in.Type)
	default:
		return zero, fmt.Errorf("type %q is not a document type: use page, map, time, board or blank", in.Type)
	}
	if in.CalendarID != "" && typ != lk.DocumentTime {
		return zero, errors.New("calendar_id is only for a time document")
	}
	if in.Markdown != nil && typ != lk.DocumentPage {
		return zero, errors.New("markdown is only for a page document; omit it or use type page")
	}
	var content blocks.List
	if in.Markdown != nil {
		var err error
		if content, _, err = markdown.FromMarkdown(*in.Markdown, markdown.FromOptions{}); err != nil {
			return zero, err
		}
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	hidden := in.IsHidden != nil && *in.IsHidden
	// Checked before the first of the (up to) two writes; skipped when the
	// resource or the new document is hidden (X-GUARD-04).
	if !hidden && !r.IsHidden {
		visible := map[string]any{"name": in.Name}
		if content != nil {
			visible["blocks"] = content
		}
		if err := s.check(visible); err != nil {
			return zero, err
		}
	}
	dc := lk.DocumentCreate{Name: in.Name, Type: typ, IsFullWidth: optPtr(in.IsFullWidth), CalendarID: optStr(in.CalendarID)}
	if in.IsHidden != nil {
		dc.IsHidden = lk.Some(*in.IsHidden)
	}
	out := DocumentOut{ResourceID: r.ID, Name: in.Name, Type: string(typ), Hidden: hidden, DryRun: in.DryRun}
	with := ""
	if in.Markdown != nil {
		with = fmt.Sprintf(" with %d block(s)", len(content))
	}
	if in.DryRun {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Dry run: would create %s document %q%s on %q (%s), %s. Nothing was written.",
			typ, in.Name, with, r.Name, r.ID, hiddenWord(hidden))}, nil
	}
	// Documents.Create takes no content, so a page with Markdown is two
	// writes: create, then replace its (empty) blocks.
	d, err := c.Documents.Create(ctx, r.ID, dc)
	if err != nil {
		return zero, err
	}
	out.DocumentID, out.Name, out.Type, out.Hidden = d.ID, d.Name, string(d.Type), d.IsHidden
	if in.Markdown != nil {
		if _, err := c.Documents.ReplaceBlocks(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, content); err != nil {
			return zero, fmt.Errorf("created document %q (%s), but writing its content failed: %w. Hint: write it with read_page and edit_page",
				d.Name, d.ID, explain(err))
		}
	}
	return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Created %s document %q (%s)%s on %q (%s), %s.",
		d.Type, d.Name, d.ID, with, r.Name, r.ID, hiddenWord(d.IsHidden))}, nil
}
