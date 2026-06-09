package mcp

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMaxBodyBytes     int64 = 16 * 1024 * 1024
	defaultMaxBatchRequests       = 50
)

type HTTPServer struct {
	server               *Server
	apiKey               string
	allowUnauthenticated bool
	allowedOrigins       []string
	maxBodyBytes         int64
	maxBatchRequests     int
}

type HTTPServerOptions struct {
	APIKey               string
	AllowUnauthenticated bool
	AllowedOrigins       []string
	MaxBodyBytes         int64
	MaxBatchRequests     int
}

func NewHTTPServer(name, version string, handler Handler, apiKey string) *HTTPServer {
	return NewHTTPServerWithOptions(name, version, handler, HTTPServerOptions{APIKey: apiKey})
}

func NewHTTPServerWithOptions(name, version string, handler Handler, opts HTTPServerOptions) *HTTPServer {
	maxBodyBytes := opts.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	maxBatchRequests := opts.MaxBatchRequests
	if maxBatchRequests <= 0 {
		maxBatchRequests = defaultMaxBatchRequests
	}
	return &HTTPServer{
		server:               NewServer(name, version, handler),
		apiKey:               opts.APIKey,
		allowUnauthenticated: opts.AllowUnauthenticated,
		allowedOrigins:       append([]string(nil), opts.AllowedOrigins...),
		maxBodyBytes:         maxBodyBytes,
		maxBatchRequests:     maxBatchRequests,
	}
}

func (h *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/mcp", h.handleMCP)
	return h.withCORS(mux)
}

func (h *HTTPServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC().Format(time.RFC3339)})
}

func (h *HTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "use POST for JSON-RPC MCP requests"})
		return
	}
	if !h.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing or invalid bearer token"})
		return
	}
	body, err := readLimitedBody(r.Body, h.maxBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
		return
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty JSON-RPC body"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if body[0] == '[' {
		h.handleBatch(ctx, w, body)
		return
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, Response{JSONRPC: "2.0", Error: &Error{Code: -32700, Message: "parse error", Data: err.Error()}})
		return
	}
	if len(req.ID) == 0 {
		_ = h.server.HandleNotification(ctx, req)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, h.server.HandleRequest(ctx, req))
}

func (h *HTTPServer) handleBatch(ctx context.Context, w http.ResponseWriter, body []byte) {
	var reqs []Request
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeJSON(w, http.StatusOK, Response{JSONRPC: "2.0", Error: &Error{Code: -32700, Message: "parse error", Data: err.Error()}})
		return
	}
	if len(reqs) > h.maxBatchRequests {
		writeJSON(w, http.StatusOK, Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid request", Data: "batch request limit exceeded"}})
		return
	}
	responses := make([]Response, 0, len(reqs))
	for _, req := range reqs {
		if len(req.ID) == 0 {
			_ = h.server.HandleNotification(ctx, req)
			continue
		}
		responses = append(responses, h.server.HandleRequest(ctx, req))
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *HTTPServer) authorized(r *http.Request) bool {
	if h.apiKey == "" {
		return h.allowUnauthenticated
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.apiKey)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func readLimitedBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	limited := io.LimitReader(body, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, http.ErrBodyReadAfterClose
	}
	return raw, nil
}

func (h *HTTPServer) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := h.allowedOrigin(r.Header.Get("Origin")); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTPServer) allowedOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return ""
	}
	for _, allowed := range h.allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || allowed == origin {
			return origin
		}
	}
	return ""
}
