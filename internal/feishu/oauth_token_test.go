package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientExchangeOAuthCodeMapsCredentialBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		assertRequestField(t, body, "grant_type", "authorization_code")
		assertRequestField(t, body, "code", "code-123")
		assertRequestField(t, body, "redirect_uri", "https://example.test/callback")
		assertRequestField(t, body, "client_id", "app-1")
		assertRequestField(t, body, "client_secret", "secret-1")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"access_token":"access-1","refresh_token":"refresh-1","expires_in":7200,"scope":"offline_access docs:doc:readonly","open_id":"open-1","user_id":"user-1","tenant_key":"tenant-1"}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthTokenPath: "/oauth/token"})
	before := time.Now()
	binding, err := client.ExchangeOAuthCode(context.Background(), "code-123", "https://example.test/callback")
	if err != nil {
		t.Fatalf("ExchangeOAuthCode returned error: %v", err)
	}
	if binding.ID == "" {
		t.Fatal("expected generated credential ID")
	}
	if binding.Provider != ProviderFeishu {
		t.Fatalf("provider = %s, want %s", binding.Provider, ProviderFeishu)
	}
	if binding.AuthType != AuthTypeUser {
		t.Fatalf("auth type = %s, want %s", binding.AuthType, AuthTypeUser)
	}
	if binding.UserID != "user-1" || binding.OpenID != "open-1" || binding.TenantKey != "tenant-1" {
		t.Fatalf("unexpected binding identity fields for %s", redactedCredentialSummary(binding))
	}
	if binding.AccessToken != "access-1" {
		t.Fatal("access token was not mapped from OAuth response")
	}
	if binding.RefreshToken != "refresh-1" {
		t.Fatal("refresh token was not mapped from OAuth response")
	}
	if strings.Join(binding.Scopes, ",") != "offline_access,docs:doc:readonly" {
		t.Fatalf("scopes = %q", strings.Join(binding.Scopes, ","))
	}
	if binding.ExpiresAt.Before(before.Add(7190*time.Second)) || binding.ExpiresAt.After(time.Now().Add(7210*time.Second)) {
		t.Fatalf("ExpiresAt outside expected range: %s", binding.ExpiresAt)
	}
}

func TestHTTPClientRefreshUserTokenPreservesIdentityAndUpdatesReturnedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/refresh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		assertRequestField(t, body, "grant_type", "refresh_token")
		assertRequestField(t, body, "refresh_token", "old-refresh")
		assertRequestField(t, body, "client_id", "app-1")
		assertRequestField(t, body, "client_secret", "secret-1")
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"scope":["offline_access","docs:doc:write"]}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthRefreshPath: "/oauth/refresh"})
	original := CredentialBinding{ID: "cred-1", Provider: ProviderLark, AuthType: AuthTypeUser, TenantKey: "tenant-1", UserID: "user-1", OpenID: "open-1", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"old"}}
	updated, err := client.RefreshUserToken(context.Background(), original)
	if err != nil {
		t.Fatalf("RefreshUserToken returned error: %v", err)
	}
	if updated.ID != original.ID || updated.Provider != original.Provider || updated.AuthType != original.AuthType || updated.TenantKey != original.TenantKey || updated.UserID != original.UserID || updated.OpenID != original.OpenID {
		t.Fatalf("identity was not preserved for %s", redactedCredentialSummary(updated))
	}
	if updated.AccessToken != "new-access" {
		t.Fatal("access token was not updated from refresh response")
	}
	if updated.RefreshToken != "new-refresh" {
		t.Fatal("refresh token was not updated from refresh response")
	}
	if strings.Join(updated.Scopes, ",") != "offline_access,docs:doc:write" {
		t.Fatalf("scopes = %q", strings.Join(updated.Scopes, ","))
	}
}

func TestHTTPClientRefreshUserTokenPreservesRefreshAndScopesWhenOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"new-access","expires_in":1800}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthRefreshPath: "/oauth/refresh"})
	original := CredentialBinding{ID: "cred-1", Provider: ProviderFeishu, AuthType: AuthTypeUser, UserID: "user-1", AccessToken: "old-access", RefreshToken: "old-refresh", Scopes: []string{"offline_access"}}
	updated, err := client.RefreshUserToken(context.Background(), original)
	if err != nil {
		t.Fatalf("RefreshUserToken returned error: %v", err)
	}
	if updated.RefreshToken != "old-refresh" {
		t.Fatal("refresh token should be preserved when refresh response omits it")
	}
	if strings.Join(updated.Scopes, ",") != "offline_access" {
		t.Fatalf("Scopes = %q", strings.Join(updated.Scopes, ","))
	}
}

func TestHTTPClientOAuthNonZeroCodeReturnsAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":99991663,"msg":"invalid authorization code"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthTokenPath: "/oauth/token"})
	_, err := client.ExchangeOAuthCode(context.Background(), "bad-code", "https://example.test/callback")
	if err == nil {
		t.Fatal("expected OAuth error")
	}
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) {
		t.Fatalf("error = %T %v, want ConnectorError", err, err)
	}
	if connectorErr.Code != ErrAuthRequired {
		t.Fatalf("error code = %s, want %s", connectorErr.Code, ErrAuthRequired)
	}
	if !strings.Contains(connectorErr.Error(), "code=99991663") {
		t.Fatalf("error should include upstream numeric code only, got: %v", connectorErr)
	}
}

