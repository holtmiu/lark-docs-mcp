package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type countingHandler struct {
	calls int
}

func (h *countingHandler) Tools() []Tool { return nil }

func (h *countingHandler) CallTool(context.Context, string, json.RawMessage) (any, error) {
	h.calls++
	return map[string]any{"ok": true}, nil
}

func TestHTTPServerRejectsMCPWhenAPIKeyMissingByDefault(t *testing.T) {
	h := NewHTTPServer("test", "0", fakeHandler{}, "").Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when remote MCP API key is not configured", w.Code)
	}
}

func TestHTTPServerCanExplicitlyAllowUnauthenticatedMCPForLocalDevelopment(t *testing.T) {
	h := NewHTTPServerWithOptions("test", "0", fakeHandler{}, HTTPServerOptions{AllowUnauthenticated: true}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when unauthenticated mode is explicitly enabled", w.Code)
	}
}

func TestHTTPServerRestrictsCORSOrigins(t *testing.T) {
	h := NewHTTPServerWithOptions("test", "0", fakeHandler{}, HTTPServerOptions{APIKey: "secret", AllowedOrigins: []string{"https://chat.openai.com"}}).Handler()

	allowed := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	allowed.Header.Set("Origin", "https://chat.openai.com")
	allowedRecorder := httptest.NewRecorder()
	h.ServeHTTP(allowedRecorder, allowed)
	if got := allowedRecorder.Header().Get("Access-Control-Allow-Origin"); got != "https://chat.openai.com" {
		t.Fatalf("allowed origin header = %q", got)
	}

	denied := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	denied.Header.Set("Origin", "https://evil.example")
	deniedRecorder := httptest.NewRecorder()
	h.ServeHTTP(deniedRecorder, denied)
	if got := deniedRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("denied origin unexpectedly got CORS header %q", got)
	}
}

func TestHTTPServerRejectsOversizedBodyBeforeJSONHandling(t *testing.T) {
	handler := &countingHandler{}
	h := NewHTTPServerWithOptions("test", "0", handler, HTTPServerOptions{APIKey: "secret", MaxBodyBytes: 32}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(strings.Repeat("{", 64)))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
}

func TestHTTPServerRejectsOversizedBatch(t *testing.T) {
	handler := &countingHandler{}
	h := NewHTTPServerWithOptions("test", "0", handler, HTTPServerOptions{APIKey: "secret", MaxBatchRequests: 2}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"ping"},
		{"jsonrpc":"2.0","id":3,"method":"ping"}
	]`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want JSON-RPC error with HTTP 200", w.Code)
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response did not decode: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Fatalf("response error = %+v, want invalid request", resp.Error)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
}
