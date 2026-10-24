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
	addTool(s, ms, &mcp.Tool{
		Name:        "create_resource",
		Description: "Create a resource (wiki entry), optionally with its first page as LK Markdown. Its name, aliases, tags and page are checked by the spoiler guard unless it is created hidden.",
		Annotations: write("Create resource", false, false),
	}, func(in CreateResourceIn) string { return in.Project }, s.createResource)
	addTool(s, ms, &mcp.Tool{
		Name:        "update_resource",
		Description: "Rename a resource, set its tags or aliases, hide or show it, or move it (parent_id or move_to_root). Only the fields given change. Showing a hidden resource checks everything it would show with the spoiler guard first.",
		Annotations: write("Update resource", true, true),
	}, func(in UpdateResourceIn) string { return in.Project }, s.updateResource)
	addTool(s, ms, &mcp.Tool{
		Name:        "edit_page",
		Description: "Replace a page with edited LK Markdown. Call read_page first and pass its version as base_version and the whole edited page as markdown; blocks left out are deleted. Use dry_run to see what would be kept, changed, added and removed.",
		Annotations: write("Edit page", true, false),
	}, func(in EditPageIn) string { return in.Project }, s.editPage)
	addTool(s, ms, &mcp.Tool{
		Name:        "set_property",
		Description: "Create or update a resource's sidebar property by exact title: a text field from markdown, or key-value rows from rows (replacing all rows). Can also hide or show it.",
		Annotations: write("Set property", true, true),
	}, func(in SetPropertyIn) string { return in.Project }, s.setProperty)
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_property",
		Description: "Delete a resource's sidebar property by exact title or by property_id.",
		Annotations: write("Delete property", true, false),
	}, func(in DeletePropertyIn) string { return in.Project }, s.deleteProperty)
	addTool(s, ms, &mcp.Tool{
		Name:        "create_document",
		Description: "Add a page or blank document (a tab) to a resource, optionally with the page's content as LK Markdown.",
		Annotations: write("Create document", false, false),
	}, func(in CreateDocumentIn) string { return in.Project }, s.createDocument)
	addTool(s, ms, &mcp.Tool{
		Name:        "upload_image",
		Description: "Upload a local JPEG, PNG, GIF or WebP file to the project's asset library and return its URL, for image blocks and properties.",
		Annotations: write("Upload image", false, false),
	}, func(in UploadImageIn) string { return in.Project }, s.uploadImage)
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_resource",
		Description: "Delete a resource with its documents and properties. A resource with children needs cascade, which deletes them too. Use dry_run to see what would go; restore_resource undoes it.",
		Annotations: write("Delete resource", true, false),
	}, func(in DeleteResourceIn) string { return in.Project }, s.deleteResource)
	addTool(s, ms, &mcp.Tool{
		Name:        "restore_resource",
		Description: "Restore a deleted resource and the descendants deleted with it. Its content is checked by the spoiler guard right after; anything flagged is hidden from players at once.",
		Annotations: write("Restore resource", false, true),
	}, func(in RestoreResourceIn) string { return in.Project }, s.restoreResource)
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
