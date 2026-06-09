package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
)

func TestCreateDocumentFolderPermissionDoesNotCanonicalizeFolderAsDocx(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/open-apis/drive/v1/permissions/folder-token/public":
			if r.URL.RawQuery != "type=folder" {
				t.Fatalf("folder permission query = %q, want type=folder", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true}})
		case "/create":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"document": map[string]any{"document_id": "new-docx-token"}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	dryRun := false
	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: true, DriveFileMetadataPathTemplate: "/drive/%s", DocxCreatePath: "/create"})
	got, err := svc.CreateDocumentWithActor(context.Background(), CreateDocumentRequest{Title: "Created", FolderToken: "folder-token", DryRun: &dryRun}, ActorContext{})
	if err != nil {
		t.Fatalf("CreateDocumentWithActor returned error: %v", err)
	}
	if got.DocumentID != "new-docx-token" {
		t.Fatalf("result = %+v", got)
	}
	if len(paths) != 2 || paths[0] != "/open-apis/drive/v1/permissions/folder-token/public" || paths[1] != "/create" {
		t.Fatalf("paths = %v, want raw folder permission check then create", paths)
	}
}
