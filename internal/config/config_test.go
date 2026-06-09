package config

import (
	"reflect"
	"testing"
)

func TestLoadOAuthConfigDefaults(t *testing.T) {
	t.Setenv("FEISHU_PROVIDER", "")
	t.Setenv("FEISHU_BASE_URL", "")
	t.Setenv("MCP_ALLOW_UNAUTHENTICATED", "")
	t.Setenv("MCP_ALLOWED_ORIGINS", "")
	t.Setenv("MCP_MAX_BODY_BYTES", "")
	t.Setenv("MCP_MAX_BATCH_REQUESTS", "")
	t.Setenv("FEISHU_OAUTH_REDIRECT_URI", "")
	t.Setenv("FEISHU_OAUTH_SCOPES", "")
	t.Setenv("FEISHU_OAUTH_STATE_SECRET", "")
	t.Setenv("FEISHU_OAUTH_AUTH_PATH", "")
	t.Setenv("FEISHU_OAUTH_TOKEN_PATH", "")
	t.Setenv("FEISHU_OAUTH_REFRESH_PATH", "")
	t.Setenv("FEISHU_TOKEN_STORE_PATH", "")
	t.Setenv("FEISHU_TOKEN_ENCRYPT_KEY", "")

	cfg := Load()

	if cfg.OAuthRedirectURI != "" {
		t.Fatalf("OAuthRedirectURI = %q, want empty", cfg.OAuthRedirectURI)
	}
	wantScopes := []string{"offline_access", "docs:doc:readonly", "docs:doc:write", "drive:drive:readonly"}
	if !reflect.DeepEqual(cfg.OAuthScopes, wantScopes) {
		t.Fatalf("OAuthScopes = %#v, want %#v", cfg.OAuthScopes, wantScopes)
	}
	if cfg.OAuthStateSecret != "" {
		t.Fatalf("OAuthStateSecret = %q, want empty", cfg.OAuthStateSecret)
	}
	if cfg.OAuthAuthPath != "/open-apis/authen/v1/authorize" {
		t.Fatalf("OAuthAuthPath = %q", cfg.OAuthAuthPath)
	}
	if cfg.OAuthTokenPath != "/open-apis/authen/v2/oauth/token" {
		t.Fatalf("OAuthTokenPath = %q", cfg.OAuthTokenPath)
	}
	if cfg.OAuthRefreshPath != "/open-apis/authen/v2/oauth/token" {
		t.Fatalf("OAuthRefreshPath = %q", cfg.OAuthRefreshPath)
	}
	if cfg.TokenStorePath != ".data/feishu_tokens.json" {
		t.Fatalf("TokenStorePath = %q", cfg.TokenStorePath)
	}
	if cfg.TokenEncryptKey != "" {
		t.Fatalf("TokenEncryptKey = %q, want empty", cfg.TokenEncryptKey)
	}
	if cfg.MCPAllowUnauthenticated {
		t.Fatal("MCPAllowUnauthenticated = true, want false by default")
	}
	if len(cfg.MCPAllowedOrigins) != 0 {
		t.Fatalf("MCPAllowedOrigins = %#v, want empty", cfg.MCPAllowedOrigins)
	}
	if cfg.MCPMaxBodyBytes != 16*1024*1024 {
		t.Fatalf("MCPMaxBodyBytes = %d", cfg.MCPMaxBodyBytes)
	}
	if cfg.MCPMaxBatchRequests != 50 {
		t.Fatalf("MCPMaxBatchRequests = %d", cfg.MCPMaxBatchRequests)
	}
}

func TestLoadOAuthScopesTrimsAndDropsEmptyValues(t *testing.T) {
	t.Setenv("FEISHU_OAUTH_SCOPES", " offline_access, ,docs:doc:readonly,, drive:drive:readonly ")

	cfg := Load()

	want := []string{"offline_access", "docs:doc:readonly", "drive:drive:readonly"}
	if !reflect.DeepEqual(cfg.OAuthScopes, want) {
		t.Fatalf("OAuthScopes = %#v, want %#v", cfg.OAuthScopes, want)
	}
}

func TestLoadSkillRegistryConfigParsesDirsAndDefaultsWriteDisabled(t *testing.T) {
	t.Setenv("FEISHU_SKILLS_DIRS", " /opt/skills/a, ,./skills/b ")
	t.Setenv("FEISHU_SKILLS_ENABLE_WRITE", "")

	cfg := Load()

	wantDirs := []string{"/opt/skills/a", "./skills/b"}
	if !reflect.DeepEqual(cfg.SkillDirs, wantDirs) {
		t.Fatalf("SkillDirs = %#v, want %#v", cfg.SkillDirs, wantDirs)
	}
	if cfg.SkillsEnableWrite {
		t.Fatal("SkillsEnableWrite = true, want false by default")
	}
}

func TestLoadSkillRegistryConfigParsesWriteEnabled(t *testing.T) {
	t.Setenv("FEISHU_SKILLS_ENABLE_WRITE", "true")

	cfg := Load()

	if !cfg.SkillsEnableWrite {
		t.Fatal("SkillsEnableWrite = false, want true")
	}
}

