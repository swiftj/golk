package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addWriteTools(ms *mcp.Server) {
	addTool(s, ms, &mcp.Tool{
		Name:        "update_document",
		Description: "Rename a document (a tab of a resource), or hide it from players or show it to them. Showing a page checks its whole content with the spoiler guard first. " +
			"For a timeline document, calendar_id sets its calendar and sources sets the timelines it chains ([] clears them); the chained lanes and events are checked by the spoiler guard first.",
		Annotations: write("Update document", true, true),
	}, func(in UpdateDocumentIn) string { return in.Project }, serial(s, s.updateDocument))
	addTool(s, ms, &mcp.Tool{
		Name:        "create_resource",
		Description: "Create a resource (wiki entry), optionally with its first page as LK Markdown. Its name, aliases, tags and page are checked by the spoiler guard unless it is created hidden.",
		Annotations: write("Create resource", false, false),
	}, func(in CreateResourceIn) string { return in.Project }, serial(s, s.createResource))
	addTool(s, ms, &mcp.Tool{
		Name:        "update_resource",
		Description: "Rename a resource, set its tags or aliases, hide or show it, or move it (parent_id or move_to_root). Only the fields given change. Showing a hidden resource checks everything it would show with the spoiler guard first.",
		Annotations: write("Update resource", true, true),
	}, func(in UpdateResourceIn) string { return in.Project }, serial(s, s.updateResource))
	addTool(s, ms, &mcp.Tool{
		Name:        "edit_page",
		Description: "Replace a page with edited LK Markdown. Call read_page first and pass its version as base_version and the whole edited page as markdown; blocks left out are deleted. Use dry_run to see what would be kept, changed, added and removed.",
		Annotations: write("Edit page", true, false),
	}, func(in EditPageIn) string { return in.Project }, serial(s, s.editPage))
	addTool(s, ms, &mcp.Tool{
		Name:        "set_property",
		Description: "Create or update a resource's sidebar property by exact title: a text field from markdown, or key-value rows from rows (replacing all rows). Can also hide or show it.",
		Annotations: write("Set property", true, true),
	}, func(in SetPropertyIn) string { return in.Project }, serial(s, s.setProperty))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_property",
		Description: "Delete a resource's sidebar property by exact title or by property_id.",
		Annotations: write("Delete property", true, false),
	}, func(in DeletePropertyIn) string { return in.Project }, serial(s, s.deleteProperty))
	addTool(s, ms, &mcp.Tool{
		Name:        "create_document",
		Description: "Add a page or blank document (a tab) to a resource, optionally with the page's content as LK Markdown.",
		Annotations: write("Create document", false, false),
	}, func(in CreateDocumentIn) string { return in.Project }, serial(s, s.createDocument))
	addTool(s, ms, &mcp.Tool{
		Name:        "upload_image",
		Description: "Upload a local JPEG, PNG, GIF or WebP file to the project's asset library and return its URL, for image blocks and properties.",
		Annotations: write("Upload image", false, false),
	}, func(in UploadImageIn) string { return in.Project }, serial(s, s.uploadImage))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_resource",
		Description: "Delete a resource with its documents and properties. A resource with children needs cascade, which deletes them too. Use dry_run to see what would go; restore_resource undoes it.",
		Annotations: write("Delete resource", true, false),
	}, func(in DeleteResourceIn) string { return in.Project }, serial(s, s.deleteResource))
	addTool(s, ms, &mcp.Tool{
		Name:        "restore_resource",
		Description: "Restore a deleted resource and the descendants deleted with it. Its content is checked by the spoiler guard right after; anything flagged is hidden from players at once.",
		Annotations: write("Restore resource", false, true),
	}, func(in RestoreResourceIn) string { return in.Project }, serial(s, s.restoreResource))
}

