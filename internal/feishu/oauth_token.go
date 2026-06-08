package feishu

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// UserTokenRefresher coordinates access to stored user credentials and refreshes
// expired OAuth user tokens at most once per credential ID within this process.
type UserTokenRefresher struct {
	client *HTTPClient
	store  TokenStore

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func NewUserTokenRefresher(client *HTTPClient, store TokenStore) *UserTokenRefresher {
	return &UserTokenRefresher{client: client, store: store, locks: map[string]*sync.Mutex{}}
}

func (r *UserTokenRefresher) Credential(ctx context.Context, credentialID string) (CredentialBinding, error) {
	if r == nil || r.store == nil || r.client == nil {
		return CredentialBinding{}, newError(ErrInvalidInput, "user token refresher requires oauth client and token store", nil)
	}
	binding, err := r.store.Get(ctx, credentialID)
	if err != nil {
		return CredentialBinding{}, err
	}
	if !binding.IsExpired(time.Now()) {
		return binding, nil
	}

	lock := r.lockFor(credentialID)
	lock.Lock()
	defer lock.Unlock()

	binding, err = r.store.Get(ctx, credentialID)
	if err != nil {
		return CredentialBinding{}, err
	}
	if !binding.IsExpired(time.Now()) {
		return binding, nil
	}
	refreshed, err := r.client.RefreshUserToken(ctx, binding)
	if err != nil {
		return CredentialBinding{}, err
	}
	if err := r.store.Save(ctx, refreshed); err != nil {
		return CredentialBinding{}, err
	}
	return refreshed, nil
}

func (r *UserTokenRefresher) lockFor(credentialID string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	lock, ok := r.locks[credentialID]
	if !ok {
		lock = &sync.Mutex{}
		r.locks[credentialID] = lock
	}
	return lock
}

type oauthTokenResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data oauthTokenData `json:"data"`
}

type oauthTokenData struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int64          `json:"expires_in"`
	Scope        oauthScopeList `json:"scope"`
	OpenID       string         `json:"open_id"`
	UserID       string         `json:"user_id"`
	TenantKey    string         `json:"tenant_key"`
}

type oauthScopeList []string

func (s *oauthScopeList) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		*s = strings.Fields(text)
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		*s = cleanScopes(values)
		return nil
	}
	return nil
}

func (c *HTTPClient) ExchangeOAuthCode(ctx context.Context, code, redirectURI string) (CredentialBinding, error) {
	code = strings.TrimSpace(code)
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" {
		return CredentialBinding{}, newError(ErrInvalidInput, "oauth code is required", nil)
	}
	if redirectURI == "" {
		return CredentialBinding{}, newError(ErrInvalidInput, "oauth redirect URI is required", nil)
	}
	if c.appID == "" || c.appSecret == "" {
		return CredentialBinding{}, newError(ErrAuthRequired, "oauth app credentials are required", nil)
	}

	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURI,
		"client_id":     c.appID,
		"client_secret": c.appSecret,
		"app_id":        c.appID,
		"app_secret":    c.appSecret,
	}
	data, err := c.doOAuthTokenRequest(ctx, c.oauthTokenPath, payload)
	if err != nil {
		return CredentialBinding{}, err
	}
	if data.AccessToken == "" {
		return CredentialBinding{}, newError(ErrAuthRequired, "oauth token response missing access token", nil)
	}
	id := newCredentialID(data.UserID, data.OpenID)
	return CredentialBinding{ID: id, Provider: c.provider, AuthType: AuthTypeUser, TenantKey: data.TenantKey, UserID: data.UserID, OpenID: data.OpenID, AccessToken: data.AccessToken, RefreshToken: data.RefreshToken, ExpiresAt: expiryFromSeconds(data.ExpiresIn), Scopes: []string(data.Scope)}, nil
}

func (c *HTTPClient) RefreshUserToken(ctx context.Context, binding CredentialBinding) (CredentialBinding, error) {
	if strings.TrimSpace(binding.RefreshToken) == "" {
		return CredentialBinding{}, newError(ErrAuthRequired, "refresh token is required", nil)
	}
	if c.appID == "" || c.appSecret == "" {
		return CredentialBinding{}, newError(ErrAuthRequired, "oauth app credentials are required", nil)
	}
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": binding.RefreshToken,
		"client_id":     c.appID,
		"client_secret": c.appSecret,
		"app_id":        c.appID,
		"app_secret":    c.appSecret,
	}
	data, err := c.doOAuthTokenRequest(ctx, c.oauthRefreshPath, payload)
	if err != nil {
		return CredentialBinding{}, err
	}
	if data.AccessToken == "" {
		return CredentialBinding{}, newError(ErrAuthRequired, "oauth refresh response missing access token", nil)
	}
	updated := binding
	updated.AccessToken = data.AccessToken
	if data.RefreshToken != "" {
		updated.RefreshToken = data.RefreshToken
	}
	updated.ExpiresAt = expiryFromSeconds(data.ExpiresIn)
	if len(data.Scope) > 0 {
		updated.Scopes = []string(data.Scope)
	}
	return updated, nil
}

func (c *HTTPClient) doOAuthTokenRequest(ctx context.Context, path string, payload map[string]string) (oauthTokenData, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return oauthTokenData{}, newError(ErrInvalidInput, "failed to encode oauth request body", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urlFor(path, nil), bytes.NewReader(body))
	if err != nil {
		return oauthTokenData{}, newError(ErrInvalidInput, "failed to create oauth request", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return oauthTokenData{}, newError(ErrUpstream, "oauth token request failed", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthTokenData{}, newError(ErrUpstream, "failed to read oauth token response", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthTokenData{}, newError(ErrAuthRequired, fmt.Sprintf("oauth token endpoint returned HTTP %d", resp.StatusCode), nil)
	}
	var out oauthTokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return oauthTokenData{}, newError(ErrUpstream, "failed to decode oauth token response", err)
	}
	if out.Code != 0 {
		return oauthTokenData{}, newError(ErrAuthRequired, fmt.Sprintf("oauth token endpoint returned code=%d", out.Code), nil)
	}
	return out.Data, nil
}

func expiryFromSeconds(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

func newCredentialID(userID, openID string) string {
	principal := strings.TrimSpace(userID)
	if principal == "" {
		principal = strings.TrimSpace(openID)
	}
	if principal != "" {
		return "user:" + principal
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "user:" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("user:%d", time.Now().UnixNano())
}
