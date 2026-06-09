package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
	"github.com/holtmiu/lark-docs-mcp/internal/feishu"
	"github.com/holtmiu/lark-docs-mcp/internal/mcp"
)

const version = "0.2.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	service := feishu.NewService(cfg)
	server := mcp.NewServer("feishu-doc-mcp-server", version, mcp.FeishuTools{Service: service, AllowCredentialSelection: true})
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
