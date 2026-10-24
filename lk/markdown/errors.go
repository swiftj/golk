package markdown

import "fmt"

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
