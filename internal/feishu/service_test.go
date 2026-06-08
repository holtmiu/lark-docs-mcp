package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
)

func TestAppendDocumentPermissionDeniedDoesNotCallAppendEndpoint(t *testing.T) {
	var appendCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/permission/doc-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": false, "reason": "viewer only"}})
		case "/append/doc-token/doc-token":
			appendCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(config.Config{
		Provider:                       "feishu",
		BaseURL:                        server.URL,
		TenantAccessToken:              "tenant-token",
		WriteDryRunDefault:             false,
		DocxPermissionPathTemplate:     "/permission/%s",
		DocxAppendChildrenPathTemplate: "/append/%s/%s",
	})
	dryRun := false
	_, err := svc.AppendDocument(context.Background(), "doc-token", AppendRequest{Markdown: "body", DryRun: &dryRun})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrPermissionDenied {
		t.Fatalf("error = %v, want PERMISSION_DENIED", err)
	}
	if appendCalled {
		t.Fatalf("append endpoint was called despite denied write permission")
	}
}

func TestAppendDocumentDryRunDoesNotCallPermissionOrAppendEndpoint(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected upstream call during dry-run: %s", r.URL.Path)
	}))
	defer server.Close()

	svc := NewService(config.Config{
		Provider:                       "feishu",
		BaseURL:                        server.URL,
		TenantAccessToken:              "tenant-token",
		WriteDryRunDefault:             false,
		DocxPermissionPathTemplate:     "/permission/%s",
		DocxAppendChildrenPathTemplate: "/append/%s/%s",
	})
	dryRun := true
	result, err := svc.AppendDocument(context.Background(), "doc-token", AppendRequest{Markdown: "body", DryRun: &dryRun})
	if err != nil {
		t.Fatalf("AppendDocument dry-run returned error: %v", err)
	}
	if !result.DryRun || len(result.Warnings) == 0 {
		t.Fatalf("result = %+v, want dry-run preview with warning", result)
	}
	if called {
		t.Fatalf("upstream was called during dry-run")
	}
}
