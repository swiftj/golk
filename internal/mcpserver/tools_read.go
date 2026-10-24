package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/markdown"
)

func (s *server) addReadTools(ms *mcp.Server) {
	s.addListProjects(ms)
	addTool(s, ms, &mcp.Tool{
		Name:        "read_page",
		Description: "Read a page document as LK Markdown, with the version to pass to edit_page. Omit document_id for the resource's first page.",
		Annotations: readOnly("Read page"),
	}, func(in ReadPageIn) string { return in.Project }, s.readPage)
	pages := newPageCache()
	addTool(s, ms, &mcp.Tool{
		Name: "search",
		Description: "Search the wiki by resource name, alias and page content. Returns the best matches first, one line per hit: " +
			"the resource and, for a content match, the document that matched. Use read_page on a hit to read it.",
		Annotations: readOnly("Search"),
	}, func(in SearchIn) string { return in.Project }, s.search)
	addTool(s, ms, &mcp.Tool{
		Name: "list_resources",
		Description: "List resources (wiki entries) one page at a time: id, name, tags, hidden flag and documents. " +
			"Pass parent_id for one resource's children, or root for the top level; neither lists the whole project. " +
			"When next_cursor is returned, call again with it (and the same filters) for the next page.",
		Annotations: readOnly("List resources"),
	}, func(in ListResourcesIn) string { return in.Project }, func(ctx context.Context, c *lk.Client, in ListResourcesIn) (toolResult[ListResourcesOut], error) {
		return s.listResources(ctx, c, pages, in)
	})
	addTool(s, ms, &mcp.Tool{
		Name: "get_resource",
		Description: "Get one resource's metadata (name, parent, tags, aliases, hidden and locked flags), its documents, " +
			"and its sidebar properties as LK Markdown. Properties marked isHidden are DM-only.",
		Annotations: readOnly("Get resource"),
	}, func(in GetResourceIn) string { return in.Project }, s.getResource)
	addTool(s, ms, &mcp.Tool{
		Name:        "list_tags",
		Description: "List the project's tags with the number of resources that carry each, most used first. Needs an admin key.",
		Annotations: readOnly("List tags"),
	}, func(in ListTagsIn) string { return in.Project }, s.listTags)
	addTool(s, ms, &mcp.Tool{
		Name: "list_assets",
		Description: "List the asset library one page at a time: images, map images, audio and folders, with id, name, type, URL, " +
			"pixel size and hidden flag. Omit folder_id for the root; pass a folder's id to list inside it. " +
			"When next_cursor is returned, call again with it (and the same filters) for the next page.",
		Annotations: readOnly("List assets"),
	}, func(in ListAssetsIn) string { return in.Project }, func(ctx context.Context, c *lk.Client, in ListAssetsIn) (toolResult[ListAssetsOut], error) {
		return s.listAssets(ctx, c, pages, in)
	})
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
		start := time.Now()
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
		s.log.InfoContext(ctx, "tool call", "tool", "list_projects", "project", "all", "duration", time.Since(start), "ok", true)
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

// maxQuery is the longest search query the API takes (lk.SearchParams).
const maxQuery = 500

// SearchIn is search's input.
type SearchIn struct {
	ProjectArg
	Query string `json:"query" jsonschema:"text to find in resource names, aliases and page content (1-500 characters)"`
	Limit int    `json:"limit,omitempty" jsonschema:"number of hits, 1-100; omit for the server default (32)"`
}

// SearchHitOut is one search hit. DocumentID is empty when the resource
// matched by name or alias.
type SearchHitOut struct {
	ResourceID   string `json:"resource_id"`
	ResourceName string `json:"resource_name"`
	DocumentID   string `json:"document_id,omitempty"`
	DocumentName string `json:"document_name,omitempty"`
}

// SearchOut is search's output.
type SearchOut struct {
	Hits []SearchHitOut `json:"hits"`
}

func (s *server) search(ctx context.Context, c *lk.Client, in SearchIn) (toolResult[SearchOut], error) {
	var zero toolResult[SearchOut]
	if strings.TrimSpace(in.Query) == "" {
		return zero, fmt.Errorf("query is empty. Hint: pass the text to search for")
	}
	if n := utf8.RuneCountInString(in.Query); n > maxQuery {
		return zero, fmt.Errorf("query is %d characters; the limit is %d. Hint: search for a shorter phrase", n, maxQuery)
	}
	if err := checkPageLimit(in.Limit); err != nil {
		return zero, err
	}
	p, err := c.Search.Search(ctx, lk.SearchParams{Query: in.Query, Limit: in.Limit})
	if err != nil {
		return zero, err
	}
	out := SearchOut{Hits: []SearchHitOut{}}
	var b strings.Builder
	for _, h := range p.Items {
		hit := SearchHitOut{ResourceID: h.Resource.ID, ResourceName: h.Resource.Name}
		fmt.Fprintf(&b, "- %s (%s)", h.Resource.Name, h.Resource.ID)
		if d := h.Document; d != nil {
			hit.DocumentID, hit.DocumentName = d.ID, d.Name
			fmt.Fprintf(&b, " › %s (%s)", d.Name, d.ID)
		}
		b.WriteString("\n")
		out.Hits = append(out.Hits, hit)
	}
	if len(out.Hits) == 0 {
		fmt.Fprintf(&b, "No results for %q.", in.Query)
	} else {
		fmt.Fprintf(&b, "%d result(s).", len(out.Hits))
	}
	return toolResult[SearchOut]{out: out, text: b.String()}, nil
}

// ListResourcesIn is list_resources' input.
type ListResourcesIn struct {
	ProjectArg
	ParentID string `json:"parent_id,omitempty" jsonschema:"list this resource's direct children"`
	Root     bool   `json:"root,omitempty" jsonschema:"list only top-level resources (not with parent_id)"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call, with the same filters"`
	Limit    int    `json:"limit,omitempty" jsonschema:"page size, 1-100; omit for the server default (50)"`
}

// DocSummaryOut is a document of a resource.
type DocSummaryOut struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Hidden bool   `json:"hidden,omitempty"`
}

