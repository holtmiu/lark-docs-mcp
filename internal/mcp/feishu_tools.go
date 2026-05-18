package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/feishu"
)

type FeishuTools struct {
	Service *feishu.Service
}

func (t FeishuTools) Tools() []Tool {
	stringProp := map[string]any{"type": "string"}
	boolProp := map[string]any{"type": "boolean"}
	intProp := map[string]any{"type": "integer"}
	return []Tool{
		{
			Name:        "feishu_doc_resolve",
			Description: "Resolve a Feishu/Lark document URL or token into a normalized document identity.",
			InputSchema: objectSchema(map[string]any{"input": stringProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_get_metadata",
			Description: "Get metadata for a Feishu/Lark docx document.",
			InputSchema: objectSchema(map[string]any{"input": stringProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_read",
			Description: "Read a Feishu/Lark docx document and return normalized blocks plus Markdown.",
			InputSchema: objectSchema(map[string]any{"input": stringProp, "format": stringProp, "maxBlocks": intProp, "maxDepth": intProp, "includeUnsupportedRaw": boolProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_append",
			Description: "Dry-run append Markdown to a Feishu/Lark document. Real write execution is disabled in MVP.",
			InputSchema: objectSchema(map[string]any{"input": stringProp, "markdown": stringProp, "afterBlockId": stringProp, "dryRun": boolProp, "operationId": stringProp}, []string{"input"}),
		},
	}
}

func (t FeishuTools) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case "feishu_doc_resolve":
		var req struct {
			Input string `json:"input"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return t.Service.Resolve(req.Input)
	case "feishu_doc_get_metadata":
		var req struct {
			Input string `json:"input"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return t.Service.GetMetadata(ctx, req.Input)
	case "feishu_doc_read":
		var req struct {
			Input                 string `json:"input"`
			Format                string `json:"format,omitempty"`
			MaxBlocks             int    `json:"maxBlocks,omitempty"`
			MaxDepth              int    `json:"maxDepth,omitempty"`
			IncludeUnsupportedRaw bool   `json:"includeUnsupportedRaw,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return t.Service.ReadDocument(ctx, req.Input, feishu.ReadOptions{Format: req.Format, MaxBlocks: req.MaxBlocks, MaxDepth: req.MaxDepth, IncludeUnsupportedRaw: req.IncludeUnsupportedRaw})
	case "feishu_doc_append":
		var req struct {
			Input        string `json:"input"`
			Markdown     string `json:"markdown,omitempty"`
			AfterBlockID string `json:"afterBlockId,omitempty"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return t.Service.AppendDocument(ctx, req.Input, feishu.AppendRequest{Markdown: req.Markdown, AfterBlockID: req.AfterBlockID, DryRun: req.DryRun, OperationID: req.OperationID})
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
