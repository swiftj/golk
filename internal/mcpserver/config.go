// Package mcpserver is lk-mcp: an MCP server that reads and edits a
// LegendKeeper wiki through the lk SDK, with a spoiler guard on every write.
package mcpserver

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// DefaultProject is the name of the project whose key is
// LEGENDKEEPER_API_KEY.
const DefaultProject = "default"

// Config is lk-mcp's configuration. It is read from the environment only.
type Config struct {
	// keys maps a project name to its API key. It is unexported so a key
	// can't be printed with the config.
	keys map[string]string
	// BaseURL overrides the API host; empty uses production.
	BaseURL string
	// ReadOnly registers only the read tools.
	ReadOnly bool
	// GuardFile is the spoiler guard's terms file.
	GuardFile string
	// GuardOff disables the guard. Without it and without GuardFile, writes
	// are refused.
	GuardOff bool
	LogLevel slog.Level
}

// ConfigFromEnv reads the configuration from environ (os.Environ form):
//
//	LEGENDKEEPER_API_KEY         key of the project named "default"
//	LEGENDKEEPER_API_KEY_<NAME>  key of the project named <name> (lower case)
//	LEGENDKEEPER_BASE_URL        API host
//	LK_MCP_READ_ONLY=true        read tools only
//	LK_MCP_GUARD_FILE            spoiler guard terms file
//	LK_MCP_GUARD=off             run without a spoiler guard
//	LK_MCP_LOG_LEVEL             debug, info (default), warn or error
func ConfigFromEnv(environ []string) (Config, error) {
	c := Config{keys: map[string]string{}, LogLevel: slog.LevelInfo}
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case k == "LEGENDKEEPER_API_KEY":
			if v != "" {
				c.keys[DefaultProject] = v
			}
		case strings.HasPrefix(k, "LEGENDKEEPER_API_KEY_"):
			name := strings.ToLower(strings.TrimPrefix(k, "LEGENDKEEPER_API_KEY_"))
			if v == "" {
				continue
			}
			if name == "" || name == DefaultProject || !validName(name) {
				return Config{}, fmt.Errorf("lk-mcp: %s: project names are letters, digits and _, and %q is reserved", k, DefaultProject)
			}
			c.keys[name] = v
		case k == "LEGENDKEEPER_BASE_URL":
			c.BaseURL = v
		case k == "LK_MCP_READ_ONLY":
			b, err := parseBool(k, v)
			if err != nil {
				return Config{}, err
			}
			c.ReadOnly = b
		case k == "LK_MCP_GUARD_FILE":
			c.GuardFile = v
		case k == "LK_MCP_GUARD":
			switch strings.ToLower(v) {
			case "off":
				c.GuardOff = true
			case "", "on":
			default:
				return Config{}, fmt.Errorf("lk-mcp: LK_MCP_GUARD must be on or off, not %q", v)
			}
		case k == "LK_MCP_LOG_LEVEL":
			if v != "" {
				if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
					return Config{}, fmt.Errorf("lk-mcp: LK_MCP_LOG_LEVEL: %w", err)
				}
			}
		}
	}
	return c, nil
}

// WithKey returns c with name's key set. It is for tests and embedders.
func (c Config) WithKey(name, key string) Config {
	keys := map[string]string{name: key}
	for k, v := range c.keys {
		if k != name {
			keys[k] = v
		}
	}
	c.keys = keys
	return c
}

// Projects returns the configured project names, sorted.
func (c Config) Projects() []string {
	names := make([]string, 0, len(c.keys))
	for n := range c.keys {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// String describes c without its keys.
func (c Config) String() string {
	guard := "off"
	if !c.GuardOff {
		guard = "file"
		if c.GuardFile == "" {
			guard = "unset (writes refused)"
		}
	}
	return fmt.Sprintf("lk-mcp config: projects %v, read-only %t, guard %s", c.Projects(), c.ReadOnly, guard)
}

// GoString describes c for %#v, also without its keys.
func (c Config) GoString() string { return c.String() }

func validName(s string) bool {
	for _, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func parseBool(k, v string) (bool, error) {
	switch strings.ToLower(v) {
	case "", "false", "0", "no":
		return false, nil
	case "true", "1", "yes":
		return true, nil
	}
	return false, fmt.Errorf("lk-mcp: %s must be true or false, not %q", k, v)
}
