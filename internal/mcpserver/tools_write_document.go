package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/swiftj/golk/lk"
)

// DocumentArg names a document of a resource.
type DocumentArg struct {
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the document"`
	DocumentID string `json:"document_id" jsonschema:"the document (tab); get_resource lists them"`
}

func (a DocumentArg) check() error {
	if a.ResourceID == "" || a.DocumentID == "" {
		return errors.New("resource_id and document_id are required")
	}
	return nil
}

func (a DocumentArg) ref() lk.DocRef {
	return lk.DocRef{ResourceID: a.ResourceID, DocumentID: a.DocumentID}
}

// DeleteDocumentIn is delete_document's input.
type DeleteDocumentIn struct {
	ProjectArg
	DryRunArg
	DocumentArg
}

func (s *server) deleteDocument(ctx context.Context, c *lk.Client, in DeleteDocumentIn) (toolResult[DocumentOut], error) {
	var zero toolResult[DocumentOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	// No guard: deleting takes content away from players.
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := findDoc(r, in.DocumentID)
	if err != nil {
		return zero, err
	}
	out := DocumentOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Type: string(d.Type), Hidden: d.IsHidden, DryRun: in.DryRun}
	what := fmt.Sprintf("%s document %q (%s) of %q (%s)", d.Type, d.Name, d.ID, r.Name, r.ID)
	if in.DryRun {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s with its content. It would wait in the project's trash, and restore_document brings it back. Nothing was written.", what)}, nil
	}
	if err := c.Documents.Delete(ctx, in.ref()); err != nil {
		return zero, err
	}
	return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Deleted %s. It is in the project's trash; restore_document brings it back.", what)}, nil
}

// RestoreDocumentIn is restore_document's input.
type RestoreDocumentIn struct {
	ProjectArg
	DryRunArg
	DocumentArg
}

// restoreDocument brings a deleted document back. A deleted document can't
// be read, so with the guard on a restored visible document (of a visible
// resource) is hidden at once, checked, and shown again only if it passes,
// as restore_resource does (X-GUARD-14).
func (s *server) restoreDocument(ctx context.Context, c *lk.Client, in RestoreDocumentIn) (toolResult[DocumentOut], error) {
	var zero toolResult[DocumentOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	out := DocumentOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Dry run: would restore document %s of resource %s in its old place, with its content. "+
			"A deleted document can't be read, so with the spoiler guard on it is hidden at once after the restore, checked, and shown again only if it passes. Nothing was written.",
			in.DocumentID, in.ResourceID)}, nil
	}
	d, err := c.Documents.Restore(ctx, in.ref())
	if err != nil {
		return zero, err
	}
	out.Name, out.Type, out.Hidden = d.Name, string(d.Type), d.IsHidden
	done := fmt.Sprintf("Restored %s document %q (%s)", d.Type, d.Name, d.ID)
	if s.cfg.GuardOff || d.IsHidden {
		return toolResult[DocumentOut]{out: out, text: done + ", " + hiddenWord(d.IsHidden) + "."}, nil
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, fmt.Errorf("%s, but reading its resource for the spoiler guard check failed, so it is visible and unchecked: %w. Hint: hide it with update_document is_hidden=true", done, explain(err))
	}
	if r.IsHidden {
		return toolResult[DocumentOut]{out: out, text: done + "; its resource is hidden from players."}, nil
	}
	if _, err := c.Documents.Update(ctx, in.ref(), lk.DocumentUpdate{IsHidden: lk.Some(true)}); err != nil {
		return zero, fmt.Errorf("%s, but hiding it for the spoiler guard check FAILED, so it may be visible to players now: %w. Hint: hide it with update_document is_hidden=true at once", done, explain(err))
	}
	d.IsHidden = false
	content, err := documentContent(ctx, c, r, d)
	if err == nil {
		err = s.check(content)
	}
	if err != nil {
		out.Hidden = true
		return zero, fmt.Errorf("%s and left it hidden from players: %w", done, err)
	}
	if _, err := c.Documents.Update(ctx, in.ref(), lk.DocumentUpdate{IsHidden: lk.Some(false)}); err != nil {
		return zero, fmt.Errorf("%s and checked it, but showing it again failed, so it is hidden: %w. Hint: show it with update_document is_hidden=false", done, explain(err))
	}
	return toolResult[DocumentOut]{out: out, text: done + ", visible to players (checked by the spoiler guard)."}, nil
}

// AssignBlockIDsIn is assign_block_ids' input.
type AssignBlockIDsIn struct {
	ProjectArg
	DryRunArg
	DocumentArg
}

// AssignBlockIDsOut is assign_block_ids' output.
type AssignBlockIDsOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	Assigned   int    `json:"assigned"`
	Version    string `json:"version,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func (s *server) assignBlockIDs(ctx context.Context, c *lk.Client, in AssignBlockIDsIn) (toolResult[AssignBlockIDsOut], error) {
	var zero toolResult[AssignBlockIDsOut]
	if err := in.check(); err != nil {
		return zero, err
	}
	// No guard: ids are not content players see.
	out := AssignBlockIDsOut{ResourceID: in.ResourceID, DocumentID: in.DocumentID, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[AssignBlockIDsOut]{out: out, text: fmt.Sprintf("Dry run: would give an id to every block of page %s that lacks one. Content doesn't change. Nothing was written.", in.DocumentID)}, nil
	}
	res, err := c.Documents.AssignBlockIDs(ctx, in.ref())
	if err != nil {
		return zero, err
	}
	out.Assigned, out.Version = res.Assigned, res.Version
	return toolResult[AssignBlockIDsOut]{out: out, text: fmt.Sprintf("Gave ids to %d block(s) of page %s; version %s.", res.Assigned, in.DocumentID, orDash(res.Version))}, nil
}
