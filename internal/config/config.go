package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultFeishuBaseURL = "https://open.feishu.cn"
	defaultLarkBaseURL   = "https://open.larksuite.com"
)

type Config struct {
	Provider                       string
	BaseURL                        string
	AppID                          string
	AppSecret                      string
	TenantAccessToken              string
	APITimeout                     time.Duration
	APIMaxRetries                  int
	DocMaxBlocks                   int
	DocMaxDepth                    int
	WriteDryRunDefault             bool
	DocxMetadataPathTemplate       string
	DocxChildrenPathTemplate       string
	DocxAppendChildrenPathTemplate string
	DocxPermissionPathTemplate     string
	FolderPermissionPathTemplate   string
	DocxCommentsPathTemplate       string
	DocxCommentRepliesPathTemplate string
	DocxCommentResolvePathTemplate string
	WikiNodePathTemplate           string
	DriveFileMetadataPathTemplate  string
	DocxCreatePath                 string
	MCPHTTPAddr                    string
	MCPServerAPIKey                string
	MCPAllowUnauthenticated        bool
	MCPAllowedOrigins              []string
	MCPMaxBodyBytes                int
	MCPMaxBatchRequests            int
	OAuthRedirectURI               string
	OAuthScopes                    []string
	OAuthStateSecret               string
	OAuthAuthPath                  string
	OAuthTokenPath                 string
	OAuthRefreshPath               string
	TokenStorePath                 string
	TokenEncryptKey                string
	SkillDirs                      []string
	// SkillsEnableWrite is the global safety gate for write-capable skills and real dryRun=false skill mutations.
	SkillsEnableWrite bool
}