// ResourceItemOut is one resource of list_resources.
type ResourceItemOut struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	ParentID    string          `json:"parent_id,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Hidden      bool            `json:"hidden,omitempty"`
	HasChildren bool            `json:"has_children,omitempty"`
	Documents   []DocSummaryOut `json:"documents"`
}

// ListResourcesOut is list_resources' output.
type ListResourcesOut struct {
	Resources  []ResourceItemOut `json:"resources"`
	HasMore    bool              `json:"has_more,omitempty"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (s *server) listResources(ctx context.Context, c *lk.Client, pc *pageCache, in ListResourcesIn) (toolResult[ListResourcesOut], error) {
	var zero toolResult[ListResourcesOut]
	if in.Root && in.ParentID != "" {
		return zero, fmt.Errorf("pass parent_id or root, not both")
	}
	if err := checkPageLimit(in.Limit); err != nil {
		return zero, err
	}
	params := lk.ResourceListParams{ParentID: in.ParentID, Limit: in.Limit}
	if in.Root {
		params.ParentID = lk.ParentRoot
	}
	scope := fmt.Sprintf("list_resources\x00%s\x00%s\x00%d", in.Project, params.ParentID, params.Limit)
	p, err := fetchPage(ctx, pc, "list_resources", scope, in.Cursor, func(ctx context.Context) (*lk.Page[lk.ResourceListItem], error) {
		return c.Resources.List(ctx, params)
	})
	if err != nil {
		return zero, err
	}
	out := ListResourcesOut{Resources: []ResourceItemOut{}, HasMore: p.HasMore, NextCursor: p.NextCursor}
	var b strings.Builder
	for _, r := range p.Items {
		it := ResourceItemOut{ID: r.ID, Name: r.Name, ParentID: r.ParentID, Tags: r.Tags, Hidden: r.IsHidden,
			HasChildren: r.HasChildren, Documents: []DocSummaryOut{}}
		fmt.Fprintf(&b, "- %s (%s)%s", r.Name, r.ID, listFlags(flagIf(r.IsHidden, "hidden"), flagIf(r.HasChildren, "has children")))
		if len(r.Tags) > 0 {
			fmt.Fprintf(&b, " tags: %s;", strings.Join(r.Tags, ", "))
		}
		var docs []string
		for _, d := range r.Documents {
			it.Documents = append(it.Documents, DocSummaryOut{ID: d.ID, Name: d.Name, Type: string(d.Type)})
			docs = append(docs, fmt.Sprintf("%s (%s, %s)", d.Name, d.Type, d.ID))
		}
		if len(docs) > 0 {
			fmt.Fprintf(&b, " docs: %s", strings.Join(docs, ", "))
		}
		b.WriteString("\n")
		out.Resources = append(out.Resources, it)
	}
	pagedFooter(&b, len(out.Resources), "resource(s)", p.NextCursor, p.HasMore)
	return toolResult[ListResourcesOut]{out: out, text: b.String()}, nil
}

