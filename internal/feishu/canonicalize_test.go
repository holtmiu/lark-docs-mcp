package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
)

func TestCanonicalizeIdentityWikiURLToDocxToken(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "docx", "obj_token": "docx-token-123", "url": "https://example.feishu.cn/docx/docx-token-123"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s"})
	identity, err := svc.Resolve("https://example.feishu.cn/wiki/wiki-token-123")
	if err != nil {
		t.Fatalf("Resolve wiki URL: %v", err)
	}
	got, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	if err != nil {
		t.Fatalf("CanonicalizeIdentity returned error: %v", err)
	}
	if gotPath != "/wiki/wiki-token-123" {
		t.Fatalf("path = %q, want escaped wiki token path", gotPath)
	}
	if got.ResourceType != ResourceDocx || got.Token != "docx-token-123" || got.Provider != ProviderFeishu {
		t.Fatalf("canonical identity = %+v, want docx-token-123", got)
	}
	if got.OriginalURL != identity.OriginalURL || got.NormalizedURL == "" {
		t.Fatalf("canonical identity did not preserve URL context: %+v", got)
	}
}

func TestReadDocumentCanonicalizesWikiBeforeDocxAPIs(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/wiki/wiki-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "docx", "obj_token": "docx-token"}}})
		case "/metadata/docx-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"document": map[string]any{"document_id": "docx-token", "title": "Wiki Doc"}}})
		case "/children/docx-token/docx-token/children":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []any{map[string]any{"block_id": "b1", "block_type": "text", "text": "hello"}}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", DocMaxBlocks: 10, DocMaxDepth: 2, WikiNodePathTemplate: "/wiki/%s", DocxMetadataPathTemplate: "/metadata/%s", DocxChildrenPathTemplate: "/children/%s/%s/children"})
	got, err := svc.ReadDocumentWithActor(context.Background(), "https://example.feishu.cn/wiki/wiki-token", ReadOptions{}, ActorContext{})
	if err != nil {
		t.Fatalf("ReadDocumentWithActor returned error: %v", err)
	}
	if got.Metadata.DocumentID != "docx-token" || len(got.Blocks) != 1 || got.Blocks[0].ID != "b1" {
		t.Fatalf("read result = %+v", got)
	}
	joined := strings.Join(paths, ",")
	if strings.Contains(joined, "/metadata/wiki-token") || strings.Contains(joined, "/children/wiki-token") {
		t.Fatalf("read used wiki token for docx APIs: %v", paths)
	}
}

func TestCheckPermissionCanonicalizesDriveFileToDocx(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/drive/drive-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"file": map[string]any{"type": "docx", "docx_token": "docx-token"}}})
		case "/permission/docx-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": true}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", DriveFileMetadataPathTemplate: "/drive/%s", DocxPermissionPathTemplate: "/permission/%s"})
	got, err := svc.CheckPermissionWithActor(context.Background(), "https://example.feishu.cn/drive/file/drive-token", ActorContext{})
	if err != nil {
		t.Fatalf("CheckPermissionWithActor returned error: %v", err)
	}
	if !got.CanRead || !got.CanWrite || !got.CanComment {
		t.Fatalf("permission = %+v", got)
	}
}

func TestCanonicalizeUnsupportedDriveFileReturnsActionableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"file": map[string]any{"type": "sheet", "file_token": "sheet-token"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", DriveFileMetadataPathTemplate: "/drive/%s"})
	identity := DocumentIdentity{Provider: ProviderFeishu, ResourceType: ResourceDriveFile, Token: "drive-token"}
	_, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrUnsupportedDocumentType {
		t.Fatalf("error = %v, want UNSUPPORTED_DOCUMENT_TYPE", err)
	}
	if !strings.Contains(strings.ToLower(connectorErr.Message), "docx") || !strings.Contains(strings.ToLower(connectorErr.Message), "sheet") {
		t.Fatalf("error message = %q, want actionable docx/sheet message", connectorErr.Message)
	}
}

func TestCanonicalizeRejectsLegacyDocType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "doc", "obj_token": "legacy-doc-token"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s"})
	identity := DocumentIdentity{Provider: ProviderFeishu, ResourceType: ResourceWiki, Token: "wiki-token"}
	_, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrUnsupportedDocumentType {
		t.Fatalf("error = %v, want UNSUPPORTED_DOCUMENT_TYPE", err)
	}
}

func TestCanonicalizeAllowsMissingTypeWhenDocxTokenIsPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"docx_token": "docx-token-without-type"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s"})
	identity := DocumentIdentity{Provider: ProviderFeishu, ResourceType: ResourceWiki, Token: "wiki-token"}
	got, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	if err != nil {
		t.Fatalf("CanonicalizeIdentity returned error: %v", err)
	}
	if got.ResourceType != ResourceDocx || got.Token != "docx-token-without-type" {
		t.Fatalf("canonical identity = %+v", got)
	}
}

func TestCanonicalizeRejectsMissingTypeWithOnlyGenericToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_token": "generic-token-without-type"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s"})
	identity := DocumentIdentity{Provider: ProviderFeishu, ResourceType: ResourceWiki, Token: "wiki-token"}
	_, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrUnsupportedDocumentType {
		t.Fatalf("error = %v, want UNSUPPORTED_DOCUMENT_TYPE", err)
	}
}

func TestCanonicalizePrefersDocxTokenOverGenericToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "docx", "obj_token": "generic-token", "docx_token": "docx-token"}}})
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s"})
	identity := DocumentIdentity{Provider: ProviderFeishu, ResourceType: ResourceWiki, Token: "wiki-token"}
	got, err := svc.CanonicalizeIdentity(context.Background(), identity, ActorContext{})
	if err != nil {
		t.Fatalf("CanonicalizeIdentity returned error: %v", err)
	}
	if got.Token != "docx-token" {
		t.Fatalf("canonical token = %q, want docx-token", got.Token)
	}
}