func Load() Config {
	provider := strings.ToLower(getenv("FEISHU_PROVIDER", "feishu"))
	defaultBase := defaultFeishuBaseURL
	if provider == "lark" {
		defaultBase = defaultLarkBaseURL
	}

	return Config{
		Provider:                       provider,
		BaseURL:                        safeBaseURL(getenv("FEISHU_BASE_URL", defaultBase), defaultBase),
		AppID:                          os.Getenv("FEISHU_APP_ID"),
		AppSecret:                      os.Getenv("FEISHU_APP_SECRET"),
		TenantAccessToken:              os.Getenv("FEISHU_TENANT_ACCESS_TOKEN"),
		APITimeout:                     time.Duration(getenvInt("FEISHU_API_TIMEOUT_MS", 15000)) * time.Millisecond,
		APIMaxRetries:                  getenvInt("FEISHU_API_MAX_RETRIES", 3),
		DocMaxBlocks:                   getenvInt("FEISHU_DOC_MAX_BLOCKS", 3000),
		DocMaxDepth:                    getenvInt("FEISHU_DOC_MAX_DEPTH", 20),
		WriteDryRunDefault:             getenvBool("FEISHU_DOC_WRITE_DRY_RUN_DEFAULT", true),
		DocxMetadataPathTemplate:       getenv("FEISHU_DOCX_METADATA_PATH_TEMPLATE", "/open-apis/docx/v1/documents/%s"),
		DocxChildrenPathTemplate:       getenv("FEISHU_DOCX_CHILDREN_PATH_TEMPLATE", "/open-apis/docx/v1/documents/%s/blocks/%s/children"),
		DocxAppendChildrenPathTemplate: getenv("FEISHU_DOCX_APPEND_CHILDREN_PATH_TEMPLATE", "/open-apis/docx/v1/documents/%s/blocks/%s/children"),
		DocxPermissionPathTemplate:     getenv("FEISHU_DOCX_PERMISSION_PATH_TEMPLATE", "/open-apis/drive/v1/permissions/%s/public?type=docx"),
		FolderPermissionPathTemplate:   getenv("FEISHU_FOLDER_PERMISSION_PATH_TEMPLATE", "/open-apis/drive/v1/permissions/%s/public?type=folder"),
		DocxCommentsPathTemplate:       getenv("FEISHU_DOCX_COMMENTS_PATH_TEMPLATE", "/open-apis/drive/v1/files/%s/comments"),
		DocxCommentRepliesPathTemplate: getenv("FEISHU_DOCX_COMMENT_REPLIES_PATH_TEMPLATE", "/open-apis/drive/v1/files/%s/comments/%s/replies"),
		DocxCommentResolvePathTemplate: getenv("FEISHU_DOCX_COMMENT_RESOLVE_PATH_TEMPLATE", "/open-apis/drive/v1/files/%s/comments/%s"),
		WikiNodePathTemplate:           getenv("FEISHU_WIKI_NODE_PATH_TEMPLATE", "/open-apis/wiki/v2/spaces/get_node?token=%s"),
		DriveFileMetadataPathTemplate:  getenv("FEISHU_DRIVE_FILE_METADATA_PATH_TEMPLATE", "/open-apis/drive/v1/files/%s"),
		DocxCreatePath:                 getenv("FEISHU_DOCX_CREATE_PATH", "/open-apis/docx/v1/documents"),
		MCPHTTPAddr:                    getenv("MCP_HTTP_ADDR", ":8080"),
		MCPServerAPIKey:                os.Getenv("MCP_SERVER_API_KEY"),
		MCPAllowUnauthenticated:        getenvBool("MCP_ALLOW_UNAUTHENTICATED", false),
		MCPAllowedOrigins:              getenvList("MCP_ALLOWED_ORIGINS", ""),
		MCPMaxBodyBytes:                getenvInt("MCP_MAX_BODY_BYTES", 16*1024*1024),
		MCPMaxBatchRequests:            getenvInt("MCP_MAX_BATCH_REQUESTS", 50),
		OAuthRedirectURI:               getenv("FEISHU_OAUTH_REDIRECT_URI", ""),
		OAuthScopes:                    getenvList("FEISHU_OAUTH_SCOPES", "offline_access,docs:doc:readonly,docs:doc:write,drive:drive:readonly"),
		OAuthStateSecret:               getenv("FEISHU_OAUTH_STATE_SECRET", ""),
		OAuthAuthPath:                  getenv("FEISHU_OAUTH_AUTH_PATH", "/open-apis/authen/v1/authorize"),
		OAuthTokenPath:                 getenv("FEISHU_OAUTH_TOKEN_PATH", "/open-apis/authen/v2/oauth/token"),
		OAuthRefreshPath:               getenv("FEISHU_OAUTH_REFRESH_PATH", "/open-apis/authen/v2/oauth/token"),
		TokenStorePath:                 getenv("FEISHU_TOKEN_STORE_PATH", ".data/feishu_tokens.json"),
		TokenEncryptKey:                getenv("FEISHU_TOKEN_ENCRYPT_KEY", ""),
		SkillDirs:                      getenvList("FEISHU_SKILLS_DIRS", ""),
		SkillsEnableWrite:              getenvBool("FEISHU_SKILLS_ENABLE_WRITE", false),
	}
}

func (c Config) ValidateRemoteMCPSecurity() error {
	if strings.TrimSpace(c.TokenStorePath) == "" {
		return nil
	}
	key := strings.TrimSpace(c.TokenEncryptKey)
	if key == "" {
		return fmt.Errorf("FEISHU_TOKEN_ENCRYPT_KEY is required when FEISHU_TOKEN_STORE_PATH is enabled for remote MCP")
	}
	if len([]byte(key)) != 16 && len([]byte(key)) != 24 && len([]byte(key)) != 32 {
		return fmt.Errorf("FEISHU_TOKEN_ENCRYPT_KEY must be 16, 24, or 32 bytes when configured for remote MCP")
	}
	return nil
}

func safeBaseURL(raw, fallback string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fallback
	}
	if !isSafeBaseURL(parsed) {
		return fallback
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func isSafeBaseURL(parsed *url.URL) bool {
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "https" {
		return true
	}
	return scheme == "http" && isLocalHost(parsed.Hostname())
}

func isLocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvList(key, fallback string) []string {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		value = fallback
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
