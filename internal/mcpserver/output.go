package mcpserver

import (
	"fmt"
	"strings"
)

// checkLimit validates an optional 1–100 page size.
func checkPageLimit(limit int) error {
	if limit < 0 || limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100, not %d (omit it for the server default)", limit)
	}
	return nil
}

// flags renders bracketed flags, such as " [hidden]", or "".
func listFlags(fs ...string) string {
	var on []string
	for _, f := range fs {
		if f != "" {
			on = append(on, f)
		}
	}
	if len(on) == 0 {
		return ""
	}
	return " [" + strings.Join(on, ", ") + "]"
}

func flagIf(b bool, name string) string {
	if b {
		return name
	}
	return ""
}

// moreLine is the closing line of a paged listing.
func pagedFooter(b *strings.Builder, n int, noun, next string, hasMore bool) {
	switch {
	case hasMore && next != "":
		fmt.Fprintf(b, "%d %s on this page; more follow: pass cursor %q for the next page.", n, noun, next)
	case hasMore:
		fmt.Fprintf(b, "%d %s on this page; the server says more follow but gave no cursor.", n, noun)
	default:
		fmt.Fprintf(b, "%d %s; no more pages.", n, noun)
	}
}
