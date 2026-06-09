package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
	"github.com/holtmiu/lark-docs-mcp/internal/mcp"
)

const version = "0.2.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	if err := cfg.ValidateRemoteMCPSecurity(); err != nil {
		fmt.Fprintf(os.Stderr, "security configuration error: %v\n", err)
		os.Exit(1)
	}
	tools, err := mcp.NewFeishuToolsFromConfig(cfg, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill registry configuration error: %v\n", err)
		os.Exit(1)
	}
	h := mcp.NewHTTPServerWithOptions("feishu-doc-mcp-http-server", version, tools, mcp.HTTPServerOptions{
		APIKey:               cfg.MCPServerAPIKey,
		AllowUnauthenticated: cfg.MCPAllowUnauthenticated,
		AllowedOrigins:       cfg.MCPAllowedOrigins,
		MaxBodyBytes:         int64(cfg.MCPMaxBodyBytes),
		MaxBatchRequests:     cfg.MCPMaxBatchRequests,
	})
	server := &http.Server{
		Addr:              cfg.MCPHTTPAddr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "feishu-doc remote MCP server listening on %s\n", cfg.MCPHTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
