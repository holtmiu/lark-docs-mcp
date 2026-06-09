package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/feishu"
	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/mcp"
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
	service := feishu.NewService(cfg)
	h := mcp.NewHTTPServerWithOptions("feishu-doc-mcp-http-server", version, mcp.FeishuTools{Service: service}, mcp.HTTPServerOptions{
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
