package skills

import (
	"strings"
	"testing"
)

func TestParseManifestValidManifestParses(t *testing.T) {
	manifest := `
name: summarize_doc_for_review
version: 0.1.0
title: Summarize Doc For Review
description: Read a Feishu/Lark doc and prepare a review summary.
capabilities:
  - doc.read
write: false
inputs:
  type: object
  required: [input]
  properties:
    input:
      type: string
steps:
  - tool: feishu_doc_resolve
    args:
      input: ${input}
  - tool: feishu_doc_read
    args:
      input: ${input}
outputs:
  type: object
`

	got, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("ParseManifest returned error: %v", err)
	}
	if got.Name != "summarize_doc_for_review" {
		t.Fatalf("Name = %q, want summarize_doc_for_review", got.Name)
	}
	if got.Version != "0.1.0" {
		t.Fatalf("Version = %q, want 0.1.0", got.Version)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "doc.read" {
		t.Fatalf("Capabilities = %#v, want [doc.read]", got.Capabilities)
	}
	if len(got.Steps) != 2 || got.Steps[1].Tool != "feishu_doc_read" {
		t.Fatalf("Steps = %#v, want second tool feishu_doc_read", got.Steps)
	}
	if got.Inputs["type"] != "object" {
		t.Fatalf("Inputs[type] = %#v, want object", got.Inputs["type"])
	}
}

func TestParseManifestMissingNameFails(t *testing.T) {
	manifest := validManifest(t)
	manifest = strings.Replace(manifest, "name: summarize_doc_for_review\n", "", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want missing name error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error = %q, want mention name", err.Error())
	}
}

func TestParseManifestInvalidStepToolFails(t *testing.T) {
	manifest := strings.Replace(validManifest(t), "tool: feishu_doc_read", "tool: shell_exec", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want invalid step tool error")
	}
	if !strings.Contains(err.Error(), "tool") || !strings.Contains(err.Error(), "shell_exec") {
		t.Fatalf("error = %q, want mention invalid tool", err.Error())
	}
}

func TestParseManifestWriteCapabilityWithoutWriteTrueFails(t *testing.T) {
	manifest := strings.Replace(validManifest(t), "- doc.read", "- doc.comment.create", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want write capability error")
	}
	if !strings.Contains(err.Error(), "write") || !strings.Contains(err.Error(), "doc.comment.create") {
		t.Fatalf("error = %q, want mention write capability", err.Error())
	}
}

func TestParseManifestWriteStepToolWithoutWriteTrueFails(t *testing.T) {
	writeTools := WriteStepToolNames()

	for _, tool := range writeTools {
		t.Run(tool, func(t *testing.T) {
			manifest := strings.Replace(validManifest(t), "tool: feishu_doc_read", "tool: "+tool, 1)

			_, err := ParseManifest([]byte(manifest))
			if err == nil {
				t.Fatal("ParseManifest succeeded, want write step tool error")
			}
			if !strings.Contains(err.Error(), "write") || !strings.Contains(err.Error(), tool) {
				t.Fatalf("error = %q, want mention write step tool", err.Error())
			}
		})
	}
}

func TestCurrentMCPWriteToolsAreSkillWriteStepTools(t *testing.T) {
	currentMCPWriteTools := []string{
		"feishu_doc_create",
		"feishu_doc_append",
		"feishu_doc_create_comment",
		"feishu_doc_reply_comment",
		"feishu_doc_resolve_comment",
	}
	writeTools := map[string]struct{}{}
	for _, tool := range WriteStepToolNames() {
		writeTools[tool] = struct{}{}
	}
	for _, tool := range currentMCPWriteTools {
		if _, ok := writeTools[tool]; !ok {
			t.Fatalf("MCP write tool %q missing from skills write step tool allowlist", tool)
		}
		if !writeStepToolRequiresOperationID(tool) {
			t.Fatalf("MCP write tool %q missing from operationId-required allowlist", tool)
		}
	}
	if len(writeTools) != len(currentMCPWriteTools) {
		t.Fatalf("skill write step tools = %v, want current MCP write tools %v", WriteStepToolNames(), currentMCPWriteTools)
	}
}

func TestParseManifestMultipleYAMLDocumentsFails(t *testing.T) {
	manifest := validManifest(t) + "\n---\n" + validManifest(t)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want multiple YAML documents error")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("error = %q, want mention multiple documents", err.Error())
	}
}

func TestParseManifestTooLargeFails(t *testing.T) {
	manifest := validManifest(t) + strings.Repeat("# padding\n", maxManifestBytes)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want manifest size error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error = %q, want mention size limit", err.Error())
	}
}

func TestParseManifestInvalidCapabilityFails(t *testing.T) {
	manifest := strings.Replace(validManifest(t), "- doc.read", "- doc.delete", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want invalid capability error")
	}
	if !strings.Contains(err.Error(), "capability") || !strings.Contains(err.Error(), "doc.delete") {
		t.Fatalf("error = %q, want mention invalid capability", err.Error())
	}
}

func TestParseManifestUnknownTopLevelFieldFails(t *testing.T) {
	manifest := strings.Replace(validManifest(t), "version: 0.1.0\n", "version: 0.1.0\nunknown: true\n", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %q, want mention unknown field", err.Error())
	}
}

func TestParseManifestInputSchemaMustBeObject(t *testing.T) {
	manifest := strings.Replace(validManifest(t), "type: object", "type: string", 1)

	_, err := ParseManifest([]byte(manifest))
	if err == nil {
		t.Fatal("ParseManifest succeeded, want input schema type error")
	}
	if !strings.Contains(err.Error(), "inputs") || !strings.Contains(err.Error(), "object") {
		t.Fatalf("error = %q, want mention inputs object", err.Error())
	}
}

func validManifest(t *testing.T) string {
	t.Helper()
	return `
name: summarize_doc_for_review
version: 0.1.0
title: Summarize Doc For Review
description: Read a Feishu/Lark doc and prepare a review summary.
capabilities:
  - doc.read
write: false
inputs:
  type: object
  required: [input]
  properties:
    input:
      type: string
steps:
  - tool: feishu_doc_read
    args:
      input: ${input}
outputs:
  type: object
`
}
