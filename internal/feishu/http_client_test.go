package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestGetJSONWithActorAuthorizationUsesUserCredential(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, TenantAccessToken: "tenant-token"})
	client.SetTokenSource(UserFirstTokenSource{
		Refresher: NewUserTokenRefresher(client, &memoryTokenStore{bindings: map[string]CredentialBinding{
			"cred-1": {ID: "cred-1", AuthType: AuthTypeUser, AccessToken: "user-token", ExpiresAt: time.Now().Add(time.Hour)},
		}}),
		Tenant: TenantTokenSource{Client: client},
	})

	var out map[string]any
	if err := client.GetJSONWithActor(context.Background(), "/anything", nil, &out, ActorContext{CredentialID: "cred-1"}); err != nil {
		t.Fatalf("GetJSONWithActor returned error: %v", err)
	}
	if gotAuth != "Bearer user-token" {
		t.Fatalf("Authorization = %q, want Bearer user-token", gotAuth)
	}
}

func TestGetJSONAuthorizationFallsBackToTenantForExistingCodePath(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, TenantAccessToken: "tenant-token"})
	client.SetTokenSource(UserFirstTokenSource{Tenant: TenantTokenSource{Client: client}})

	var out map[string]any
	if err := client.GetJSON(context.Background(), "/anything", url.Values{"q": []string{"v"}}, &out); err != nil {
		t.Fatalf("GetJSON returned error: %v", err)
	}
	if gotAuth != "Bearer tenant-token" {
		t.Fatalf("Authorization = %q, want Bearer tenant-token", gotAuth)
	}
}

func TestUserFirstTokenSourceMissingUserCredentialReturnsAuthRequiredWhenFallbackDisabled(t *testing.T) {
	client := NewHTTPClient(HTTPClientOptions{BaseURL: "https://example.test", TenantAccessToken: "tenant-token"})
	client.SetTokenSource(UserFirstTokenSource{
		Refresher: NewUserTokenRefresher(client, &memoryTokenStore{bindings: map[string]CredentialBinding{}}),
		Tenant:    TenantTokenSource{Client: client},
	})

	var out map[string]any
	err := client.GetJSONWithActor(context.Background(), "/anything", nil, &out, ActorContext{CredentialID: "missing"})
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) || connectorErr.Code != ErrAuthRequired {
		t.Fatalf("error = %v, want AUTH_REQUIRED", err)
	}
}

type memoryTokenStore struct {
	bindings map[string]CredentialBinding
}

func (s *memoryTokenStore) Save(ctx context.Context, binding CredentialBinding) error {
	if s.bindings == nil {
		s.bindings = map[string]CredentialBinding{}
	}
	s.bindings[binding.ID] = binding
	return nil
}

func (s *memoryTokenStore) Get(ctx context.Context, id string) (CredentialBinding, error) {
	binding, ok := s.bindings[id]
	if !ok {
		return CredentialBinding{}, newError(ErrAuthRequired, "credential binding not found", nil)
	}
	return binding, nil
}

func (s *memoryTokenStore) Delete(ctx context.Context, id string) error {
	delete(s.bindings, id)
	return nil
}
