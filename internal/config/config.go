package config

import (
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
	Provider                 string
	BaseURL                  string
	AppID                    string
	AppSecret                string
	TenantAccessToken        string
	APITimeout               time.Duration
	APIMaxRetries            int
	DocMaxBlocks             int
	DocMaxDepth              int
	WriteDryRunDefault       bool
	DocxMetadataPathTemplate string
	DocxChildrenPathTemplate string
}

func Load() Config {
	provider := strings.ToLower(getenv("FEISHU_PROVIDER", "feishu"))
	defaultBase := defaultFeishuBaseURL
	if provider == "lark" {
		defaultBase = defaultLarkBaseURL
	}

	return Config{
		Provider:                 provider,
		BaseURL:                  strings.TrimRight(getenv("FEISHU_BASE_URL", defaultBase), "/"),
		AppID:                    os.Getenv("FEISHU_APP_ID"),
		AppSecret:                os.Getenv("FEISHU_APP_SECRET"),
		TenantAccessToken:        os.Getenv("FEISHU_TENANT_ACCESS_TOKEN"),
		APITimeout:               time.Duration(getenvInt("FEISHU_API_TIMEOUT_MS", 15000)) * time.Millisecond,
		APIMaxRetries:            getenvInt("FEISHU_API_MAX_RETRIES", 3),
		DocMaxBlocks:             getenvInt("FEISHU_DOC_MAX_BLOCKS", 3000),
		DocMaxDepth:              getenvInt("FEISHU_DOC_MAX_DEPTH", 20),
		WriteDryRunDefault:       getenvBool("FEISHU_DOC_WRITE_DRY_RUN_DEFAULT", true),
		DocxMetadataPathTemplate: getenv("FEISHU_DOCX_METADATA_PATH_TEMPLATE", "/open-apis/docx/v1/documents/%s"),
		DocxChildrenPathTemplate: getenv("FEISHU_DOCX_CHILDREN_PATH_TEMPLATE", "/open-apis/docx/v1/documents/%s/blocks/%s/children"),
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
