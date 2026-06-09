package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/holtmiu/lark-docs-mcp/internal/feishu"
	"github.com/holtmiu/lark-docs-mcp/internal/skills"
)

type FeishuTools struct {
	Service                  *feishu.Service
	AllowCredentialSelection bool
	SkillRegistry            SkillRegistry
	SkillsEnableWrite        bool
}

type SkillRegistry interface {
	List() []skills.Manifest
	Get(name string) (skills.Manifest, bool)
}

type SkillSummary struct {
	Name         string   `json:"name"`
	Title        string   `json:"title,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities"`
}

type SkillManifestSummary struct {
	Name         string         `json:"name"`
	Version      string         `json:"version,omitempty"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	Capabilities []string       `json:"capabilities"`
	Write        bool           `json:"write"`
	Inputs       map[string]any `json:"inputs,omitempty"`
	Steps        []skills.Step  `json:"steps,omitempty"`
	Outputs      map[string]any `json:"outputs,omitempty"`
}

type SkillListResult struct {
	Skills []SkillSummary `json:"skills"`
}

type SkillGetResult struct {
	Skill SkillManifestSummary `json:"skill"`
}

type structuredToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Name    string `json:"name,omitempty"`
}

func (e structuredToolError) Error() string {
	raw, err := json.Marshal(e)
	if err != nil {
		return e.Message
	}
	return string(raw)
}

func structuredSkillRunError(err error) error {
	var skillErr skills.SkillError
	if errors.As(err, &skillErr) {
		return structuredToolError{Code: skillErr.Code, Message: skillErr.Error(), Name: skillErr.Name}
	}
	return structuredToolError{Code: "skill_step_failed", Message: err.Error()}
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
	inputProp := map[string]any{"type": "string", "maxLength": 2048}
	contentProp := map[string]any{"type": "string", "minLength": 1, "maxLength": 20000}
	commentIDProp := map[string]any{"type": "string", "maxLength": 256}
	tools := []Tool{
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
			Name:        "feishu_doc_check_permission",
			Description: "Safely preflight read/write/comment capability for a Feishu/Lark document before reading, writing, or commenting.",
			InputSchema: objectSchema(map[string]any{"input": inputProp, "credentialId": credentialIDProp}, []string{"input"}),
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
		{
			Name:        "feishu_doc_list_comments",
			Description: "List Feishu/Lark document comments with optional pagination.",
			InputSchema: objectSchema(map[string]any{"input": inputProp, "credentialId": credentialIDProp, "pageSize": intProp, "pageToken": stringProp}, []string{"input"}),
		},
		{
			Name:        "feishu_doc_create_comment",
			Description: "Create a Feishu/Lark document comment. Dry-run is enabled by default unless dryRun=false or server default is changed.",
			InputSchema: objectSchema(map[string]any{"input": inputProp, "credentialId": credentialIDProp, "content": contentProp, "blockId": stringProp, "quote": stringProp, "dryRun": boolProp, "operationId": stringProp}, []string{"input", "content"}),
		},
		{
			Name:        "feishu_doc_reply_comment",
			Description: "Reply to a Feishu/Lark document comment. Dry-run is enabled by default unless dryRun=false or server default is changed.",
			InputSchema: objectSchema(map[string]any{"input": inputProp, "credentialId": credentialIDProp, "commentId": commentIDProp, "content": contentProp, "dryRun": boolProp, "operationId": stringProp}, []string{"input", "commentId", "content"}),
		},
		{
			Name:        "feishu_doc_resolve_comment",
			Description: "Resolve or reopen a Feishu/Lark document comment. Dry-run is enabled by default unless dryRun=false or server default is changed.",
			InputSchema: objectSchema(map[string]any{"input": inputProp, "credentialId": credentialIDProp, "commentId": commentIDProp, "resolved": boolProp, "dryRun": boolProp, "operationId": stringProp}, []string{"input", "commentId", "resolved"}),
		},
	}
	if t.SkillRegistry != nil {
		tools = append(tools,
			Tool{
				Name:        "feishu_skill_list",
				Description: "List configured Feishu/Lark skill manifests as read-only discovery summaries. Does not execute skills.",
				InputSchema: objectSchema(map[string]any{}, nil),
			},
			Tool{
				Name:        "feishu_skill_get",
				Description: "Get one configured Feishu/Lark skill manifest summary by name as read-only discovery. Does not execute skills.",
				InputSchema: objectSchema(map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}, []string{"name"}),
			},
			Tool{
				Name:        "feishu_skill_run",
				Description: "Run a configured Feishu/Lark skill. Read-only behavior is the default; write-capable skills require server write enablement, dry-run by default, operationId for real mutations, and permission preflight.",
				InputSchema: objectSchema(map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "inputs": map[string]any{"type": "object", "additionalProperties": true}, "dryRun": boolProp}, []string{"name"}),
			},
		)
	}
	return tools
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
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.GetMetadataWithActor(ctx, req.Input, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_check_permission":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if len(req.Input) > 2048 {
			return nil, fmt.Errorf("input exceeds max length 2048")
		}
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.CheckPermissionWithActor(ctx, req.Input, feishu.ActorContext{CredentialID: req.CredentialID})
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
		if err := t.validateCredentialID(req.CredentialID); err != nil {
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
		if err := t.validateCredentialID(req.CredentialID); err != nil {
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
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.AppendDocumentWithActor(ctx, req.Input, feishu.AppendRequest{Markdown: req.Markdown, AfterBlockID: req.AfterBlockID, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_list_comments":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
			PageSize     int    `json:"pageSize,omitempty"`
			PageToken    string `json:"pageToken,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateDocumentInput(req.Input); err != nil {
			return nil, err
		}
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.ListComments(ctx, req.Input, feishu.ListCommentsRequest{PageSize: req.PageSize, PageToken: req.PageToken}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_create_comment":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
			Content      string `json:"content"`
			BlockID      string `json:"blockId,omitempty"`
			Quote        string `json:"quote,omitempty"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateDocumentInput(req.Input); err != nil {
			return nil, err
		}
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		return t.Service.CreateComment(ctx, req.Input, feishu.CreateCommentRequest{Content: req.Content, BlockID: req.BlockID, Quote: req.Quote, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_reply_comment":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
			CommentID    string `json:"commentId"`
			Content      string `json:"content"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateDocumentInput(req.Input); err != nil {
			return nil, err
		}
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		if len(req.CommentID) > 256 {
			return nil, fmt.Errorf("commentId exceeds max length 256")
		}
		return t.Service.ReplyComment(ctx, req.Input, req.CommentID, feishu.ReplyCommentRequest{Content: req.Content, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_doc_resolve_comment":
		var req struct {
			Input        string `json:"input"`
			CredentialID string `json:"credentialId,omitempty"`
			CommentID    string `json:"commentId"`
			Resolved     *bool  `json:"resolved"`
			DryRun       *bool  `json:"dryRun,omitempty"`
			OperationID  string `json:"operationId,omitempty"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if err := validateDocumentInput(req.Input); err != nil {
			return nil, err
		}
		if err := t.validateCredentialID(req.CredentialID); err != nil {
			return nil, err
		}
		if len(req.CommentID) > 256 {
			return nil, fmt.Errorf("commentId exceeds max length 256")
		}
		if req.Resolved == nil {
			return nil, fmt.Errorf("resolved is required")
		}
		return t.Service.ResolveComment(ctx, req.Input, req.CommentID, feishu.ResolveCommentRequest{Resolved: *req.Resolved, DryRun: req.DryRun, OperationID: req.OperationID}, feishu.ActorContext{CredentialID: req.CredentialID})
	case "feishu_skill_list":
		if t.SkillRegistry == nil {
			return nil, structuredToolError{Code: "registry_unconfigured", Message: "skill registry is not configured"}
		}
		if err := decodeArgs(args, &struct{}{}); err != nil {
			return nil, err
		}
		return SkillListResult{Skills: summarizeSkillList(t.SkillRegistry.List())}, nil
	case "feishu_skill_get":
		if t.SkillRegistry == nil {
			return nil, structuredToolError{Code: "registry_unconfigured", Message: "skill registry is not configured"}
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Name) == "" {
			return nil, structuredToolError{Code: "invalid_skill_name", Message: "skill name is required"}
		}
		if len(req.Name) > 128 {
			return nil, structuredToolError{Code: "invalid_skill_name", Message: "skill name exceeds max length 128"}
		}
		manifest, ok := t.SkillRegistry.Get(req.Name)
		if !ok {
			return nil, structuredToolError{Code: "skill_not_found", Message: fmt.Sprintf("skill %q was not found", req.Name), Name: req.Name}
		}
		return SkillGetResult{Skill: summarizeSkillManifest(manifest)}, nil
	case "feishu_skill_run":
		if t.SkillRegistry == nil {
			return nil, structuredToolError{Code: "registry_unconfigured", Message: "skill registry is not configured"}
		}
		var req skills.RunRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Skill) == "" {
			return nil, structuredToolError{Code: "invalid_skill_name", Message: "skill name is required"}
		}
		if len(req.Skill) > 128 {
			return nil, structuredToolError{Code: "invalid_skill_name", Message: "skill name exceeds max length 128"}
		}
		executor := skills.NewExecutorWithOptions(t.SkillRegistry, feishuToolCaller{tools: t}, skills.ExecutorOptions{EnableWrite: t.SkillsEnableWrite, EnableRealWrites: t.SkillsEnableWrite})
		result, err := executor.Run(ctx, req)
		if err != nil {
			return nil, structuredSkillRunError(err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

type feishuToolCaller struct {
	tools FeishuTools
}

func (c feishuToolCaller) CallTool(ctx context.Context, tool string, args map[string]any) (any, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal skill step args: %w", err)
	}
	return c.tools.CallTool(ctx, tool, raw)
}

func summarizeSkillList(manifests []skills.Manifest) []SkillSummary {
	summaries := make([]SkillSummary, 0, len(manifests))
	for _, manifest := range manifests {
		summaries = append(summaries, SkillSummary{
			Name:         manifest.Name,
			Title:        manifest.Title,
			Description:  manifest.Description,
			Capabilities: append([]string(nil), manifest.Capabilities...),
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries
}

func summarizeSkillManifest(manifest skills.Manifest) SkillManifestSummary {
	return SkillManifestSummary{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Title:        manifest.Title,
		Description:  manifest.Description,
		Capabilities: append([]string(nil), manifest.Capabilities...),
		Write:        manifest.Write,
		Inputs:       manifest.Inputs,
		Steps:        append([]skills.Step(nil), manifest.Steps...),
		Outputs:      manifest.Outputs,
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

func (t FeishuTools) validateCredentialID(value string) error {
	if err := validateCredentialID(value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != "" && !t.AllowCredentialSelection {
		return fmt.Errorf("credentialId is disabled for this MCP server")
	}
	return nil
}

func validateDocumentInput(value string) error {
	if len(value) > 2048 {
		return fmt.Errorf("input exceeds max length 2048")
	}
	return nil
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