// serial runs h holding the server's write lock. MCP tool calls run
// concurrently, and a write tool's guard decision rests on what it read
// (a target's hidden flag, the content an unhide shows): without the lock
// an edit_page on a hidden resource could be checked as hidden, an
// update_resource could unhide it after scanning the old content, and the
// edit would land on a visible page unchecked. Holding one lock from a
// write tool's first read to its last write makes each check-then-write
// atomic with respect to every other write tool of this server.
func serial[In, Out any](s *server, h func(context.Context, *lk.Client, In) (toolResult[Out], error)) func(context.Context, *lk.Client, In) (toolResult[Out], error) {
	return func(ctx context.Context, c *lk.Client, in In) (toolResult[Out], error) {
		// Without a guard every write tool refuses (D-011), also those
		// that can't show anything (delete, hide, move).
		if err := s.guardReady(); err != nil {
			var zero toolResult[Out]
			return zero, err
		}
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		return h(ctx, c, in)
	}
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
	CalendarID *string `json:"calendar_id,omitempty" jsonschema:"a timeline document's calendar (list_calendars); its dates and names show on the timeline"`
	// Sources is a pointer so that [] (clear the chain) differs from
	// omitted.
	Sources *[]SourceOut `json:"sources,omitempty" jsonschema:"the timelines a timeline document chains, in order, replacing the list; [] clears it. Their lanes and events are checked by the spoiler guard unless this timeline is hidden"`
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
	if in.Name == nil && in.IsHidden == nil && in.CalendarID == nil && in.Sources == nil {
		return zero, errors.New("nothing to change: give name, is_hidden, calendar_id or sources")
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
	// Calendar and chain belong to timeline documents. Resources.Get
	// leaves out a document's sources, so read the document itself.
	var calendar *lk.Calendar
	var newSources []lk.DocRef
	if in.CalendarID != nil || in.Sources != nil {
		if d.Type != lk.DocumentTime {
			return zero, fmt.Errorf("calendar_id and sources need a timeline (time) document; %q (%s) is a %s document", d.Name, d.ID, d.Type)
		}
		full, err := c.Documents.Get(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID})
		if err != nil {
			return zero, err
		}
		if in.CalendarID != nil && *in.CalendarID != full.CalendarID {
			if *in.CalendarID == "" {
				return zero, errors.New("calendar_id is empty: give a calendar id from list_calendars")
			}
			// The calendar must exist; a custom one's spec is checked
			// below, as the timeline shows its names.
			for cal, err := range c.Calendars.All(ctx) {
				if err != nil {
					return zero, err
				}
				if cal.ID == *in.CalendarID {
					calendar = &cal
					break
				}
			}
			if calendar == nil {
				return zero, fmt.Errorf("no calendar %q. Hint: list_calendars lists the ids", *in.CalendarID)
			}
			u.CalendarID = lk.Some(*in.CalendarID)
			did = append(did, fmt.Sprintf("set its calendar to %q (%s)", calendar.Spec.Name, calendar.ID))
		}
		if in.Sources != nil {
			newSources = make([]lk.DocRef, 0, len(*in.Sources))
			for _, src := range *in.Sources {
				if src.ResourceID == "" || src.DocumentID == "" {
					return zero, errors.New("each source needs resource_id and document_id")
				}
				newSources = append(newSources, lk.DocRef{ResourceID: src.ResourceID, DocumentID: src.DocumentID})
			}
			if !slices.Equal(newSources, full.Sources) {
				u.Sources = lk.Some(newSources)
				if len(newSources) == 0 {
					did = append(did, "clear its chained timelines")
				} else {
					did = append(did, fmt.Sprintf("chain %d timeline(s)", len(newSources)))
				}
			} else {
				newSources = nil
			}
		}
	}
	if len(did) == 0 {
		return toolResult[DocumentOut]{out: out, text: fmt.Sprintf("Document %q (%s) already matches; nothing to change.", d.Name, d.ID)}, nil
	}
	// The guard checks what players would see: nothing when the resource
	// or the document stays hidden; the name, and when the document is
	// being shown, its whole page; a newly set custom calendar's spec; and
	// the lanes and events of newly chained timelines (X-GUARD-08).
	if !r.IsHidden && !out.Hidden {
		visible := map[string]any{"name": out.Name}
		if d.IsHidden && !s.cfg.GuardOff {
			content, err := documentContent(ctx, c, r, d)
			if err != nil {
				return zero, err
			}
			content["name"] = out.Name
			visible = content
		}
		if calendar != nil && !lk.IsBuiltinCalendar(calendar.ID) {
			visible["calendar"] = calendar.Spec
		}
		if len(newSources) > 0 && !s.cfg.GuardOff {
			chain, err := chainContent(ctx, c, newSources)
			if err != nil {
				return zero, err
			}
			visible["sources"] = chain
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
