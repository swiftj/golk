// Package guard is lk-mcp's spoiler guard. It finds protected terms in
// content bound for players, so a write can be refused before it is sent.
// The terms come from a file outside the repository (LK_MCP_GUARD_FILE).
//
// The rules, in brief (X-GUARD-01..03, D-011, D-012):
//
//   - The terms file is UTF-8, one term per line. Lines are trimmed; blank
//     lines and lines starting with '#' are skipped. It is read on the
//     first Check and re-read whenever its identity, size or mtime changes.
//     A file that changes while it is read, or that held terms and now
//     holds none (a rewrite in progress), fails the Check.
//   - A term matches case-insensitively (Unicode simple folding) as whole
//     words inside one string; see match.go.
//   - Every string value of the checked content is examined, except inside
//     an object marked "kind":"secret" or "isHidden":true; see visible.go.
package guard

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// Violation is one protected term found in player-visible content.
type Violation struct {
	// Term is the protected term as written in the terms file.
	Term string
	// Path is where it was found: a JSON path into the checked value
	// ($.blocks[2].content[0].text, $.name, $.tags[1]).
	Path string
}

// Guard checks content against the protected terms. It is safe for
// concurrent use.
type Guard struct {
	path string

	mu     sync.Mutex
	info   os.FileInfo // stat of the file the current matcher came from
	loaded *matcher
}

// Open returns a guard that reads its terms from path. The file is read on
// first use and again whenever it changes.
func Open(path string) *Guard { return &Guard{path: path} }

// Check returns the protected terms in the player-visible strings of v,
// which is any JSON-encodable value (a blocks.List, a property, a struct
// of names). See visible.go for what counts as visible. An error means the
// guard could not run, and the write must be refused.
//
// Violations come in document order of v's JSON encoding, then in the
// order the terms appear in the file; each (term, path) pair appears once.
func (g *Guard) Check(v any) ([]Violation, error) {
	m, err := g.matcher()
	if err != nil {
		return nil, err
	}
	return check(m, v)
}

// Terms returns the number of protected terms currently loaded, loading or
// reloading the file as Check would. It never returns the terms themselves,
// so it is safe to log.
func (g *Guard) Terms() (int, error) {
	m, err := g.matcher()
	if err != nil {
		return 0, err
	}
	return len(m.terms), nil
}

// matcher returns the matcher for the terms file as it is now, re-reading
// the file when its stat differs from the one last loaded. A failure is
// never cached: every Check stats the file again.
func (g *Guard) matcher() (*matcher, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fi, err := os.Stat(g.path)
	if err != nil {
		return nil, g.fileErr(err)
	}
	if !fi.Mode().IsRegular() {
		return nil, g.fileErr(errors.New("not a regular file"))
	}
	if g.loaded != nil && sameFile(g.info, fi) {
		return g.loaded, nil
	}
	data, err := os.ReadFile(g.path)
	if err != nil {
		return nil, g.fileErr(err)
	}
	// A file being rewritten in place (truncated, then written) can be
	// read half-way. Refuse unless the file is the same before and after
	// the read; and a file that was holding terms and now reads as none is
	// taken to be mid-write (an emptied terms file needs a restart). The
	// error isn't cached, so the next Check tries again.
	after, err := os.Stat(g.path)
	if err != nil {
		return nil, g.fileErr(err)
	}
	if !sameFile(fi, after) || int64(len(data)) != fi.Size() {
		return nil, g.fileErr(errors.New("changed while being read; try again"))
	}
	terms, err := parseTerms(data)
	if err != nil {
		return nil, g.fileErr(err)
	}
	m := newMatcher(terms)
	if len(m.terms) == 0 && g.loaded != nil && len(g.loaded.terms) > 0 {
		return nil, g.fileErr(errors.New("now holds no terms, though it held some; it may be mid-write. Restart lk-mcp if it was emptied on purpose"))
	}
	g.info, g.loaded = fi, m
	return g.loaded, nil
}

func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// fileErr describes a terms-file problem by path and reason only; it never
// includes file content.
func (g *Guard) fileErr(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("guard: terms file %q: %w", g.path, err)
}

// ParseTerms parses a terms file: UTF-8, one term per line, each line
// trimmed of surrounding white space, blank lines and lines starting with
// '#' skipped, and a leading byte-order mark ignored. Invalid UTF-8 is an
// error that names the line number but not its content. An empty result is
// valid.
func ParseTerms(data []byte) ([]string, error) { return parseTerms(data) }

func parseTerms(data []byte) ([]string, error) {
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	if !utf8.Valid(data) {
		line := 1 + bytes.Count(data[:invalidAt(data)], []byte("\n"))
		return nil, fmt.Errorf("line %d is not valid UTF-8", line)
	}
	var terms []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, len(data)+1)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		terms = append(terms, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return terms, nil
}

func invalidAt(b []byte) int {
	for i := 0; i < len(b); {
		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && n <= 1 {
			return i
		}
		i += n
	}
	return len(b)
}
