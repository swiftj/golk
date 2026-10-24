package lk

import (
	"context"
	"iter"
	"net/url"
	"strconv"
	"time"

	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// Project is a LegendKeeper project.
type Project struct {
	ID       string
	Name     string
	IsPublic bool
	// CoverImageURL and CoverGradient are empty when the project uses the default card.
	CoverImageURL string
	CoverGradient string
	// HomeResourceID is the home page resource, or empty when none is set.
	// Only Projects.Get and Client.Project return it.
	HomeResourceID string
	CreatedAt      time.Time
	LastActivityAt time.Time
}

// ProjectsService lists and reads projects.
type ProjectsService struct{ c *Client }

// ProjectListParams filters Projects.List.
type ProjectListParams struct {
	// Limit is the page size (1–100). Zero uses the server default.
	Limit int
}

// List returns the first page of projects. With an API key, the page holds
// only the key's project.
func (s *ProjectsService) List(ctx context.Context, p ProjectListParams, opts ...CallOption) (*Page[Project], error) {
	return s.pager(p, opts).page(ctx, "")
}

// All iterates over every project.
func (s *ProjectsService) All(ctx context.Context, p ProjectListParams, opts ...CallOption) iter.Seq2[Project, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Project], error) { return s.List(ctx, p, opts...) })
}

func (s *ProjectsService) pager(p ProjectListParams, opts []CallOption) *pager[gen.ProjectList, Project] {
	q := url.Values{}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	return &pager[gen.ProjectList, Project]{
		c: s.c, tmpl: gen.OpListProjects.Path, query: q, opts: opts,
		items: func(w gen.ProjectList) ([]Project, opt.Optional[bool], opt.Nullable[string]) {
			out := make([]Project, len(w.Data))
			for i, p := range w.Data {
				out[i] = projectFromWire(p)
			}
			return out, w.HasMore, w.NextCursor
		},
	}
}

// Get returns project id.
func (s *ProjectsService) Get(ctx context.Context, id string, opts ...CallOption) (*Project, error) {
	var w gen.ProjectDetail
	err := s.c.call(ctx, gen.OpGetProject.Method, gen.OpGetProject.Path,
		map[string]string{"projectId": id}, request{out: &w}, opts)
	if err != nil {
		return nil, err
	}
	p := projectFromWire(gen.Project{
		ID: w.ID, Name: w.Name, IsPublic: w.IsPublic, CreatedAt: w.CreatedAt,
		CoverImageURL: w.CoverImageURL, CoverGradient: w.CoverGradient, LastActivityAt: w.LastActivityAt,
	})
	p.HomeResourceID = w.HomeResourceID.Or("")
	return &p, nil
}

func projectFromWire(w gen.Project) Project {
	return Project{
		ID:             w.ID,
		Name:           w.Name.Or(""),
		IsPublic:       w.IsPublic.Or(false),
		CoverImageURL:  w.CoverImageURL.Or(""),
		CoverGradient:  w.CoverGradient.Or(""),
		CreatedAt:      utc(w.CreatedAt.Or(time.Time{})),
		LastActivityAt: utc(w.LastActivityAt.Or(time.Time{})),
	}
}

func utc(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}

// Project returns the project this client acts in: the configured project,
// or the credential's only project, resolved once and cached.
func (c *Client) Project(ctx context.Context) (*Project, error) {
	id, err := c.resolveProjectID(ctx)
	if err != nil {
		return nil, err
	}
	return c.Projects.Get(ctx, id)
}

// resolveCall is one in-flight project resolution that others can wait on.
type resolveCall struct {
	done chan struct{}
	id   string
	err  error
	// cancelled reports that the resolving caller's context ended, so the
	// error says nothing about the credential and a waiter should try itself.
	cancelled bool
}

// resolveProjectID returns the configured or cached project ID, or resolves
// it with one projects.list call. Concurrent callers share one resolution.
// A failure is not cached, so the next call tries again.
func (c *Client) resolveProjectID(ctx context.Context) (string, error) {
	for {
		c.projMu.Lock()
		if c.projectID != "" {
			id := c.projectID
			c.projMu.Unlock()
			return id, nil
		}
		if call := c.projCall; call != nil {
			c.projMu.Unlock()
			select {
			case <-call.done:
				if call.cancelled && ctx.Err() == nil {
					continue
				}
				return call.id, call.err
			case <-ctx.Done():
				return "", &ConnectionError{Method: "GET", Path: gen.OpListProjects.Path, Err: context.Cause(ctx)}
			}
		}
		call := &resolveCall{done: make(chan struct{})}
		c.projCall = call
		c.projMu.Unlock()

		call.id, call.err = c.lookupProject(ctx)
		call.cancelled = ctx.Err() != nil

		c.projMu.Lock()
		if call.err == nil {
			c.projectID = call.id
		}
		c.projCall = nil
		c.projMu.Unlock()
		close(call.done)
		return call.id, call.err
	}
}

func (c *Client) lookupProject(ctx context.Context) (string, error) {
	page, err := c.Projects.List(ctx, ProjectListParams{})
	if err != nil {
		return "", err
	}
	switch {
	case len(page.Items) > 1 || page.HasMore:
		return "", ErrAmbiguousProject
	case len(page.Items) == 0:
		return "", ErrNoProject
	}
	return page.Items[0].ID, nil
}
