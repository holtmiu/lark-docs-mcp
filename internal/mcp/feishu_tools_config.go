package mcp

import (
	"fmt"
	"strings"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
	"github.com/holtmiu/lark-docs-mcp/internal/feishu"
	"github.com/holtmiu/lark-docs-mcp/internal/skills"
)

// NewFeishuToolsFromConfig builds the MCP tool handler used by command entrypoints.
// Skill discovery is exposed only when FEISHU_SKILLS_DIRS/config.SkillDirs is set.
// Discovery returns manifest steps, so manifests should not contain secrets.
func NewFeishuToolsFromConfig(cfg config.Config, allowCredentialSelection bool) (FeishuTools, error) {
	tools := FeishuTools{
		Service:                  feishu.NewService(cfg),
		AllowCredentialSelection: allowCredentialSelection,
		SkillsEnableWrite:        cfg.SkillsEnableWrite,
	}
	if len(cfg.SkillDirs) == 0 {
		return tools, nil
	}

	registry, err := skills.LoadRegistryWithOptions(cfg.SkillDirs, skills.RegistryOptions{EnableWrite: cfg.SkillsEnableWrite})
	if err != nil {
		return FeishuTools{}, fmt.Errorf("load skill registry: %s", redactConfiguredSecrets(err.Error(), cfg))
	}
	tools.SkillRegistry = registry
	return tools, nil
}

func redactConfiguredSecrets(message string, cfg config.Config) string {
	for _, secret := range []string{
		cfg.AppSecret,
		cfg.TenantAccessToken,
		cfg.MCPServerAPIKey,
		cfg.OAuthStateSecret,
		cfg.TokenEncryptKey,
	} {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return message
}
