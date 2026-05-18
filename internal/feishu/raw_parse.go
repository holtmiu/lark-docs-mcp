package feishu

import (
	"fmt"
	"strconv"
	"strings"
)

func metadataFromRaw(identity DocumentIdentity, raw map[string]any) DocumentMetadata {
	data := asMap(raw["data"])
	doc := firstMap(data, "document", "doc", "metadata")
	if len(doc) == 0 {
		doc = data
	}
	title := firstString(doc, "title", "name")
	if title == "" {
		title = firstString(data, "title", "name")
	}
	return DocumentMetadata{
		DocumentID:   firstNonEmpty(firstString(doc, "document_id", "documentId", "docx_token", "token"), identity.Token),
		Title:        title,
		URL:          firstNonEmpty(firstString(doc, "url"), identity.NormalizedURL),
		OwnerID:      firstString(doc, "owner_id", "ownerId", "owner"),
		CreatedTime:  scalarString(doc["create_time"], doc["created_time"], doc["createdTime"]),
		UpdatedTime:  scalarString(doc["update_time"], doc["updated_time"], doc["updatedTime"]),
		RevisionID:   firstString(doc, "revision_id", "revisionId"),
		ResourceType: identity.ResourceType,
		Permissions:  &PermissionSnapshot{CanRead: true, CanWrite: false, Visibility: "unknown", Reason: "read succeeded; write permission not checked in MVP"},
	}
}

func pageBlocks(raw map[string]any) ([]map[string]any, string, bool) {
	data := asMap(raw["data"])
	items := firstSlice(data, "items", "blocks", "children")
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m := asMap(item); len(m) > 0 {
			out = append(out, m)
		}
	}
	next := firstString(data, "page_token", "next_page_token", "nextPageToken")
	hasMore := boolValue(data["has_more"]) || boolValue(data["hasMore"])
	return out, next, hasMore
}

func normalizeBlock(provider Provider, raw map[string]any, includeRaw bool) NormalizedBlock {
	id := firstString(raw, "block_id", "blockId", "id")
	rawType := blockType(raw)
	typ := normalizedType(rawType)
	text := extractText(raw)
	attrs := map[string]any{}
	if level := headingLevel(rawType); level > 0 {
		attrs["level"] = level
	}
	if token := firstString(raw, "file_token", "image_token", "token"); token != "" {
		attrs["token"] = token
	}
	if hasChildren(raw) {
		attrs["hasChildren"] = true
	}
	if len(attrs) == 0 {
		attrs = nil
	}

	source := &NormalizedBlockSource{Provider: provider, RawType: rawType}
	if includeRaw {
		source.Raw = raw
	}
	return NormalizedBlock{ID: id, Type: typ, Text: text, Attrs: attrs, Source: source}
}

func hasChildren(raw map[string]any) bool {
	if boolValue(raw["has_children"]) || boolValue(raw["hasChildren"]) {
		return true
	}
	if children, ok := raw["children"].([]any); ok && len(children) > 0 {
		return true
	}
	if ids, ok := raw["children_ids"].([]any); ok && len(ids) > 0 {
		return true
	}
	return false
}

func blockType(raw map[string]any) string {
	for _, key := range []string{"block_type", "blockType", "type"} {
		if value, ok := raw[key]; ok {
			return scalarString(value)
		}
	}
	return "unknown"
}

func normalizedType(rawType string) string {
	t := strings.ToLower(strings.ReplaceAll(rawType, " ", "_"))
	switch {
	case strings.Contains(t, "heading") || strings.HasPrefix(t, "h") && len(t) <= 3:
		return "heading"
	case strings.Contains(t, "bullet"):
		return "bullet_list"
	case strings.Contains(t, "ordered"):
		return "ordered_list"
	case strings.Contains(t, "todo") || strings.Contains(t, "task"):
		return "todo_list"
	case strings.Contains(t, "code"):
		return "code_block"
	case strings.Contains(t, "quote"):
		return "quote"
	case strings.Contains(t, "divider"):
		return "divider"
	case strings.Contains(t, "table"):
		return "table"
	case strings.Contains(t, "image"):
		return "image"
	case strings.Contains(t, "file"):
		return "file"
	case t == "text" || strings.Contains(t, "paragraph") || strings.Contains(t, "text_run"):
		return "paragraph"
	default:
		return "unsupported"
	}
}

func headingLevel(rawType string) int {
	lower := strings.ToLower(rawType)
	for i := 1; i <= 9; i++ {
		if strings.Contains(lower, fmt.Sprintf("heading%d", i)) || strings.Contains(lower, fmt.Sprintf("heading_%d", i)) || lower == fmt.Sprintf("h%d", i) {
			return i
		}
	}
	return 0
}

func extractText(raw map[string]any) string {
	var parts []string
	collectText(raw, &parts, 0)
	return strings.Join(parts, "")
}

func collectText(value any, parts *[]string, depth int) {
	if depth > 10 || value == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"text", "content", "plain_text", "plainText"} {
			if s, ok := v[key].(string); ok && s != "" {
				*parts = append(*parts, s)
				return
			}
		}
		for _, child := range v {
			collectText(child, parts, depth+1)
		}
	case []any:
		for _, item := range v {
			collectText(item, parts, depth+1)
		}
	}
}

func firstMap(parent map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if m := asMap(parent[key]); len(m) > 0 {
			return m
		}
	}
	return nil
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func firstSlice(parent map[string]any, keys ...string) []any {
	for _, key := range keys {
		if items, ok := parent[key].([]any); ok {
			return items
		}
	}
	return nil
}

func firstString(parent map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := scalarString(parent[key]); value != "" {
			return value
		}
	}
	return ""
}

func scalarString(values ...any) string {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			return v
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boolValue(value any) bool {
	b, _ := value.(bool)
	return b
}
