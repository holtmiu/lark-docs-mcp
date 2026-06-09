package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holtmiu/lark-docs-mcp/internal/config"
)

func TestNewFeishuToolsFromConfigOmitsSkillRegistryWhenDirsUnset(t *testing.T) {
	tools, err := NewFeishuToolsFromConfig(config.Config{}, true)
	if err != nil {
		t.Fatalf("NewFeishuToolsFromConfig returned error: %v", err)
	}
	if tools.Service == nil {
		t.Fatal("Service was not configured")
	}
	if tools.SkillRegistry != nil {
		t.Fatalf("SkillRegistry = %#v, want nil without SkillDirs", tools.SkillRegistry)
	}
	if !tools.AllowCredentialSelection {
		t.Fatal("AllowCredentialSelection was not propagated")
	}
}

func TestNewFeishuToolsFromConfigLoadsSkillRegistryWithWritePolicy(t *testing.T) {
	root := t.TempDir()
	writeMCPTestSkillManifest(t, filepath.Join(root, "writer", "skill.yaml"), true)

	_, err := NewFeishuToolsFromConfig(config.Config{SkillDirs: []string{root}}, false)
	if err == nil {
		t.Fatal("NewFeishuToolsFromConfig succeeded for write skill with write disabled")
	}
	if !strings.Contains(err.Error(), "load skill registry") || !strings.Contains(err.Error(), "write") {
		t.Fatalf("error = %q, want actionable fail-closed registry load error", err.Error())
	}

	tools, err := NewFeishuToolsFromConfig(config.Config{SkillDirs: []string{root}, SkillsEnableWrite: true}, false)
	if err != nil {
		t.Fatalf("NewFeishuToolsFromConfig with write enabled returned error: %v", err)
	}
	if tools.SkillRegistry == nil {
		t.Fatal("SkillRegistry was not configured")
	}
	manifest, ok := tools.SkillRegistry.Get("writer")
	if !ok || !manifest.Write {
		t.Fatalf("SkillRegistry.Get(writer) = %#v, %v; want write manifest", manifest, ok)
	}
}

func TestNewFeishuToolsFromConfigRedactsConfiguredSecretsInLoadErrors(t *testing.T) {
	secret := "super-secret-token"
	_, err := NewFeishuToolsFromConfig(config.Config{SkillDirs: []string{"/definitely/missing/" + secret}, AppSecret: secret}, false)
	if err == nil {
		t.Fatal("NewFeishuToolsFromConfig succeeded for missing skills dir")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked configured secret: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %q, want redaction marker", err.Error())
	}
}

func writeMCPTestSkillManifest(t *testing.T, path string, write bool) {
	t.Helper()
	capability := "doc.read"
	tool := "feishu_doc_read"
	writeValue := "false"
	name := "reader"
	if write {
		capability = "doc.comment.create"
		tool = "feishu_doc_create_comment"
		writeValue = "true"
		name = "writer"
	}
	manifest := `
name: ` + name + `
version: 0.1.0
title: Test Skill
description: Test discovery manifest.
capabilities:
  - ` + capability + `
write: ` + writeValue + `
inputs:
  type: object
steps:
  - tool: ` + tool + `
outputs:
  type: object
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
