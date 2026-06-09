package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
)

func TestPermissionAllowedResponseMapsCapabilities(t *testing.T) {
	svc, _, closeServer := newPermissionTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/permission/doc-token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"can_read":    true,
				"can_write":   true,
				"can_comment": true,
				"visibility":  "shared",
			},
		})
	})
	defer closeServer()

	got, err := svc.CheckPermission(context.Background(), "doc-token")
	if err != nil {
		t.Fatalf("CheckPermission returned error: %v", err)
	}
	if !got.CanRead || !got.CanWrite || !got.CanComment {
		t.Fatalf("snapshot = %+v, want all capabilities true", got)
	}
	if got.Visibility != "shared" {
		t.Fatalf("Visibility = %q", got.Visibility)
	}
}

func TestPermissionFeishuPublicResponseMapsCommentCapability(t *testing.T) {
	svc, _, closeServer := newPermissionTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"permission_public": map[string]any{
					"comment_entity":    "anyone_can_view",
					"external_access":   true,
					"invite_external":   true,
					"link_share_entity": "tenant_readable",
					"lock_switch":       false,
					"security_entity":   "anyone_can_view",
					"share_entity":      "anyone",
				},
			},
		})
	})
	defer closeServer()

	got, err := svc.CheckPermission(context.Background(), "doc-token")
	if err != nil {
		t.Fatalf("CheckPermission returned error: %v", err)
	}
	if !got.CanRead || got.CanWrite || !got.CanComment {
		t.Fatalf("snapshot = %+v, want Feishu public view/comment capability without edit permission", got)
	}
	if got.Visibility != "tenant_readable" {
		t.Fatalf("Visibility = %q", got.Visibility)
	}
}

func TestPermissionDeniedResponseMapsPermissionDeniedSnapshot(t *testing.T) {
	svc, _, closeServer := newPermissionTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"permission": map[string]any{
					"can_read":       true,
					"can_write":      false,
					"can_comment":    false,
					"reason":         "viewer only",
					"requiredScopes": []any{"docs:doc:write"},
				},
			},
		})
	})
	defer closeServer()

	got, err := svc.CheckPermission(context.Background(), "doc-token")
	if err != nil {
		t.Fatalf("CheckPermission returned error: %v", err)
	}
	if !got.CanRead || got.CanWrite || got.CanComment {
		t.Fatalf("snapshot = %+v, want read allowed but write/comment denied", got)
	}
	if got.SuggestedAction == "" {
		t.Fatalf("SuggestedAction is empty for denied write/comment snapshot: %+v", got)
	}
	if len(got.RequiredScopes) != 1 || got.RequiredScopes[0] != "docs:doc:write" {
		t.Fatalf("RequiredScopes = %#v", got.RequiredScopes)
	}
}

func TestPermissionUpstream403DoesNotAttemptWriteOrCommentProbe(t *testing.T) {
	var paths []string
	svc, _, closeServer := newPermissionTestService(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "write") || strings.Contains(r.URL.Path, "comment") {
			t.Fatalf("unexpected write/comment probe path called: %s", r.URL.Path)
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	defer closeServer()

	_, err := svc.CheckPermission(context.Background(), "doc-token")
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrPermissionDenied {
		t.Fatalf("error = %v, want PERMISSION_DENIED", err)
	}
	if len(paths) != 1 || paths[0] != "/permission/doc-token" {
		t.Fatalf("paths = %#v, want only permission endpoint", paths)
	}
}

func newPermissionTestService(t *testing.T, handler http.HandlerFunc) (*Service, *httptest.Server, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	svc := NewService(config.Config{
		Provider:                       "feishu",
		BaseURL:                        server.URL,
		TenantAccessToken:              "tenant-token",
		DocxPermissionPathTemplate:     "/permission/%s",
		DocxAppendChildrenPathTemplate: "/append/%s/%s",
		DocxMetadataPathTemplate:       "/metadata/%s",
		DocxChildrenPathTemplate:       "/children/%s/%s",
		DocxCreatePath:                 "/create",
		APIMaxRetries:                  0,
	})
	return svc, server, server.Close
}
