package feishu

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
)

func TestBuildOAuthAuthURLIncludesRequiredParams(t *testing.T) {
	result, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback",
		State:       "state-123",
		Scopes:      []string{"offline_access", "docs:doc:readonly"},
	})
	if err != nil {
		t.Fatalf("BuildOAuthAuthURL returned error: %v", err)
	}

	parsed, err := url.Parse(result.URL)
	if err != nil {
		t.Fatalf("result URL did not parse: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "open.feishu.cn" || parsed.Path != "/open-apis/authen/v1/authorize" {
		t.Fatalf("unexpected URL target: %s", result.URL)
	}
	params := parsed.Query()
	if got := firstNonEmptyParam(params.Get("app_id"), params.Get("client_id")); got != "cli_a123" {
		t.Fatalf("app/client id param = %q", got)
	}
	if got := params.Get("redirect_uri"); got != "https://example.test/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := params.Get("state"); got != "state-123" {
		t.Fatalf("state = %q", got)
	}
	if got := strings.Fields(params.Get("scope")); strings.Join(got, ",") != "offline_access,docs:doc:readonly" {
		t.Fatalf("scope fields = %#v", got)
	}
	if result.Provider != ProviderFeishu {
		t.Fatalf("Provider = %q", result.Provider)
	}
	if result.RedirectURI != "https://example.test/callback" {
		t.Fatalf("RedirectURI = %q", result.RedirectURI)
	}
}

func TestBuildOAuthAuthURLUsesProviderBaseURL(t *testing.T) {
	result, err := BuildOAuthAuthURL(ProviderLark, "https://open.larksuite.com/", "cli_lark", "open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/lark/callback",
		Scopes:      []string{"offline_access"},
	})
	if err != nil {
		t.Fatalf("BuildOAuthAuthURL returned error: %v", err)
	}
	parsed, err := url.Parse(result.URL)
	if err != nil {
		t.Fatalf("result URL did not parse: %v", err)
	}
	if parsed.Host != "open.larksuite.com" {
		t.Fatalf("host = %q", parsed.Host)
	}
	if result.Provider != ProviderLark {
		t.Fatalf("Provider = %q", result.Provider)
	}
}

func TestBuildOAuthAuthURLRejectsMissingRedirectURI(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		Scopes: []string{"offline_access"},
	})
	if err == nil {
		t.Fatal("expected error for missing redirect URI")
	}
}

func TestBuildOAuthAuthURLRejectsHTTPNonLocalBaseURL(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "http://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback",
	})
	if err == nil {
		t.Fatal("expected error for non-HTTPS non-local base URL")
	}
}

func TestBuildOAuthAuthURLRejectsRedirectURIWithoutSchemeHost(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "/callback",
	})
	if err == nil {
		t.Fatal("expected error for redirect URI without scheme and host")
	}
}

func TestBuildOAuthAuthURLRejectsRedirectURIWithFragment(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback#fragment",
	})
	if err == nil {
		t.Fatal("expected error for redirect URI fragment")
	}
}

func TestBuildOAuthAuthURLRejectsHTTPNonLocalRedirectURI(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "http://example.test/callback",
	})
	if err == nil {
		t.Fatal("expected error for non-HTTPS non-local redirect URI")
	}
}

func TestBuildOAuthAuthURLAllowsHTTPLocalhostRedirectURI(t *testing.T) {
	result, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "http://localhost:8080/callback",
	})
	if err != nil {
		t.Fatalf("BuildOAuthAuthURL returned error: %v", err)
	}
	if result.RedirectURI != "http://localhost:8080/callback" {
		t.Fatalf("RedirectURI = %q", result.RedirectURI)
	}
}

func TestBuildOAuthAuthURLRejectsMissingAppID(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", " ", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback",
		Scopes:      []string{"offline_access"},
	})
	if err == nil {
		t.Fatal("expected error for missing app id")
	}
}

func TestBuildOAuthAuthURLRejectsScopesWithEmbeddedWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		scope string
	}{
		{name: "space", scope: "offline_access docs:doc:readonly"},
		{name: "tab", scope: "offline_access\tdocs:doc:readonly"},
		{name: "newline", scope: "offline_access\ndocs:doc:readonly"},
		{name: "control", scope: "offline_access\x00docs:doc:readonly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
				RedirectURI: "https://example.test/callback",
				Scopes:      []string{tt.scope},
			})
			if err == nil {
				t.Fatalf("expected error for invalid scope %q", tt.scope)
			}
		})
	}
}

