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

func TestCommentModelJSONFieldNamesAndOmitEmpty(t *testing.T) {
	payload, err := json.Marshal(Comment{ID: "c-1", Content: "hello"})
	if err != nil {
		t.Fatalf("Marshal Comment: %v", err)
	}
	got := string(payload)
	if got != `{"id":"c-1","content":"hello"}` {
		t.Fatalf("Comment JSON = %s", got)
	}

	withOptional, err := json.Marshal(Comment{
		ID:          "c-1",
		Content:     "hello",
		AuthorID:    "ou_1",
		CreatedTime: "100",
		UpdatedTime: "200",
		Resolved:    true,
		Quote:       "quoted text",
	})
	if err != nil {
		t.Fatalf("Marshal Comment optional fields: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(withOptional, &decoded); err != nil {
		t.Fatalf("Unmarshal optional JSON: %v", err)
	}
	for _, key := range []string{"id", "content", "authorId", "createdTime", "updatedTime", "resolved", "quote"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("optional Comment JSON missing key %q in %s", key, string(withOptional))
		}
	}

	listPayload, err := json.Marshal(CommentListResult{DocumentID: "doc-1", Comments: []Comment{}})
	if err != nil {
		t.Fatalf("Marshal CommentListResult: %v", err)
	}
	if string(listPayload) != `{"documentId":"doc-1","comments":[]}` {
		t.Fatalf("CommentListResult JSON = %s", listPayload)
	}
}

func TestCreateCommentRequestModelJSONFieldNamesAndOmitEmpty(t *testing.T) {
	dryRun := true
	payload, err := json.Marshal(CreateCommentRequest{Content: "hello", BlockID: "blk-1", Quote: "quote", DryRun: &dryRun, OperationID: "op-1"})
	if err != nil {
		t.Fatalf("Marshal CreateCommentRequest: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal request JSON: %v", err)
	}
	for _, key := range []string{"content", "blockId", "quote", "dryRun", "operationId"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("CreateCommentRequest JSON missing key %q in %s", key, string(payload))
		}
	}

	payload, err = json.Marshal(CreateCommentRequest{Content: "hello"})
	if err != nil {
		t.Fatalf("Marshal minimal CreateCommentRequest: %v", err)
	}
	if string(payload) != `{"content":"hello"}` {
		t.Fatalf("minimal CreateCommentRequest JSON = %s", payload)
	}
}

func TestCreateCommentRequestValidateRejectsEmptyContent(t *testing.T) {
	err := (CreateCommentRequest{Content: " \t\n"}).Validate()
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrInvalidInput {
		t.Fatalf("Validate error = %v, want INVALID_INPUT", err)
	}
}

func TestCreateCommentRequestValidateRejectsOverlyLongContent(t *testing.T) {
	err := (CreateCommentRequest{Content: strings.Repeat("x", maxCommentContentLength+1)}).Validate()
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrInvalidInput {
		t.Fatalf("Validate error = %v, want INVALID_INPUT", err)
	}
}

func TestCommentListPathPaginationAndResponseNormalization(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []any{map[string]any{
					"comment_id":  "c/1",
					"content":     "hello",
					"user_id":     "ou_1",
					"create_time": "100",
					"update_time": "200",
					"is_solved":   true,
					"quote":       "quoted",
				}},
				"has_more":   true,
				"page_token": "next-1",
			},
		})
	}))
	defer server.Close()

	svc := NewService(config.Config{
		Provider:                       "feishu",
		BaseURL:                        server.URL,
		TenantAccessToken:              "tenant-token",
		DocxCommentsPathTemplate:       "/comments/%s",
		WriteDryRunDefault:             true,
		DocxPermissionPathTemplate:     "/permission/%s",
		DocxCommentResolvePathTemplate: "/comments/%s/%s",
	})
	got, err := svc.ListComments(context.Background(), "doc-token-123", ListCommentsRequest{PageSize: 20, PageToken: "prev"}, ActorContext{})
	if err != nil {
		t.Fatalf("ListComments returned error: %v", err)
	}
	if gotPath != "/comments/doc-token-123" {
		t.Fatalf("path = %q, want document token path", gotPath)
	}
	if !strings.Contains(gotQuery, "page_size=20") || !strings.Contains(gotQuery, "page_token=prev") {
		t.Fatalf("query = %q, want pagination params", gotQuery)
	}
	if got.DocumentID != "doc-token-123" || !got.HasMore || got.PageToken != "next-1" || len(got.Comments) != 1 {
		t.Fatalf("result = %+v", got)
	}
	comment := got.Comments[0]
	if comment.ID != "c/1" || comment.Content != "hello" || comment.AuthorID != "ou_1" || comment.CreatedTime != "100" || comment.UpdatedTime != "200" || !comment.Resolved || comment.Quote != "quoted" {
		t.Fatalf("normalized comment = %+v", comment)
	}
}

