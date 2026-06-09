package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
	"github.com/holtmiu/lark-docs-mcp/internal/mcp"
)

const version = "0.2.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	tools, err := mcp.NewFeishuToolsFromConfig(cfg, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill registry configuration error: %v\n", err)
		os.Exit(1)
	}
	server := mcp.NewServer("feishu-doc-mcp-server", version, tools)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
