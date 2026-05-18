package feishu

import (
	"strings"
	"testing"
)

func TestExportMarkdown(t *testing.T) {
	got := exportMarkdown([]NormalizedBlock{
		{ID: "1", Type: "heading", Text: "Title", Attrs: map[string]any{"level": 1}},
		{ID: "2", Type: "paragraph", Text: "Body"},
	})
	if !strings.Contains(got, "# Title") || !strings.Contains(got, "Body") {
		t.Fatalf("unexpected markdown output")
	}
}
