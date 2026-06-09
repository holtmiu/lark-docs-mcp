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
