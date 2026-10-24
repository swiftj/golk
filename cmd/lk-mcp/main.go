// Command lk-mcp is the LegendKeeper MCP server. In phase 1 it is a stub that
// prints its version, so the static release pipeline can be built and checked.
package main

import (
	"fmt"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	_, _ = fmt.Fprintf(os.Stdout, "lk-mcp %s\n", version)
}