// GetResourceIn is get_resource's input.
type GetResourceIn struct {
	ProjectArg
	ResourceID string `json:"resource_id" jsonschema:"the resource (wiki entry) id"`
}

// PropertySummaryOut identifies a sidebar property; its content is only in the
// text output.
type PropertySummaryOut struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Title  string `json:"title,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// ResourceAncestorOut is an ancestor of a resource.
type ResourceAncestorOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetResourceOut is get_resource's output.
type GetResourceOut struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	ParentID   string                `json:"parent_id,omitempty"`
	Ancestors  []ResourceAncestorOut `json:"ancestors,omitempty"`
	Tags       []string              `json:"tags"`
	Aliases    []string              `json:"aliases"`
	Hidden     bool                  `json:"hidden,omitempty"`
	Locked     bool                  `json:"locked,omitempty"`
	Documents  []DocSummaryOut       `json:"documents"`
	Properties []PropertySummaryOut  `json:"properties"`
}

func (s *server) getResource(ctx context.Context, c *lk.Client, in GetResourceIn) (toolResult[GetResourceOut], error) {
	var zero toolResult[GetResourceOut]
	if in.ResourceID == "" {
		return zero, fmt.Errorf("resource_id is empty. Hint: find it with search or list_resources")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	out := GetResourceOut{ID: r.ID, Name: r.Name, ParentID: r.ParentID, Tags: stringsOrEmpty(r.Tags), Aliases: stringsOrEmpty(r.Aliases),
		Hidden: r.IsHidden, Locked: r.IsLocked, Documents: []DocSummaryOut{}, Properties: []PropertySummaryOut{}}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (resource %s)%s\n", r.Name, r.ID, listFlags(flagIf(r.IsHidden, "hidden from players"), flagIf(r.IsLocked, "locked")))
	if len(r.Ancestors) > 0 {
		var path []string
		for _, a := range r.Ancestors {
			out.Ancestors = append(out.Ancestors, ResourceAncestorOut{ID: a.ID, Name: a.Name})
			path = append(path, fmt.Sprintf("%s (%s)", a.Name, a.ID))
		}
		fmt.Fprintf(&b, "Parent path: %s\n", strings.Join(path, " › "))
	} else if r.ParentID != "" {
		fmt.Fprintf(&b, "Parent: %s\n", r.ParentID)
	} else {
		b.WriteString("Parent: none (top level)\n")
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(r.Tags, ", "))
	}
	if len(r.Aliases) > 0 {
		fmt.Fprintf(&b, "Aliases: %s\n", strings.Join(r.Aliases, ", "))
	}
	b.WriteString("Documents:\n")
	for _, d := range r.Documents {
		out.Documents = append(out.Documents, DocSummaryOut{ID: d.ID, Name: d.Name, Type: string(d.Type), Hidden: d.IsHidden})
		fmt.Fprintf(&b, "- %s (%s, %s)%s\n", d.Name, d.Type, d.ID, listFlags(flagIf(d.IsHidden, "hidden from players")))
	}
	if len(r.Documents) == 0 {
		b.WriteString("- none\n")
	}
	var hidden []string
	for _, p := range r.Properties {
		po := propertySummary(p)
		out.Properties = append(out.Properties, po)
		if po.Hidden {
			hidden = append(hidden, fmt.Sprintf("%s (%s)", orDash(po.Title), po.ID))
		}
	}
	if len(r.Properties) == 0 {
		b.WriteString("Sidebar properties: none.")
		return toolResult[GetResourceOut]{out: out, text: b.String()}, nil
	}
	md, err := markdown.ToMarkdown(blocks.List{&blocks.InfoboxBlock{Rows: r.Properties}}, markdown.ToOptions{})
	if err != nil {
		return zero, err
	}
	fmt.Fprintf(&b, "Sidebar properties (%d), as LK Markdown; isHidden marks DM-only content:\n", len(r.Properties))
	if len(hidden) > 0 {
		fmt.Fprintf(&b, "Hidden from players: %s\n", strings.Join(hidden, ", "))
	} else {
		b.WriteString("Hidden from players: none (every property is player-visible)\n")
	}
	b.WriteString("\n" + md)
	return toolResult[GetResourceOut]{out: out, text: b.String()}, nil
}

// propertyOut describes a property without its content.
func propertySummary(p blocks.Property) PropertySummaryOut {
	po := PropertySummaryOut{ID: p.NodeID(), Kind: string(p.Kind())}
	if e := p.PropertyEnvelope(); e != nil {
		po.Title = e.Title.Or("")
		po.Hidden = e.IsHidden.Or(false)
		return po
	}
	if rp, ok := p.(*blocks.RawProperty); ok {
		var v struct {
			Title    string `json:"title"`
			IsHidden bool   `json:"isHidden"`
		}
		_ = json.Unmarshal(rp.JSON, &v)
		po.Title, po.Hidden = v.Title, v.IsHidden
	}
	return po
}

func stringsOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListTagsIn is list_tags' input.
type ListTagsIn struct {
	ProjectArg
}

// TagCountOut is a tag and how many resources carry it.
type TagCountOut struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ListTagsOut is list_tags' output.
type ListTagsOut struct {
	Tags []TagCountOut `json:"tags"`
}

func (s *server) listTags(ctx context.Context, c *lk.Client, _ ListTagsIn) (toolResult[ListTagsOut], error) {
	var zero toolResult[ListTagsOut]
	p, err := c.Tags.List(ctx)
	if err != nil {
		return zero, err
	}
	out := ListTagsOut{Tags: []TagCountOut{}}
	var b strings.Builder
	for _, t := range p.Items {
		out.Tags = append(out.Tags, TagCountOut{Tag: t.Tag, Count: t.Count})
		fmt.Fprintf(&b, "- %s (%d)\n", t.Tag, t.Count)
	}
	fmt.Fprintf(&b, "%d tag(s).", len(out.Tags))
	return toolResult[ListTagsOut]{out: out, text: b.String()}, nil
}

// ListAssetsIn is list_assets' input.
type ListAssetsIn struct {
	ProjectArg
	FolderID string `json:"folder_id,omitempty" jsonschema:"list this folder's direct contents; omit for the root"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call, with the same folder_id and limit"`
	Limit    int    `json:"limit,omitempty" jsonschema:"page size, 1-100; omit for the server default (50)"`
}

