package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
)

func TestListCommentsCanonicalizesWikiToDocxFile(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/wiki/wiki-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "docx", "obj_token": "docx-token"}}})
		case "/comments/docx-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []any{map[string]any{"comment_id": "c1", "content": "hello"}}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WikiNodePathTemplate: "/wiki/%s", DocxCommentsPathTemplate: "/comments/%s"})
	got, err := svc.ListComments(context.Background(), "https://example.feishu.cn/wiki/wiki-token", ListCommentsRequest{}, ActorContext{})
	if err != nil {
		t.Fatalf("ListComments returned error: %v", err)
	}
	if got.DocumentID != "docx-token" || len(got.Comments) != 1 || got.Comments[0].ID != "c1" {
		t.Fatalf("comments result = %+v", got)
	}
	if len(paths) != 2 || paths[0] != "/wiki/wiki-token" || paths[1] != "/comments/docx-token" {
		t.Fatalf("paths = %v, want wiki resolution then docx comments", paths)
	}
}
