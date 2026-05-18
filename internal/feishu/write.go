package feishu

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func buildCreateDocumentRequest(req CreateDocumentRequest) (map[string]any, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, newError(ErrInvalidInput, "title is required", nil)
	}
	body := map[string]any{"title": title}
	if req.FolderToken != "" {
		body["folder_token"] = req.FolderToken
	}
	return body, nil
}

func buildAppendBlocksRequest(req AppendRequest) (map[string]any, []string, error) {
	blocks := req.Blocks
	if len(blocks) == 0 && strings.TrimSpace(req.Markdown) != "" {
		blocks = markdownToBlocks(req.Markdown)
	}
	if len(blocks) == 0 {
		return nil, nil, newError(ErrInvalidInput, "markdown or blocks is required", nil)
	}
	children := make([]any, 0, len(blocks))
	ids := make([]string, 0, len(blocks))
	for i, block := range blocks {
		children = append(children, normalizedBlockToFeishuBlock(block))
		if block.ID != "" {
			ids = append(ids, block.ID)
		} else {
			ids = append(ids, fmt.Sprintf("new_block_%d", i+1))
		}
	}
	return map[string]any{"children": children}, ids, nil
}

func normalizedBlockToFeishuBlock(block NormalizedBlock) map[string]any {
	text := strings.TrimSpace(block.Text)
	switch block.Type {
	case "heading":
		level := intAttr(block.Attrs, "level", 2)
		if level < 1 || level > 9 {
			level = 2
		}
		return textBlock(fmt.Sprintf("heading%d", level), text)
	case "bullet_list":
		return textBlock("bullet", text)
	case "ordered_list":
		return textBlock("ordered", text)
	case "todo_list":
		b := textBlock("todo", text)
		b["checked"] = false
		return b
	case "code_block":
		return map[string]any{"block_type": "code", "code": map[string]any{"language": "plain_text", "content": block.Text}}
	case "quote":
		return textBlock("quote", text)
	case "divider":
		return map[string]any{"block_type": "divider"}
	default:
		return textBlock("text", text)
	}
}

func textBlock(blockType, text string) map[string]any {
	return map[string]any{
		"block_type": blockType,
		"text": map[string]any{
			"elements": []any{
				map[string]any{"text_run": map[string]any{"content": text}},
			},
		},
	}
}

func defaultOperationID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}

func writeDryRun(defaultValue bool, override *bool) bool {
	if override != nil {
		return *override
	}
	return defaultValue
}
