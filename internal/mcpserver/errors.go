package mcpserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/swiftj/golk/internal/mcpserver/guard"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

// GuardError reports a write the spoiler guard refused.
type GuardError struct {
	Violations []guard.Violation
}

func (e *GuardError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "spoiler guard: refused, %d protected term(s) in player-visible content:", len(e.Violations))
	for _, v := range e.Violations {
		fmt.Fprintf(&b, "\n- %q at %s", v.Term, v.Path)
	}
	b.WriteString("\nHint: move that text into a :::secret block, or into a hidden document, resource or property, or reword it.")
	return b.String()
}

// errNoGuard refuses a write when no guard is configured.
var errNoGuard = errors.New("spoiler guard: no terms file is configured, so writes are refused. Hint: set LK_MCP_GUARD_FILE to the protected-terms file, or LK_MCP_GUARD=off to write without a guard")

// explain turns err into a tool error a model can act on: the code, the
// request ID and a hint. It never includes an API key; lk errors don't
// carry one.
func explain(err error) error {
	var ae *lk.APIError
	var se *markdown.SyntaxError
	var ide *markdown.IDError
	var ge *GuardError
	switch {
	case errors.As(err, &ge), errors.Is(err, errNoGuard):
		return err
	case errors.As(err, &ae):
		var b strings.Builder
		fmt.Fprintf(&b, "LegendKeeper error %s (HTTP %d)", orDash(string(ae.Code)), ae.Status)
		if ae.RequestID != "" {
			fmt.Fprintf(&b, ", request ID %s", ae.RequestID)
		}
		if ae.Message != "" {
			fmt.Fprintf(&b, ": %s", ae.Message)
		}
		for _, fe := range ae.Errors {
			fmt.Fprintf(&b, "\n- %s: %s", fe.Path, fe.Message)
		}
		if h := hint(ae); h != "" {
			fmt.Fprintf(&b, "\nHint: %s", h)
		}
		return errors.New(b.String())
	case errors.As(err, &se):
		return fmt.Errorf("%s\nHint: fix the Markdown at that line; LK Markdown fails closed on anything it can't read exactly", se.Error())
	case errors.As(err, &ide):
		return fmt.Errorf("%s\nHint: keep each {#id} and ::unsupported line as read_page returned it", ide.Error())
	case errors.Is(err, markdown.ErrRemovesOpaque):
		return fmt.Errorf("%s\nHint: keep the ::unsupported and lk-raw lines from read_page; pass allow_remove_opaque only if deleting that content is intended", err.Error())
	case errors.Is(err, lk.ErrTimeout):
		return fmt.Errorf("%s\nHint: LegendKeeper did not answer in time; try again", err.Error())
	}
	var ce *lk.ConnectionError
	if errors.As(err, &ce) {
		return fmt.Errorf("could not reach LegendKeeper: %s\nHint: check the network and LEGENDKEEPER_BASE_URL", err.Error())
	}
	return err
}

func hint(e *lk.APIError) string {
	switch e.Code {
	case lk.ErrVersionMismatch:
		return "the page changed since it was read; call read_page again and redo the edit on the new version"
	case lk.ErrNotFound:
		return "check the id; hidden content is visible only to admin keys"
	case lk.ErrValidation, lk.ErrInvalidContent:
		return "fix the fields listed above"
	case lk.ErrRateLimited, lk.ErrUploadRateExceeded:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("rate limited; wait %s and try again", e.RetryAfter)
		}
		return "rate limited; wait and try again"
	case lk.ErrForbidden, lk.ErrInsufficientScope:
		return "the key's role can't do this; an admin key may be needed"
	case lk.ErrHasChildren:
		return "the resource has children; pass cascade to delete them too, or move them first"
	case lk.ErrUnauthenticated:
		return "the API key is invalid or revoked"
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
