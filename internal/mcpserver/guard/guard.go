// Package guard is lk-mcp's spoiler guard. It finds protected terms in
// content bound for players, so a write can be refused before it is sent.
// The terms come from a file outside the repository (LK_MCP_GUARD_FILE).
package guard

import "errors"

// Violation is one protected term found in player-visible content.
type Violation struct {
	// Term is the protected term as written in the terms file.
	Term string
	// Path is where it was found: a JSON path into the checked value
	// ($.blocks[2].content[0].text), or a field name (name, tags[1]).
	Path string
}

// Guard checks content against the protected terms. It is safe for
// concurrent use.
type Guard struct {
	path string
}

// Open returns a guard that reads its terms from path. The file is read on
// first use and again whenever it changes.
func Open(path string) *Guard { return &Guard{path: path} }

// Check returns the protected terms in the player-visible strings of v,
// which is any JSON-encodable value (a blocks.List, a property, a struct
// of names). See visible.go for what counts as visible. An error means the
// guard could not run, and the write must be refused.
func (g *Guard) Check(v any) ([]Violation, error) {
	return nil, errors.New("guard: not implemented")
}
