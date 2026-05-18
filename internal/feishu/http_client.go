package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPClient struct {
	baseURL     string
	appID       string
	appSecret   string
	staticToken string
	client      *http.Client
	maxRetries  int
	cachedToken string
	tokenExpiry time.Time
}

type HTTPClientOptions struct {
	BaseURL           string
	AppID             string
	AppSecret         string
	TenantAccessToken string
	Timeout           time.Duration
	MaxRetries        int
}

func NewHTTPClient(opts HTTPClientOptions) *HTTPClient {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		appID:       opts.AppID,
		appSecret:   opts.AppSecret,
		staticToken: opts.TenantAccessToken,
		client:      &http.Client{Timeout: timeout},
		maxRetries:  opts.MaxRetries,
	}
}

func (c *HTTPClient) TenantToken(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry.Add(-1*time.Minute)) {
		return c.cachedToken, nil
	}
	if c.appID == "" || c.appSecret == "" {
		return "", newError(ErrAuthRequired, "FEISHU_APP_ID/FEISHU_APP_SECRET or FEISHU_TENANT_ACCESS_TOKEN is required", nil)
	}

	payload := map[string]string{"app_id": c.appID, "app_secret": c.appSecret}
	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int64  `json:"expire"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", nil, payload, &out, false); err != nil {
		return "", err
	}
	if out.Code != 0 || out.TenantAccessToken == "" {
		return "", newError(ErrAuthRequired, fmt.Sprintf("failed to obtain tenant token: code=%d msg=%s", out.Code, out.Msg), nil)
	}
	c.cachedToken = out.TenantAccessToken
	ttl := time.Duration(out.Expire) * time.Second
	if ttl <= 0 {
		ttl = 90 * time.Minute
	}
	c.tokenExpiry = time.Now().Add(ttl)
	return c.cachedToken, nil
}

func (c *HTTPClient) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out, true)
}

func (c *HTTPClient) PostJSON(ctx context.Context, path string, in any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, nil, in, out, true)
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, query url.Values, in any, out any, withAuth bool) error {
	body, err := encodeBody(in)
	if err != nil {
		return newError(ErrInvalidInput, "failed to encode request body", err)
	}

	attempts := c.maxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff(attempt))
		}

		req, err := http.NewRequestWithContext(ctx, method, c.urlFor(path, query), bytes.NewReader(body))
		if err != nil {
			return newError(ErrInvalidInput, "failed to create HTTP request", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		if withAuth {
			token, err := c.TenantToken(ctx)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		err = decodeResponse(resp, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(resp.StatusCode) {
			return err
		}
	}
	return newError(ErrUpstream, "upstream request failed after retries", lastErr)
}

func (c *HTTPClient) urlFor(path string, query url.Values) string {
	u := c.baseURL + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func encodeBody(in any) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(in)
}

func decodeResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return newError(ErrUpstream, "failed to read upstream response", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return newError(ErrPermissionDenied, fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode), nil)
	}
	if resp.StatusCode == http.StatusNotFound {
		return newError(ErrDocumentNotFound, "document not found", nil)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return newError(ErrRateLimited, "upstream rate limited the request", nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newError(ErrUpstream, fmt.Sprintf("upstream returned HTTP %d: %s", resp.StatusCode, string(raw)), nil)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return newError(ErrUpstream, "failed to decode upstream response", err)
	}
	return nil
}

func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(200*(1<<(attempt-1))) * time.Millisecond
	if d > 2*time.Second {
		return 2 * time.Second
	}
	return d
}
