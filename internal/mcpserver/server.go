package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver/guard"
	"github.com/swiftj/golk/lk"
)

// Checker is the spoiler guard as the tools use it (*guard.Guard).
type Checker interface {
	Check(v any) ([]guard.Violation, error)
}

// Deps are what New needs besides the config.
type Deps struct {
	// Logger receives one line per tool call, never content or keys.
	// Nil discards.
	Logger *slog.Logger
	// ClientOptions are added to every lk client (tests point them at a
	// fake server).
	ClientOptions []lk.Option
	// Guard is the spoiler guard. Nil means there are no protected terms:
	// writes are not checked, as with Config.GuardOff (D-017).
	Guard Checker
	// Version is the server version reported to clients.
	Version string
}

// server is the state shared by the tool handlers.
type server struct {
	cfg      Config
	log      *slog.Logger
	projects *projects
	guard    Checker
	// noTerms is set when no terms file is configured, so the guard is off.
	noTerms bool
	// writeMu serializes the write tools (see serial).
	writeMu sync.Mutex
}

// New returns the MCP server with its tools registered. In read-only mode
// only the read tools exist.
func New(cfg Config, d Deps) *mcp.Server {
	log := d.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &server{cfg: cfg, log: log, projects: newProjects(cfg, d.ClientOptions), guard: d.Guard}
	if d.Guard == nil {
		// No terms file: nothing is protected, so writes are not checked
		// (D-017, owner decision replacing D-011's refusal).
		s.noTerms, s.cfg.GuardOff = !cfg.GuardOff, true
	}
	ms := mcp.NewServer(&mcp.Implementation{Name: "lk-mcp", Title: "LegendKeeper", Version: d.Version}, &mcp.ServerOptions{
		Instructions: instructions,
	})
	s.addReadTools(ms)
	if !cfg.ReadOnly {
		s.addWriteTools(ms)
	}
	// Phase 4 tool groups register their read tools always and their
	// write tools only when writes are enabled.
	s.addMapTools(ms, !cfg.ReadOnly)
	s.addTemplateTools(ms, !cfg.ReadOnly)
	s.addTimelineTools(ms, !cfg.ReadOnly)
	s.addCalendarTools(ms, !cfg.ReadOnly)
	s.addBoardTools(ms, !cfg.ReadOnly)
	return ms
}

const instructions = `Tools for a LegendKeeper wiki that players read.
Pages are LK Markdown: read_page returns it with a version; edit_page takes the whole edited page and that version.
Keep :::secret blocks for DM-only text, and keep ::unsupported and lk-raw lines as returned.
Every write tool has dry_run; use it to preview. When the DM has configured protected terms, a spoiler guard refuses writes that put them in player-visible content (list_projects says whether it is on).`

// ProjectArg is the project parameter every tool takes.
type ProjectArg struct {
	Project string `json:"project,omitempty" jsonschema:"configured project name; omit for the default project"`
}

// toolResult is what a handler returns: the structured output and the text
// the model reads.
type toolResult[Out any] struct {
	out  Out
	text string
}

func boolPtr(b bool) *bool { return &b }

// readOnly, write and destructive are tool annotations. Every tool works on
// one closed wiki, so none is open-world.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
}

func write(title string, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: boolPtr(destructive), IdempotentHint: idempotent, OpenWorldHint: boolPtr(false)}
}

// addTool registers a tool whose handler gets the call's lk client. It
// logs one line per call (tool, project, duration, outcome) and turns
// errors into actionable tool errors.
func addTool[In, Out any](s *server, ms *mcp.Server, t *mcp.Tool, project func(In) string,
	h func(ctx context.Context, c *lk.Client, in In) (toolResult[Out], error)) {
	mcp.AddTool(ms, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		start := time.Now()
		c, name, err := s.projects.client(project(in))
		if err == nil {
			var r toolResult[Out]
			r, err = h(ctx, c, in)
			if err == nil {
				s.log.InfoContext(ctx, "tool call", "tool", t.Name, "project", name, "duration", time.Since(start), "ok", true)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.text}}}, r.out, nil
			}
		}
		s.log.InfoContext(ctx, "tool call", "tool", t.Name, "project", name, "duration", time.Since(start), "ok", false, "error", errorKind(err))
		return nil, zero, explain(err)
	})
}

// errorKind is a short, content-free label for logs.
func errorKind(err error) string {
	var ae *lk.APIError
	var ge *GuardError
	switch {
	case errors.As(err, &ae):
		return string(ae.Code)
	case errors.As(err, &ge):
		return "GUARD"
	}
	return "error"
}

// check runs the spoiler guard over v, content about to be written where
// players can see it.
func (s *server) check(v any) error {
	if s.cfg.GuardOff {
		return nil
	}
	vs, err := s.guard.Check(v)
	if err != nil {
		return errors.Join(errors.New("spoiler guard could not run, so the write is refused"), err)
	}
	if len(vs) > 0 {
		s.log.Info("spoiler guard refused a write", "violations", len(vs))
		return &GuardError{Violations: vs}
	}
	return nil
}
