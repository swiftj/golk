package markdown

import (
	"errors"
	"fmt"
	"strings"
)

// SyntaxError reports LK Markdown that FromMarkdown rejects.
type SyntaxError struct {
	// Line is the 1-based line of the input; 0 when the error is not tied
	// to one line.
	Line   int
	Reason string
}

func (e *SyntaxError) Error() string {
	if e.Line == 0 {
		return "markdown: " + e.Reason
	}
	return fmt.Sprintf("markdown: line %d: %s", e.Line, e.Reason)
}

// ErrRemovesOpaque matches (with errors.Is) the *RemoveError FromMarkdown
// returns for an edit that drops content Markdown can't show (M-SAFE-04).
var ErrRemovesOpaque = errors.New("markdown: edit removes unsupported or lk-raw content of Base")

// RemoveError reports an edit that drops unsupported or lk-raw nodes of
// Base, at any depth (M-SAFE-04). The editor never saw that content, so
// dropping it is refused unless FromOptions.AllowRemoveOpaque is set.
type RemoveError struct {
	// Removed lists each dropped opaque node itself (not the top of the
	// subtree that held it), in Base document order, with Opaque set.
	Removed []RemovedBlock
}

func (e *RemoveError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "markdown: edit removes %d block(s) Markdown can't show (", len(e.Removed))
	for i, r := range e.Removed {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(string(r.Kind))
		if r.ID != "" {
			b.WriteString(" " + r.ID)
		}
	}
	b.WriteString("); keep their ::unsupported or lk-raw lines, or set AllowRemoveOpaque")
	return b.String()
}

// Is reports whether target is ErrRemovesOpaque.
func (e *RemoveError) Is(target error) bool { return target == ErrRemovesOpaque }

// IDError reports an id FromMarkdown rejects (M-ID-03, M-SAFE-06): the
// same id twice, an id whose Base node is of another kind, or an
// ::unsupported id that is not in Base.
type IDError struct {
	ID string
	// Path is where the parsed node is, as blocks.Walk writes it.
	Path   string
	Reason string
}

func (e *IDError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("markdown: %s: %s", e.Path, e.Reason)
	}
	return fmt.Sprintf("markdown: %s: id %q: %s", e.Path, e.ID, e.Reason)
}
