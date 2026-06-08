package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/feishu"
)

func TestToolsIncludesOAuthAuthURL(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(config.Config{})}.Tools()
	for _, tool := range tools {
		if tool.Name == "feishu_oauth_auth_url" {
			return
		}
	}
	t.Fatalf("feishu_oauth_auth_url not found in tools: %#v", tools)
}

func TestOAuthAuthURLCallReturnsResult(t *testing.T) {
	cfg := testOAuthToolConfig()
	tools := FeishuTools{Service: feishu.NewService(cfg)}

	args := json.RawMessage([]byte("{\"state\":\"state-123\"}"))
	got, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	result, ok := got.(feishu.OAuthAuthURLResult)
	if !ok {
		t.Fatalf("result type = %T, want feishu.OAuthAuthURLResult", got)
	}
	if result.Provider != feishu.ProviderFeishu {
		t.Fatalf("Provider = %q", result.Provider)
	}
	if result.RedirectURI != "https://example.test/default/callback" {
		t.Fatalf("RedirectURI = %q", result.RedirectURI)
	}
	if !reflect.DeepEqual(result.Scopes, []string{"offline_access", "docs:doc:readonly"}) {
		t.Fatalf("Scopes = %#v", result.Scopes)
	}
	parsed, err := url.Parse(result.URL)
	if err != nil {
		t.Fatalf("URL did not parse: %v", err)
	}
	if parsed.Query().Get("state") != "state-123" {
		t.Fatalf("state param = %q", parsed.Query().Get("state"))
	}
	if parsed.Query().Get("redirect_uri") != cfg.OAuthRedirectURI {
		t.Fatalf("redirect_uri param = %q", parsed.Query().Get("redirect_uri"))
	}
}

func TestOAuthAuthURLResultDoesNotExposeAppSecretWhenJSONMarshaled(t *testing.T) {
	const sentinelAppSecret = "sentinel-app-secret-must-not-leak"
	cfg := testOAuthToolConfig()
	cfg.AppSecret = sentinelAppSecret
	tools := FeishuTools{Service: feishu.NewService(cfg)}

	got, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", json.RawMessage([]byte(`{}`)))
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	marshaled, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	resultJSON := string(marshaled)
	if strings.Contains(resultJSON, sentinelAppSecret) {
		t.Fatalf("marshaled OAuth auth URL result exposed app secret %q: %s", sentinelAppSecret, resultJSON)
	}
	for _, forbiddenName := range []string{"AppSecret", "appSecret", "app_secret"} {
		if strings.Contains(resultJSON, forbiddenName) {
			t.Fatalf("marshaled OAuth auth URL result exposed app secret field name %q: %s", forbiddenName, resultJSON)
		}
	}
}

func TestOAuthAuthURLSchemaStateMaxLength(t *testing.T) {
	props := oauthAuthURLTool(t).InputSchema["properties"].(map[string]any)
	state := props["state"].(map[string]any)
	if got := state["maxLength"]; got != 256 {
		t.Fatalf("state maxLength = %#v, want 256", got)
	}
}

func TestOAuthAuthURLSchemaRedirectURIMaxLength(t *testing.T) {
	props := oauthAuthURLTool(t).InputSchema["properties"].(map[string]any)
	redirectURI := props["redirectUri"].(map[string]any)
	if got := redirectURI["maxLength"]; got != 2048 {
		t.Fatalf("redirectUri maxLength = %#v, want 2048", got)
	}
}

func TestOAuthAuthURLSchemaDisallowsAdditionalProperties(t *testing.T) {
	if got := oauthAuthURLTool(t).InputSchema["additionalProperties"]; got != false {
		t.Fatalf("additionalProperties = %#v, want false", got)
	}
}

func TestOAuthAuthURLSchemaHasScopeItemLimits(t *testing.T) {
	oauthTool := oauthAuthURLTool(t)
	props := oauthTool.InputSchema["properties"].(map[string]any)
	scopes := props["scopes"].(map[string]any)
	if got := scopes["maxItems"]; got != 20 {
		t.Fatalf("scopes maxItems = %#v, want 20", got)
	}
	items := scopes["items"].(map[string]any)
	if got := items["maxLength"]; got != 256 {
		t.Fatalf("scope item maxLength = %#v, want 256", got)
	}
}