func TestLoadMCPSecurityOverrides(t *testing.T) {
	t.Setenv("MCP_ALLOW_UNAUTHENTICATED", "true")
	t.Setenv("MCP_ALLOWED_ORIGINS", " https://chat.openai.com, https://example.test ")
	t.Setenv("MCP_MAX_BODY_BYTES", "4096")
	t.Setenv("MCP_MAX_BATCH_REQUESTS", "3")

	cfg := Load()

	if !cfg.MCPAllowUnauthenticated {
		t.Fatal("MCPAllowUnauthenticated = false, want true")
	}
	wantOrigins := []string{"https://chat.openai.com", "https://example.test"}
	if !reflect.DeepEqual(cfg.MCPAllowedOrigins, wantOrigins) {
		t.Fatalf("MCPAllowedOrigins = %#v, want %#v", cfg.MCPAllowedOrigins, wantOrigins)
	}
	if cfg.MCPMaxBodyBytes != 4096 {
		t.Fatalf("MCPMaxBodyBytes = %d", cfg.MCPMaxBodyBytes)
	}
	if cfg.MCPMaxBatchRequests != 3 {
		t.Fatalf("MCPMaxBatchRequests = %d", cfg.MCPMaxBatchRequests)
	}
}

func TestValidateRemoteMCPSecurityRejectsPlaintextTokenStore(t *testing.T) {
	cfg := Config{TokenStorePath: ".data/feishu_tokens.json", TokenEncryptKey: ""}
	if err := cfg.ValidateRemoteMCPSecurity(); err == nil {
		t.Fatal("expected plaintext token store to be rejected for remote MCP")
	}
}

func TestValidateRemoteMCPSecurityAllowsDisabledTokenStore(t *testing.T) {
	cfg := Config{TokenStorePath: "", TokenEncryptKey: ""}
	if err := cfg.ValidateRemoteMCPSecurity(); err != nil {
		t.Fatalf("ValidateRemoteMCPSecurity returned error: %v", err)
	}
}

func TestValidateRemoteMCPSecurityAllowsEncryptedTokenStore(t *testing.T) {
	cfg := Config{TokenStorePath: ".data/feishu_tokens.json", TokenEncryptKey: "12345678901234567890123456789012"}
	if err := cfg.ValidateRemoteMCPSecurity(); err != nil {
		t.Fatalf("ValidateRemoteMCPSecurity returned error: %v", err)
	}
}

func TestLoadOAuthConfigOverrides(t *testing.T) {
	t.Setenv("FEISHU_OAUTH_REDIRECT_URI", " https://example.test/oauth/callback ")
	t.Setenv("FEISHU_OAUTH_SCOPES", "offline_access,docs:doc:readonly")
	t.Setenv("FEISHU_OAUTH_STATE_SECRET", "state-secret")
	t.Setenv("FEISHU_OAUTH_AUTH_PATH", "/custom/auth")
	t.Setenv("FEISHU_OAUTH_TOKEN_PATH", "/custom/token")
	t.Setenv("FEISHU_OAUTH_REFRESH_PATH", "/custom/refresh")
	t.Setenv("FEISHU_TOKEN_STORE_PATH", "/tmp/tokens.json")
	t.Setenv("FEISHU_TOKEN_ENCRYPT_KEY", "encrypt-key")

	cfg := Load()

	if cfg.OAuthRedirectURI != "https://example.test/oauth/callback" {
		t.Fatalf("OAuthRedirectURI = %q", cfg.OAuthRedirectURI)
	}
	wantScopes := []string{"offline_access", "docs:doc:readonly"}
	if !reflect.DeepEqual(cfg.OAuthScopes, wantScopes) {
		t.Fatalf("OAuthScopes = %#v, want %#v", cfg.OAuthScopes, wantScopes)
	}
	checks := map[string]string{
		"OAuthStateSecret": cfg.OAuthStateSecret,
		"OAuthAuthPath":    cfg.OAuthAuthPath,
		"OAuthTokenPath":   cfg.OAuthTokenPath,
		"OAuthRefreshPath": cfg.OAuthRefreshPath,
		"TokenStorePath":   cfg.TokenStorePath,
		"TokenEncryptKey":  cfg.TokenEncryptKey,
	}
	want := map[string]string{
		"OAuthStateSecret": "state-secret",
		"OAuthAuthPath":    "/custom/auth",
		"OAuthTokenPath":   "/custom/token",
		"OAuthRefreshPath": "/custom/refresh",
		"TokenStorePath":   "/tmp/tokens.json",
		"TokenEncryptKey":  "encrypt-key",
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("overrides = %#v, want %#v", checks, want)
	}
}

func TestLoadBaseURLRejectsHTTPFeishuHostAndFallsBack(t *testing.T) {
	t.Setenv("FEISHU_PROVIDER", "")
	t.Setenv("FEISHU_BASE_URL", "http://open.feishu.cn")

	cfg := Load()

	if cfg.BaseURL != defaultFeishuBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultFeishuBaseURL)
	}
}

func TestLoadBaseURLRejectsHTTPLarkHostAndFallsBack(t *testing.T) {
	t.Setenv("FEISHU_PROVIDER", "lark")
	t.Setenv("FEISHU_BASE_URL", "http://open.larksuite.com")

	cfg := Load()

	if cfg.BaseURL != defaultLarkBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultLarkBaseURL)
	}
}

func TestLoadBaseURLAcceptsHTTPLocalhost(t *testing.T) {
	t.Setenv("FEISHU_PROVIDER", "")
	t.Setenv("FEISHU_BASE_URL", "http://localhost:9999")

	cfg := Load()

	if cfg.BaseURL != "http://localhost:9999" {
		t.Fatalf("BaseURL = %q, want http://localhost:9999", cfg.BaseURL)
	}
}

func TestLoadBaseURLRejectsMalformedAndFallsBack(t *testing.T) {
	t.Setenv("FEISHU_PROVIDER", "")
	t.Setenv("FEISHU_BASE_URL", "https:///open.feishu.cn")

	cfg := Load()

	if cfg.BaseURL != defaultFeishuBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultFeishuBaseURL)
	}
}
