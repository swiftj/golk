package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

func (s *server) addReadTools(ms *mcp.Server) {
	s.addListProjects(ms)
	addTool(s, ms, &mcp.Tool{
		Name:        "read_page",
		Description: "Read a page document as LK Markdown, with the version to pass to edit_page. Omit document_id for the resource's first page.",
		Annotations: readOnly("Read page"),
	}, func(in ReadPageIn) string { return in.Project }, s.readPage)
}

// ProjectInfo is one configured project.
type ProjectInfo struct {
	Name        string `json:"name"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	Default     bool   `json:"default,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ListProjectsOut is list_projects' output.
type ListProjectsOut struct {
	Projects []ProjectInfo `json:"projects"`
	ReadOnly bool          `json:"read_only"`
	// Guard is "on", "off" or "unset" (writes refused).
	Guard string `json:"guard"`
}

func (s *server) guardState() string {
	switch {
	case s.cfg.GuardOff:
		return "off"
	case s.guard == nil:
		return "unset"
	}
	return "on"
}

func (s *server) addListProjects(ms *mcp.Server) {
	mcp.AddTool(ms, &mcp.Tool{
		Name:        "list_projects",
		Description: "List the configured LegendKeeper projects (pass a name as project to other tools), and whether writes are enabled and guarded.",
		Annotations: readOnly("List projects"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ListProjectsOut, error) {
		out := ListProjectsOut{Projects: []ProjectInfo{}, ReadOnly: s.cfg.ReadOnly, Guard: s.guardState()}
		var b strings.Builder
		for _, name := range s.cfg.Projects() {
			pi := ProjectInfo{Name: name, Default: name == DefaultProject}
			c, _, err := s.projects.client(name)
			var p *lk.Project
			if err == nil {
				p, err = c.Project(ctx)
			}
			if err != nil {
				pi.Error = explain(err).Error()
				fmt.Fprintf(&b, "- %s: error: %s\n", name, pi.Error)
			} else {
				pi.ProjectID, pi.ProjectName = p.ID, p.Name
				fmt.Fprintf(&b, "- %s: %s (%s)\n", name, p.Name, p.ID)
			}
			out.Projects = append(out.Projects, pi)
		}
		if len(out.Projects) == 0 {
			b.WriteString("No project is configured: set LEGENDKEEPER_API_KEY in the server's environment.\n")
		}
		fmt.Fprintf(&b, "read-only: %t; spoiler guard: %s", out.ReadOnly, out.Guard)
		s.log.InfoContext(ctx, "tool call", "tool", "list_projects", "ok", true)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, out, nil
	})
}

// ReadPageIn is read_page's input.
type ReadPageIn struct {
	ProjectArg
	ResourceID string `json:"resource_id" jsonschema:"the resource (wiki entry) id"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"a page document of the resource; omit for its first page"`
	IDs        bool   `json:"ids,omitempty" jsonschema:"write {#id} on every block (rarely needed: edit_page keeps ids without them)"`
}

// PageInfo identifies a page and its version.
type PageInfo struct {
	ResourceID     string `json:"resource_id"`
	ResourceName   string `json:"resource_name"`
	ResourceHidden bool   `json:"resource_hidden,omitempty"`
	DocumentID     string `json:"document_id"`
	DocumentName   string `json:"document_name"`
	DocumentHidden bool   `json:"document_hidden,omitempty"`
	Version        string `json:"version"`
}

// pageDoc finds the page document a call means: docID, or the resource's
// first page document.
func pageDoc(r *lk.Resource, docID string) (*lk.Document, error) {
	for i := range r.Documents {
		d := &r.Documents[i]
		// A blank document becomes a page on its first write.
		isPage := d.Type == lk.DocumentPage || d.Type == lk.DocumentBlank
		if docID == "" && isPage || docID != "" && d.ID == docID {
			if !isPage {
				return nil, fmt.Errorf("document %s of %q is a %s document, not a page", d.ID, r.Name, d.Type)
			}
			return d, nil
		}
	}
	if docID == "" {
		return nil, fmt.Errorf("resource %q (%s) has no page document", r.Name, r.ID)
	}
	return nil, fmt.Errorf("resource %q (%s) has no document %s", r.Name, r.ID, docID)
}

func (s *server) readPage(ctx context.Context, c *lk.Client, in ReadPageIn) (toolResult[PageInfo], error) {
	var zero toolResult[PageInfo]
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := pageDoc(r, in.DocumentID)
	if err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	db, err := c.Documents.GetBlocks(ctx, ref)
	if err != nil {
		return zero, err
	}
	md, err := markdown.ToMarkdown(db.Blocks, markdown.ToOptions{IDs: in.IDs})
	if err != nil {
		return zero, err
	}
	info := PageInfo{
		ResourceID: r.ID, ResourceName: r.Name, ResourceHidden: r.IsHidden,
		DocumentID: d.ID, DocumentName: d.Name, DocumentHidden: d.IsHidden, Version: db.Version,
	}
	return toolResult[PageInfo]{out: info, text: pageHeader(info) + "\n\n" + md}, nil
}

// pageHeader is the one-line description of a page that starts a tool's
// text output.
func pageHeader(p PageInfo) string {
	var hidden []string
	if p.ResourceHidden {
		hidden = append(hidden, "resource hidden from players")
	}
	if p.DocumentHidden {
		hidden = append(hidden, "document hidden from players")
	}
	h := fmt.Sprintf("%s › %s (resource %s, document %s, version %s)", p.ResourceName, p.DocumentName, p.ResourceID, p.DocumentID, p.Version)
	if len(hidden) > 0 {
		h += " [" + strings.Join(hidden, "; ") + "]"
	}
	return h
}