func TestHTTPClientOAuthNonZeroCodeDoesNotLeakUpstreamMsgSecrets(t *testing.T) {
	leakedSecret := "fake-access-token-secret-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":99991663,"msg":"invalid token ` + leakedSecret + ` refresh-secret-456"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthTokenPath: "/oauth/token"})
	_, err := client.ExchangeOAuthCode(context.Background(), "bad-code", "https://example.test/callback")
	if err == nil {
		t.Fatal("expected OAuth error")
	}
	msg := err.Error()
	if strings.Contains(msg, leakedSecret) || strings.Contains(msg, "refresh-secret-456") || strings.Contains(msg, "invalid token") {
		t.Fatal("OAuth error leaked upstream message content")
	}
	if !strings.Contains(msg, "code=99991663") {
		t.Fatal("OAuth error should retain numeric upstream code")
	}
}

func TestUserTokenRefresherConcurrentExpiredCredentialRefreshesOnce(t *testing.T) {
	var refreshCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&refreshCalls, 1)
		if r.URL.Path != "/oauth/refresh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		assertRequestField(t, body, "grant_type", "refresh_token")
		assertRequestField(t, body, "refresh_token", "old-refresh")
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"shared-new-access","refresh_token":"shared-new-refresh","expires_in":3600,"scope":["offline_access"]}}`))
	}))
	defer server.Close()

	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	expired := CredentialBinding{ID: "cred-expired", Provider: ProviderFeishu, AuthType: AuthTypeUser, UserID: "user-1", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"offline_access"}}
	if err := store.Save(context.Background(), expired); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthRefreshPath: "/oauth/refresh"})
	refresher := NewUserTokenRefresher(client, store)

	const workers = 25
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			binding, err := refresher.Credential(context.Background(), expired.ID)
			if err != nil {
				errs <- err
				return
			}
			if binding.AccessToken != "shared-new-access" {
				errs <- newError(ErrUpstream, "worker received unexpected access token", nil)
				return
			}
			if binding.RefreshToken != "shared-new-refresh" {
				errs <- newError(ErrUpstream, "worker received unexpected refresh token", nil)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Credential returned error: %v", err)
		}
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh HTTP calls = %d, want 1", got)
	}
	stored, err := store.Get(context.Background(), expired.ID)
	if err != nil {
		t.Fatalf("Get refreshed credential returned error: %v", err)
	}
	if stored.AccessToken != "shared-new-access" {
		t.Fatal("refreshed access token was not saved")
	}
}

func TestUserTokenRefresherNonExpiredCredentialDoesNotRefresh(t *testing.T) {
	var refreshCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&refreshCalls, 1)
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"unexpected","expires_in":3600}}`))
	}))
	defer server.Close()

	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	want := CredentialBinding{ID: "cred-fresh", Provider: ProviderFeishu, AuthType: AuthTypeUser, UserID: "user-1", AccessToken: "fresh-access", RefreshToken: "fresh-refresh", ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{"offline_access"}}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	client := NewHTTPClient(HTTPClientOptions{BaseURL: server.URL, AppID: "app-1", AppSecret: "secret-1", OAuthRefreshPath: "/oauth/refresh"})
	got, err := NewUserTokenRefresher(client, store).Credential(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("Credential returned error: %v", err)
	}
	if got.AccessToken != want.AccessToken {
		t.Fatal("non-expired credential access token changed")
	}
	if calls := atomic.LoadInt32(&refreshCalls); calls != 0 {
		t.Fatalf("refresh HTTP calls = %d, want 0", calls)
	}
}

func TestUserTokenRefresherMissingCredentialReturnsStructuredError(t *testing.T) {
	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	client := NewHTTPClient(HTTPClientOptions{BaseURL: "https://example.invalid", AppID: "app-1", AppSecret: "secret-1"})
	_, err = NewUserTokenRefresher(client, store).Credential(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected missing credential error")
	}
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) {
		t.Fatalf("error = %T %v, want ConnectorError", err, err)
	}
	if connectorErr.Code != ErrAuthRequired {
		t.Fatalf("error code = %s, want %s", connectorErr.Code, ErrAuthRequired)
	}
}

func assertRequestField(t *testing.T, body map[string]any, field, want string) {
	t.Helper()
	if got, _ := body[field].(string); got != want {
		t.Fatalf("request field %s = %s, want %s", field, redactedRequestFieldValue(field, got), redactedRequestFieldValue(field, want))
	}
}

func redactedRequestFieldValue(field, value string) string {
	switch field {
	case "client_secret", "app_secret", "refresh_token", "code":
		if value == "" {
			return "<empty>"
		}
		return "<redacted>"
	default:
		return `"` + value + `"`
	}
}
