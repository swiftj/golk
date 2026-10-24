package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addWriteTools(ms *mcp.Server) {
	addTool(s, ms, &mcp.Tool{
		Name:        "update_document",
		Description: "Rename a document (a tab of a resource), or hide it from players or show it to them. Showing a page checks its whole content with the spoiler guard first.",
		Annotations: write("Update document", true, true),
	}, func(in UpdateDocumentIn) string { return in.Project }, s.updateDocument)
}

// DryRunArg is the dry_run parameter every write tool takes.
type DryRunArg struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"check and describe the change without writing anything"`
}

// UpdateDocumentIn is update_document's input.
type UpdateDocumentIn struct {
	ProjectArg
	DryRunArg
	ResourceID string  `json:"resource_id"`
	DocumentID string  `json:"document_id"`
	Name       *string `json:"name,omitempty" jsonschema:"the new name"`
	IsHidden   *bool   `json:"is_hidden,omitempty" jsonschema:"true hides the document from players; false shows it"`
}

// DocumentOut describes a document after a write (or, with dry_run, as it
// would be).
type DocumentOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Hidden     bool   `json:"hidden"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func findDoc(r *lk.Resource, id string) (*lk.Document, error) {
	for i := range r.Documents {
		if r.Documents[i].ID == id {
			return &r.Documents[i], nil
		}
	}
	return nil, fmt.Errorf("resource %q (%s) has no document %s", r.Name, r.ID, id)
}

func (s *server) updateDocument(ctx context.Context, c *lk.Client, in UpdateDocumentIn) (toolResult[DocumentOut], error) {
	var zero toolResult[DocumentOut]
	if in.Name == nil && in.IsHidden == nil {
		return zero, errors.New("nothing to change: give name, is_hidden or both")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := findDoc(r, in.DocumentID)
	if err != nil {
		return zero, err
	}
	out := DocumentOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, Type: string(d.Type), Hidden: d.IsHidden, DryRun: in.DryRun}
	var u lk.DocumentUpdate
	var did []string
	if in.Name != nil && *in.Name != d.Name {
		u.Name, out.Name = lk.Some(*in.Name), *in.Name
		did = append(did, fmt.Sprintf("rename %q to %q", d.Name, *in.Name))
	}
	if in.IsHidden != nil && *in.IsHidden != d.IsHidden {
		u.IsHidden, out.Hidden = lk.Some(*in.IsHidden), *in.IsHidden
		did = append(did, map[bool]string{true: "hide it from players", false: "show it to players"}[*in.IsHidden])
	}
	if len(did) == 0 {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Document %q (%s) already matches; nothing to change.", d.Name, d.ID)}, nil
	}
	// The guard checks what players would see: nothing when the resource
	// or the document stays hidden; the name, and when the document is
	// being shown, its whole page.
	if !r.IsHidden && !out.Hidden {
		visible := map[string]any{"name": out.Name}
		if d.IsHidden && (d.Type == lk.DocumentPage || d.Type == lk.DocumentBlank) {
			db, err := c.Documents.GetBlocks(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID})
			if err != nil {
				return zero, err
			}
			visible["blocks"] = db.Blocks
		}
		if err := s.check(visible); err != nil {
			return zero, err
		}
	}
	summary := strings.Join(did, " and ")
	if in.DryRun {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Dry run: would %s (document %s of %q). Nothing was written.", summary, d.ID, r.Name)}, nil
	}
	nd, err := c.Documents.Update(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, u)
	if err != nil {
		return zero, err
	}
	out.Name, out.Hidden, out.Type = nd.Name, nd.IsHidden, string(nd.Type)
	return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Did %s: document %q (%s) of %q.", summary, nd.Name, nd.ID, r.Name)}, nil
}
