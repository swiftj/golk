// Command lk-mcp is the LegendKeeper MCP server. It speaks MCP over stdio;
// stdout carries only protocol frames, and logs go to stderr.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver"
	"github.com/swiftj/golk/internal/mcpserver/guard"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		_, _ = fmt.Fprintf(os.Stdout, "lk-mcp %s\n", version)
		return
	}
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lk-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := mcpserver.ConfigFromEnv(os.Environ())
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	log.Info("starting", "version", version, "config", cfg.String())
	d := mcpserver.Deps{Logger: log, Version: version}
	if cfg.GuardFile != "" && !cfg.GuardOff {
		d.Guard = guard.Open(cfg.GuardFile)
	} else if !cfg.GuardOff && !cfg.ReadOnly {
		log.Info("no spoiler guard terms file (LK_MCP_GUARD_FILE); writes are not checked")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mcpserver.New(cfg, d).Run(ctx, &mcp.StdioTransport{})
}
