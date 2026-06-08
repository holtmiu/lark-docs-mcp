package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/feishu"
)

type FeishuTools struct {
	Service *feishu.Service
}

func (t FeishuTools) Tools() []Tool {
	stringProp := map[string]any{"type": "string"}
	stateProp := map[string]any{"type": "string", "maxLength": 256}
	redirectURIProp := map[string]any{"type": "string", "maxLength": 2048}
	scopesProp := map[string]any{"type": "array", "items": map[string]any{"type": "string", "maxLength": feishu.OAuthScopeMaxLength}, "maxItems": 20}
	credentialIDProp := map[string]any{"type": "string", "maxLength": 128}
	boolProp := map[string]any{"type": "boolean"}
	intProp := map[string]any{"type": "integer", "minimum": 1}
	formatProp := map[string]any{"type": "string", "enum": []string{"json", "markdown", "both"}}
	return []Tool{
		{
			Name:        "feishu_oauth_auth_url",
			Description: "Build a Feishu/Lark user OAuth authorization URL for granting document permissions. Does not expose app secrets or tokens.",
			InputSchema: objectSchema(map[string]any{"state": stateProp, "redirectUri": redirectURIProp, "scopes": scopesProp}, nil),
		},
		{
			Name:        "feishu_doc_resolve",
			Description: "Resolve a Feishu/Lark document URL or token into a normalized document identity. This tool does not call Feishu APIs.",
			InputSchema: objectSchema(map[string]any{"input": stringProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_get_metadata",
			Description: "Get metadata for a Feishu/Lark docx document using the configured Feishu/Lark app credentials.",
			InputSchema: objectSchema(map[string]any{"input": stringProp, "credentialId": credentialIDProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_read",
			Description: "Read a Feishu/Lark docx document and return normalized blocks plus Markdown. Requires the document to be accessible to the configured app/token.",
			InputSchema: objectSchema(map[string]any{"input": stringProp, "credentialId": credentialIDProp, "format": formatProp, "maxBlocks": intProp, "maxDepth": intProp, "includeUnsupportedRaw": boolProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_create",
			Description: "Create a Feishu/Lark docx document and optionally append Markdown content. Dry-run is enabled by default unless dryRun=false or server default is changed.",
			InputSchema: objectSchema(map[string]any{"title": stringProp, "credentialId": credentialIDProp, "folderToken": stringProp, "markdown": stringProp, "dryRun": boolProp, "operationId": stringProp}, []string{"title"}),
		},
		{
			Name:        "feishu_doc_append",
			Description: "Append Markdown content to a Feishu/Lark docx document. Dry-run is enabled by default unless dryRun=false or server default is changed.",
			InputSchema: objectSchema(map[string]any{"input": stringProp, "credentialId": credentialIDProp, "markdown": stringProp, "afterBlockId": stringProp, "dryRun": boolProp, "operationId": stringProp}, []string{"input", "markdown"}),
		},
	}
}

func (t FeishuTools) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case "feishu_oauth_auth_url":
		var req struct {
			State       string   `json:"state,omitempty"`
			RedirectURI string   `json:"redirectUri,omitempty"`
			Scopes      []string `json:"scopes,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if len(req.State) > 256 {
			return nil, fmt.Errorf("state exceeds max length 256")
		}
		if len(req.RedirectURI) > 2048 {
			return nil, fmt.Errorf("redirectUri exceeds max length 2048")
		}
		if len(req.Scopes) > 20 {
			return nil, fmt.Errorf("scopes exceeds max items 20")
		}
		for i, scope := range req.Scopes {
			if len(scope) > feishu.OAuthScopeMaxLength {
				return nil, fmt.Errorf("scopes[%d] exceeds max length %d", i, feishu.OAuthScopeMaxLength)
			}
		}
		return t.Service.BuildOAuthAuthURL(feishu.OAuthAuthURLRequest{RedirectURI: req.RedirectURI, State: req.State, Scopes: req.Scopes})
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
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.GetMetadataWithActor(ctx, req.Input, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_read":
		var req struct {
			Input                 string `json:"input"`
			CredentialID          string `json:"credentialId,omitempty"`
			Format                string `json:"format,omitempty"`
			MaxBlocks             int    `json:"maxBlocks,omitempty"`
			MaxDepth              int    `json:"maxDepth,omitempty"`
			IncludeUnsupportedRaw bool   `json:"includeUnsupportedRaw,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.ReadDocumentWithActor(ctx, req.Input, feishu.ReadOptions{Format: req.Format, MaxBlocks: req.MaxBlocks, MaxDepth: req.MaxDepth, IncludeUnsupportedRaw: req.IncludeUnsupportedRaw}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_create":
		var req struct {
			Title        string `json:"title"`
			CredentialID string `json:"credentialId,omitempty"`
			FolderToken  string `json:"folderToken,omitempty"`
			Markdown     string `json:"markdown,omitempty"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.CreateDocumentWithActor(ctx, feishu.CreateDocumentRequest{Title: req.Title, FolderToken: req.FolderToken, Markdown: req.Markdown, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_append":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
			Markdown     string `json:"markdown,omitempty"`
			AfterBlockID string `json:"afterBlockId,omitempty"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.AppendDocumentWithActor(ctx, req.Input, feishu.AppendRequest{Markdown: req.Markdown, AfterBlockID: req.AfterBlockID, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid tool arguments: multiple JSON values")
	}
	return nil
}

func validateCredentialID(value string) error {
	if len(value) > 128 {
		return fmt.Errorf("credentialId exceeds max length 128")
	}
	return nil
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