func TestOAuthAuthURLValidationRejectsScopeMaxItems(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	scopes := make([]string, 21)
	for i := range scopes {
		scopes[i] = "offline_access"
	}
	args, err := json.Marshal(map[string]any{"scopes": scopes})
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	_, err = tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected validation error for too many scopes")
	}
}

func TestOAuthAuthURLValidationRejectsScopeItemMaxLength(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args, err := json.Marshal(map[string]any{"scopes": []string{strings.Repeat("a", 257)}})
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	_, err = tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected validation error for overlong scope")
	}
}

func TestOAuthAuthURLValidationRejectsScopeEmbeddedWhitespace(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args := json.RawMessage([]byte("{\"scopes\":[\"offline_access docs:doc:readonly\"]}"))
	_, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected validation error for embedded whitespace")
	}
}

func TestOAuthAuthURLValidationRejectsUnknownArgument(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args := json.RawMessage([]byte("{\"state\":\"state-123\",\"unexpected\":true}"))
	_, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected unknown argument error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %q, want unknown field error", err.Error())
	}
}

func TestOAuthAuthURLValidationRejectsRedirectURIMismatchWhenConfigured(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args := json.RawMessage([]byte("{\"redirectUri\":\"https://evil.example/callback\"}"))
	_, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected redirect URI mismatch error")
	}
}

func TestOAuthAuthURLValidationRejectsCallerRedirectURIWhenNoneConfigured(t *testing.T) {
	cfg := testOAuthToolConfig()
	cfg.OAuthRedirectURI = ""
	tools := FeishuTools{Service: feishu.NewService(cfg)}
	args := json.RawMessage([]byte("{\"redirectUri\":\"https://caller.example/callback\"}"))
	_, err := tools.CallTool(context.Background(), "feishu_oauth_auth_url", args)
	if err == nil {
		t.Fatal("expected redirect URI configuration error")
	}
}

func TestFeishuDocReadAcceptsCredentialID(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args := json.RawMessage([]byte(`{"input":"doc-token","credentialId":"cred-1"}`))
	_, err := tools.CallTool(context.Background(), "feishu_doc_read", args)
	if err == nil {
		t.Fatal("expected upstream/auth error after credentialId decodes")
	}
	if strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("credentialId was rejected as an unknown field: %v", err)
	}
}

func TestFeishuDocReadRejectsOverlongCredentialID(t *testing.T) {
	tools := FeishuTools{Service: feishu.NewService(testOAuthToolConfig())}
	args, err := json.Marshal(map[string]any{"input": "doc-token", "credentialId": strings.Repeat("a", 129)})
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	_, err = tools.CallTool(context.Background(), "feishu_doc_read", args)
	if err == nil || !strings.Contains(err.Error(), "credentialId exceeds max length 128") {
		t.Fatalf("error = %v, want credentialId max length error", err)
	}
}

func TestReadToolSchemaIncludesCredentialID(t *testing.T) {
	props := toolByName(t, "feishu_doc_read").InputSchema["properties"].(map[string]any)
	credentialID := props["credentialId"].(map[string]any)
	if got := credentialID["maxLength"]; got != 128 {
		t.Fatalf("credentialId maxLength = %#v, want 128", got)
	}
}

func testOAuthToolConfig() config.Config {
	return config.Config{
		Provider:           "feishu",
		BaseURL:            "https://open.feishu.cn",
		AppID:              "cli_test",
		OAuthRedirectURI:   "https://example.test/default/callback",
		OAuthScopes:        []string{"offline_access", "docs:doc:readonly"},
		OAuthAuthPath:      "/open-apis/authen/v1/authorize",
		APITimeout:         1,
		APIMaxRetries:      1,
		DocMaxBlocks:       10,
		DocMaxDepth:        2,
		WriteDryRunDefault: true,
	}
}

func oauthAuthURLTool(t *testing.T) *Tool {
	t.Helper()
	return toolByName(t, "feishu_oauth_auth_url")
}

func toolByName(t *testing.T, name string) *Tool {
	t.Helper()
	tools := FeishuTools{Service: feishu.NewService(config.Config{})}.Tools()
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	t.Fatalf("%s not found", name)
	return nil
}
