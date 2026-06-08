package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
)

func TestServiceGetMetadataWithActorUsesActorCredential(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"document": map[string]any{"document_id": "doc-token", "title": "Doc"}},
		})
	}))
	defer server.Close()

	svc := NewService(config.Config{
		Provider:                 "feishu",
		BaseURL:                  server.URL,
		TenantAccessToken:        "tenant-token",
		DocxMetadataPathTemplate: "/open-apis/docx/v1/documents/%s",
	})
	svc.client.SetTokenSource(UserFirstTokenSource{
		Refresher: NewUserTokenRefresher(svc.client, &memoryTokenStore{bindings: map[string]CredentialBinding{
			"cred-1": {ID: "cred-1", AuthType: AuthTypeUser, AccessToken: "user-token", ExpiresAt: time.Now().Add(time.Hour)},
		}}),
		Tenant: TenantTokenSource{Client: svc.client},
	})

	if _, err := svc.GetMetadataWithActor(context.Background(), "doc-token", ActorContext{CredentialID: "cred-1"}); err != nil {
		t.Fatalf("GetMetadataWithActor returned error: %v", err)
	}
	if gotAuth != "Bearer user-token" {
		t.Fatalf("Authorization = %q, want Bearer user-token", gotAuth)
	}
}

func TestServiceGetMetadataWithCredentialIDRequiresUserTokenStore(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer server.Close()

	svc := NewService(config.Config{
		Provider:                 "feishu",
		BaseURL:                  server.URL,
		TenantAccessToken:        "tenant-token",
		DocxMetadataPathTemplate: "/open-apis/docx/v1/documents/%s",
	})

	_, err := svc.GetMetadataWithActor(context.Background(), "doc-token", ActorContext{CredentialID: "cred-1"})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrAuthRequired {
		t.Fatalf("error = %v, want AUTH_REQUIRED", err)
	}
	if called {
		t.Fatalf("upstream was called with tenant fallback; want fail closed before request")
	}
}

func TestConfiguredTokenStoreRejectsInvalidEncryptionKey(t *testing.T) {
	_, err := newConfiguredTokenStore(config.Config{
		TokenStorePath:  t.TempDir() + "/tokens.json",
		TokenEncryptKey: "short-invalid-key",
	})
	if err == nil {
		t.Fatalf("expected invalid TokenEncryptKey to return error")
	}
}
