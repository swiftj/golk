package mcpserver

import (
	"fmt"
	"strings"
	"sync"

	"github.com/swiftj/golk/lk"
)

// projects holds one lk.Client per configured project, made on first use.
// Each client has its own rate limiter, since limits are per key.
type projects struct {
	cfg  Config
	opts []lk.Option

	mu      sync.Mutex
	clients map[string]*lk.Client
}

func newProjects(cfg Config, opts []lk.Option) *projects {
	return &projects{cfg: cfg, opts: opts, clients: map[string]*lk.Client{}}
}

// resolve returns the project name a tool call means: name itself, or with
// none given the default project, or the only project when there is one.
func (p *projects) resolve(name string) (string, error) {
	names := p.cfg.Projects()
	if len(names) == 0 {
		return "", fmt.Errorf("no LegendKeeper project is configured: set LEGENDKEEPER_API_KEY (or LEGENDKEEPER_API_KEY_<NAME>) in the server's environment")
	}
	if name == "" {
		if _, ok := p.cfg.keys[DefaultProject]; ok {
			return DefaultProject, nil
		}
		if len(names) == 1 {
			return names[0], nil
		}
		return "", fmt.Errorf("several projects are configured (%s): pass project", strings.Join(names, ", "))
	}
	name = strings.ToLower(name)
	if _, ok := p.cfg.keys[name]; !ok {
		return "", fmt.Errorf("unknown project %q: configured projects are %s", name, strings.Join(names, ", "))
	}
	return name, nil
}

// client returns the client of the project a tool call means, and its name.
func (p *projects) client(name string) (*lk.Client, string, error) {
	name, err := p.resolve(name)
	if err != nil {
		return nil, "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[name]; ok {
		return c, name, nil
	}
	opts := []lk.Option{lk.WithUserAgent("lk-mcp")}
	if p.cfg.BaseURL != "" {
		opts = append(opts, lk.WithBaseURL(p.cfg.BaseURL))
	}
	c, err := lk.New(p.cfg.keys[name], append(opts, p.opts...)...)
	if err != nil {
		return nil, "", fmt.Errorf("project %s: %w", name, err)
	}
	p.clients[name] = c
	return c, name, nil
}
