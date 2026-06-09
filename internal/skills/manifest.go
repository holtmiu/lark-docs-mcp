package skills

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

const maxManifestBytes = 1 << 20

// Manifest describes a reusable skill workflow loaded from a local manifest.
type Manifest struct {
	Name         string         `yaml:"name" json:"name"`
	Version      string         `yaml:"version" json:"version"`
	Title        string         `yaml:"title" json:"title"`
	Description  string         `yaml:"description" json:"description"`
	Capabilities []string       `yaml:"capabilities" json:"capabilities"`
	Write        bool           `yaml:"write" json:"write"`
	Inputs       map[string]any `yaml:"inputs" json:"inputs"`
	Steps        []Step         `yaml:"steps" json:"steps"`
	Outputs      map[string]any `yaml:"outputs" json:"outputs"`
}

// Step is one ordered call to an allowlisted internal MCP tool.
type Step struct {
	Tool string         `yaml:"tool" json:"tool"`
	Args map[string]any `yaml:"args" json:"args"`
}

var allowedStepTools = map[string]struct{}{
	"feishu_doc_resolve":          {},
	"feishu_doc_get_metadata":     {},
	"feishu_doc_check_permission": {},
	"feishu_doc_read":             {},
	"feishu_doc_create":           {},
	"feishu_doc_append":           {},
	"feishu_doc_list_comments":    {},
	"feishu_doc_create_comment":   {},
	"feishu_doc_reply_comment":    {},
	"feishu_doc_resolve_comment":  {},
}

var allowedCapabilities = map[string]struct{}{
	"doc.read":             {},
	"doc.metadata":         {},
	"doc.permission.check": {},
	"doc.create":           {},
	"doc.append":           {},
	"doc.comment.list":     {},
	"doc.comment.create":   {},
	"doc.comment.reply":    {},
	"doc.comment.resolve":  {},
}

var writeCapabilities = map[string]struct{}{
	"doc.create":          {},
	"doc.append":          {},
	"doc.comment.create":  {},
	"doc.comment.reply":   {},
	"doc.comment.resolve": {},
}

var writeStepTools = map[string]struct{}{
	"feishu_doc_create":          {},
	"feishu_doc_append":          {},
	"feishu_doc_create_comment":  {},
	"feishu_doc_reply_comment":   {},
	"feishu_doc_resolve_comment": {},
}

// ParseManifest parses and validates a YAML skill manifest into typed structs.
func ParseManifest(data []byte) (Manifest, error) {
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("skill manifest too large: %d bytes exceeds limit %d", len(data), maxManifestBytes)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse skill manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Manifest{}, fmt.Errorf("parse skill manifest: %w", err)
		}
		return Manifest{}, fmt.Errorf("skill manifest must contain exactly one YAML document; multiple documents are not allowed")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate checks manifest schema constraints that must be enforced before registry loading or execution.
func (m Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("skill manifest name is required")
	}
	if m.Inputs == nil {
		return fmt.Errorf("skill manifest inputs schema is required and must be an object")
	}
	if got, ok := m.Inputs["type"].(string); !ok || got != "object" {
		return fmt.Errorf("skill manifest inputs schema type must be object")
	}
	for _, capability := range m.Capabilities {
		if !isAllowedCapability(capability) {
			return fmt.Errorf("skill manifest capability %q is not allowed; allowed capabilities: %v", capability, sortedKeys(allowedCapabilities))
		}
		if isWriteCapability(capability) && !m.Write {
			return fmt.Errorf("skill manifest capability %q requires write: true", capability)
		}
	}
	for i, step := range m.Steps {
		if step.Tool == "" {
			return fmt.Errorf("skill manifest steps[%d].tool is required", i)
		}
		if !isAllowedStepTool(step.Tool) {
			return fmt.Errorf("skill manifest steps[%d].tool %q is not allowed; allowed tools: %v", i, step.Tool, sortedKeys(allowedStepTools))
		}
		if isWriteStepTool(step.Tool) && !m.Write {
			return fmt.Errorf("skill manifest steps[%d].tool %q requires write: true", i, step.Tool)
		}
	}
	return nil
}

func isAllowedStepTool(tool string) bool {
	_, ok := allowedStepTools[tool]
	return ok
}

func isAllowedCapability(capability string) bool {
	_, ok := allowedCapabilities[capability]
	return ok
}

func isWriteCapability(capability string) bool {
	_, ok := writeCapabilities[capability]
	return ok
}

func isWriteStepTool(tool string) bool {
	_, ok := writeStepTools[tool]
	return ok
}

// WriteStepToolNames returns the current skill write-step tool allowlist.
func WriteStepToolNames() []string {
	return sortedKeys(writeStepTools)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