// AssetItemOut is one asset.
type AssetItemOut struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	FolderID string `json:"folder_id,omitempty"`
	URL      string `json:"url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

// ListAssetsOut is list_assets' output.
type ListAssetsOut struct {
	Assets     []AssetItemOut `json:"assets"`
	HasMore    bool           `json:"has_more,omitempty"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func (s *server) listAssets(ctx context.Context, c *lk.Client, pc *pageCache, in ListAssetsIn) (toolResult[ListAssetsOut], error) {
	var zero toolResult[ListAssetsOut]
	if err := checkPageLimit(in.Limit); err != nil {
		return zero, err
	}
	params := lk.AssetListParams{FolderID: in.FolderID, Limit: in.Limit}
	scope := fmt.Sprintf("list_assets\x00%s\x00%s\x00%d", in.Project, params.FolderID, params.Limit)
	p, err := fetchPage(ctx, pc, "list_assets", scope, in.Cursor, func(ctx context.Context) (*lk.Page[lk.Asset], error) {
		return c.Assets.List(ctx, params)
	})
	if err != nil {
		return zero, err
	}
	out := ListAssetsOut{Assets: []AssetItemOut{}, HasMore: p.HasMore, NextCursor: p.NextCursor}
	var b strings.Builder
	for _, a := range p.Items {
		out.Assets = append(out.Assets, AssetItemOut{ID: a.ID, Name: a.Name, Type: string(a.Type), FolderID: a.FolderID,
			URL: a.URL, Width: a.Width, Height: a.Height, Hidden: a.IsHidden})
		fmt.Fprintf(&b, "- %s (%s) %s", a.Name, a.ID, a.Type)
		if a.Width > 0 && a.Height > 0 {
			fmt.Fprintf(&b, " %dx%d", a.Width, a.Height)
		}
		b.WriteString(listFlags(flagIf(a.IsHidden, "hidden")))
		if a.URL != "" {
			fmt.Fprintf(&b, " %s", a.URL)
		}
		b.WriteString("\n")
	}
	pagedFooter(&b, len(out.Assets), "asset(s)", p.NextCursor, p.HasMore)
	return toolResult[ListAssetsOut]{out: out, text: b.String()}, nil
}