func TestCommentCreateBodyDryRunAndPermissionGate(t *testing.T) {
	var mutationCalls int
	var permissionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/permission/doc-token":
			permissionCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": true}})
		case "/comments/doc-token":
			mutationCalls++
			if r.Method != http.MethodPost {
				t.Fatalf("create method = %s, want POST", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if body["content"] != "hello" || body["block_id"] != "blk/1" || body["quote"] != "quoted" {
				t.Fatalf("create body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"comment": map[string]any{"comment_id": "c-1", "content": "hello"}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: false, DocxCommentsPathTemplate: "/comments/%s", DocxPermissionPathTemplate: "/permission/%s"})
	dryRun := true
	preview, err := svc.CreateComment(context.Background(), "doc-token", CreateCommentRequest{Content: "hello", DryRun: &dryRun}, ActorContext{})
	if err != nil {
		t.Fatalf("CreateComment dry-run returned error: %v", err)
	}
	if !preview.DryRun || preview.Comment.ID != "" || len(preview.Warnings) == 0 || mutationCalls != 0 || permissionCalls != 0 {
		t.Fatalf("dry-run result=%+v mutationCalls=%d permissionCalls=%d", preview, mutationCalls, permissionCalls)
	}

	dryRun = false
	created, err := svc.CreateComment(context.Background(), "doc-token", CreateCommentRequest{Content: "hello", BlockID: "blk/1", Quote: "quoted", DryRun: &dryRun}, ActorContext{})
	if err != nil {
		t.Fatalf("CreateComment returned error: %v", err)
	}
	if created.DryRun || created.Comment.ID != "c-1" || mutationCalls != 1 || permissionCalls != 1 {
		t.Fatalf("created=%+v mutationCalls=%d permissionCalls=%d", created, mutationCalls, permissionCalls)
	}
}

func TestCommentCreatePermissionDeniedDoesNotCallMutationEndpoint(t *testing.T) {
	var mutationCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/permission/doc-token":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": false, "reason": "viewer only"}})
		case "/comments/doc-token":
			mutationCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dryRun := false
	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: false, DocxCommentsPathTemplate: "/comments/%s", DocxPermissionPathTemplate: "/permission/%s"})
	_, err := svc.CreateComment(context.Background(), "doc-token", CreateCommentRequest{Content: "hello", DryRun: &dryRun}, ActorContext{})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrPermissionDenied {
		t.Fatalf("CreateComment error=%v, want PERMISSION_DENIED", err)
	}
	if mutationCalled {
		t.Fatalf("comment create endpoint called despite denied comment permission")
	}
}

func TestCommentReplyBodyAndPathEscaping(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/permission/doc-token-123":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": true}})
		case "/comments/doc-token-123/c%2F1/replies":
			gotPath = r.URL.EscapedPath()
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode reply body: %v", err)
			}
			if body["content"] != "reply" {
				t.Fatalf("reply body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"reply": map[string]any{"reply_id": "r-1", "content": "reply"}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	dryRun := false
	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: false, DocxCommentsPathTemplate: "/comments/%s", DocxCommentRepliesPathTemplate: "/comments/%s/%s/replies", DocxPermissionPathTemplate: "/permission/%s"})
	got, err := svc.ReplyComment(context.Background(), "doc-token-123", "c/1", ReplyCommentRequest{Content: "reply", DryRun: &dryRun}, ActorContext{})
	if err != nil {
		t.Fatalf("ReplyComment returned error: %v", err)
	}
	if gotPath != "/comments/doc-token-123/c%2F1/replies" || got.Comment.ID != "r-1" {
		t.Fatalf("path=%q result=%+v", gotPath, got)
	}
}

func TestCommentResolveBodyPathAndPermissionGate(t *testing.T) {
	var resolveCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/permission/doc-token-123":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": false, "reason": "comments disabled"}})
		case "/comments/doc-token-123/c%2F1":
			resolveCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	dryRun := false
	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: false, DocxCommentResolvePathTemplate: "/comments/%s/%s", DocxPermissionPathTemplate: "/permission/%s"})
	_, err := svc.ResolveComment(context.Background(), "doc-token-123", "c/1", ResolveCommentRequest{Resolved: true, DryRun: &dryRun}, ActorContext{})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrPermissionDenied {
		t.Fatalf("ResolveComment error=%v, want PERMISSION_DENIED", err)
	}
	if resolveCalled {
		t.Fatalf("resolve endpoint called despite denied comment permission")
	}
}

func TestCommentResolveBodyPathAndSuccess(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/permission/doc-token-123":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"can_read": true, "can_write": true, "can_comment": true}})
		case "/comments/doc-token-123/c%2F1":
			gotMethod = r.Method
			gotPath = r.URL.EscapedPath()
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode resolve body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"comment": map[string]any{"comment_id": "c/1", "is_solved": true}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	dryRun := false
	svc := NewService(config.Config{Provider: "feishu", BaseURL: server.URL, TenantAccessToken: "tenant-token", WriteDryRunDefault: false, DocxCommentResolvePathTemplate: "/comments/%s/%s", DocxPermissionPathTemplate: "/permission/%s"})
	got, err := svc.ResolveComment(context.Background(), "doc-token-123", "c/1", ResolveCommentRequest{Resolved: true, DryRun: &dryRun}, ActorContext{})
	if err != nil {
		t.Fatalf("ResolveComment returned error: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/comments/doc-token-123/c%2F1" || gotBody["resolved"] != true || got.Comment.ID != "c/1" || !got.Comment.Resolved {
		t.Fatalf("method=%q path=%q body=%#v result=%+v", gotMethod, gotPath, gotBody, got)
	}
}
