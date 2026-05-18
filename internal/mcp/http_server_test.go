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

type fakeHandler struct{}

func (fakeHandler) Tools() []Tool {
	return []Tool{{Name: "x", Description: "x", InputSchema: map[string]any{"type": "object"}}}
}

func (fakeHandler) CallTool(context.Context, string, json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestHTTPServerRequiresBearer(t *testing.T) {
	h := NewHTTPServer("test", "0", fakeHandler{}, "secret").Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHTTPServerPing(t *testing.T) {
	h := NewHTTPServer("test", "0", fakeHandler{}, "secret").Handler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