func TestBuildOAuthAuthURLRejectsScopeLongerThanMaxLength(t *testing.T) {
	_, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback",
		Scopes:      []string{strings.Repeat("a", 257)},
	})
	if err == nil {
		t.Fatal("expected error for scope longer than 256")
	}
}

func TestBuildOAuthAuthURLEncodesSpecialCharacters(t *testing.T) {
	result, err := BuildOAuthAuthURL(ProviderFeishu, "https://open.feishu.cn", "cli_a+123", "/open-apis/authen/v1/authorize", OAuthAuthURLRequest{
		RedirectURI: "https://example.test/callback?next=/docs/a b&x=1+2",
		State:       "state with + plus & ampersand",
		Scopes:      []string{"offline_access", "docs:doc:readonly"},
	})
	if err != nil {
		t.Fatalf("BuildOAuthAuthURL returned error: %v", err)
	}
	parsed, err := url.Parse(result.URL)
	if err != nil {
		t.Fatalf("result URL did not parse: %v", err)
	}
	params := parsed.Query()
	if got := params.Get("app_id"); got != "cli_a+123" {
		t.Fatalf("app_id = %q", got)
	}
	if got := params.Get("redirect_uri"); got != "https://example.test/callback?next=/docs/a b&x=1+2" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := params.Get("state"); got != "state with + plus & ampersand" {
		t.Fatalf("state = %q", got)
	}
}

func TestServiceBuildOAuthAuthURLRejectsRedirectURIMismatchWhenConfigured(t *testing.T) {
	svc := NewService(testOAuthConfig())
	_, err := svc.BuildOAuthAuthURL(OAuthAuthURLRequest{RedirectURI: "https://evil.example/callback"})
	if err == nil {
		t.Fatal("expected redirect URI mismatch error")
	}
}

func TestServiceBuildOAuthAuthURLAllowsMatchingRedirectURIWhenConfigured(t *testing.T) {
	cfg := testOAuthConfig()
	svc := NewService(cfg)
	result, err := svc.BuildOAuthAuthURL(OAuthAuthURLRequest{RedirectURI: " https://example.test/default/callback "})
	if err != nil {
		t.Fatalf("BuildOAuthAuthURL returned error: %v", err)
	}
	if result.RedirectURI != cfg.OAuthRedirectURI {
		t.Fatalf("RedirectURI = %q", result.RedirectURI)
	}
}

func TestServiceBuildOAuthAuthURLRejectsCallerRedirectURIWhenNoneConfigured(t *testing.T) {
	cfg := testOAuthConfig()
	cfg.OAuthRedirectURI = ""
	svc := NewService(cfg)
	_, err := svc.BuildOAuthAuthURL(OAuthAuthURLRequest{RedirectURI: "https://caller.example/callback"})
	if err == nil {
		t.Fatal("expected error for caller redirect URI when no redirect URI is configured")
	}
}

func TestValidateOAuthScopesDropsEmptyValues(t *testing.T) {
	scopes, err := validateOAuthScopes([]string{" offline_access ", "", " \t ", "docs:doc:readonly"})
	if err != nil {
		t.Fatalf("validateOAuthScopes returned error: %v", err)
	}
	want := []string{"offline_access", "docs:doc:readonly"}
	if !reflect.DeepEqual(scopes, want) {
		t.Fatalf("scopes = %#v, want %#v", scopes, want)
	}
}

func TestValidateOAuthScopesReportsInvalidScope(t *testing.T) {
	_, err := validateOAuthScopes([]string{"offline_access docs:doc:readonly"})
	if !errors.Is(err, ErrInvalidOAuthScope) {
		t.Fatalf("error = %v, want ErrInvalidOAuthScope", err)
	}
}

func testOAuthConfig() config.Config {
	return config.Config{
		Provider:         "feishu",
		BaseURL:          "https://open.feishu.cn",
		AppID:            "cli_test",
		OAuthRedirectURI: "https://example.test/default/callback",
		OAuthScopes:      []string{"offline_access", "docs:doc:readonly"},
		OAuthAuthPath:    "/open-apis/authen/v1/authorize",
	}
}

func firstNonEmptyParam(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
