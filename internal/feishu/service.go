package feishu

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/holtmiu/ChatGPT_MCP_Connectors/internal/config"
)

type Service struct {
	cfg      config.Config
	resolver Resolver
	client   *HTTPClient
}

func NewService(cfg config.Config) *Service {
	return &Service{
		cfg:      cfg,
		resolver: NewResolver(cfg.Provider),
		client: NewHTTPClient(HTTPClientOptions{
			BaseURL:           cfg.BaseURL,
			AppID:             cfg.AppID,
			AppSecret:         cfg.AppSecret,
			TenantAccessToken: cfg.TenantAccessToken,
			Timeout:           cfg.APITimeout,
			MaxRetries:        cfg.APIMaxRetries,
		}),
	}
}

func (s *Service) Resolve(input string) (DocumentIdentity, error) {
	return s.resolver.Resolve(input)
}

func (s *Service) GetMetadata(ctx context.Context, input string) (DocumentMetadata, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return DocumentMetadata{}, err
	}
	return s.GetMetadataByIdentity(ctx, identity)
}

func (s *Service) GetMetadataByIdentity(ctx context.Context, identity DocumentIdentity) (DocumentMetadata, error) {
	if identity.ResourceType != ResourceDocx && identity.ResourceType != ResourceUnknown {
		return DocumentMetadata{}, newError(ErrUnsupportedDocumentType, fmt.Sprintf("metadata for resource type %s is not implemented yet", identity.ResourceType), nil)
	}

	var raw map[string]any
	path := fmt.Sprintf(s.cfg.DocxMetadataPathTemplate, url.PathEscape(identity.Token))
	if err := s.client.GetJSON(ctx, path, nil, &raw); err != nil {
		return DocumentMetadata{}, err
	}
	metadata := metadataFromRaw(identity, raw)
	if metadata.DocumentID == "" {
		metadata.DocumentID = identity.Token
	}
	if metadata.ResourceType == "" {
		metadata.ResourceType = identity.ResourceType
	}
	return metadata, nil
}

func (s *Service) ReadDocument(ctx context.Context, input string, options ReadOptions) (DocumentReadResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return DocumentReadResult{}, err
	}
	metadata, err := s.GetMetadataByIdentity(ctx, identity)
	if err != nil {
		return DocumentReadResult{}, err
	}

	maxBlocks := options.MaxBlocks
	if maxBlocks <= 0 || maxBlocks > s.cfg.DocMaxBlocks {
		maxBlocks = s.cfg.DocMaxBlocks
	}
	maxDepth := options.MaxDepth
	if maxDepth <= 0 || maxDepth > s.cfg.DocMaxDepth {
		maxDepth = s.cfg.DocMaxDepth
	}

	state := readState{maxBlocks: maxBlocks, maxDepth: maxDepth, includeRaw: options.IncludeUnsupportedRaw}
	blocks, err := s.readChildren(ctx, identity, identity.Token, 0, &state)
	if err != nil {
		return DocumentReadResult{}, err
	}

	result := DocumentReadResult{Metadata: metadata, Blocks: blocks, Warnings: state.warnings, Truncated: state.truncated}
	if options.Format == "markdown" || options.Format == "both" || options.Format == "" {
		result.Markdown = exportMarkdown(blocks)
	}
	return result, nil
}

func (s *Service) CreateDocument(ctx context.Context, req CreateDocumentRequest) (DocumentWriteResult, error) {
	body, err := buildCreateDocumentRequest(req)
	if err != nil {
		return DocumentWriteResult{}, err
	}
	dryRun := writeDryRun(s.cfg.WriteDryRunDefault, req.DryRun)
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID("create", req.Title, req.FolderToken, req.Markdown)
	}
	result := DocumentWriteResult{OperationID: operationID, DryRun: dryRun, Request: body, Warnings: []string{}}
	if dryRun {
		result.Warnings = append(result.Warnings, "dry-run only: no document was created")
		return result, nil
	}

	var raw map[string]any
	if err := s.client.PostJSON(ctx, s.cfg.DocxCreatePath, body, &raw); err != nil {
		return DocumentWriteResult{}, err
	}
	identity := documentIdentityFromCreateResponse(raw, s.cfg.Provider)
	result.DocumentID = identity.Token
	result.URL = identity.NormalizedURL
	if result.DocumentID == "" {
		return result, newError(ErrUpstream, "create document response did not include document id", nil)
	}
	if strings.TrimSpace(req.Markdown) != "" {
		appendResult, err := s.AppendDocument(ctx, result.DocumentID, AppendRequest{Markdown: req.Markdown, DryRun: &dryRun, OperationID: operationID + ":append"})
		if err != nil {
			return result, err
		}
		result.ChangedBlocks = appendResult.ChangedBlocks
	}
	return result, nil
}

func (s *Service) AppendDocument(ctx context.Context, input string, req AppendRequest) (DocumentWriteResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return DocumentWriteResult{}, err
	}
	dryRun := writeDryRun(s.cfg.WriteDryRunDefault, req.DryRun)
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID("append", identity.Token, req.AfterBlockID, req.Markdown)
	}
	body, ids, err := buildAppendBlocksRequest(req)
	if err != nil {
		return DocumentWriteResult{}, err
	}
	blockID := strings.TrimSpace(req.AfterBlockID)
	if blockID == "" {
		blockID = identity.Token
	}

	result := DocumentWriteResult{
		OperationID:   operationID,
		DocumentID:    identity.Token,
		ChangedBlocks: ids,
		URL:           identity.NormalizedURL,
		DryRun:        dryRun,
		Request:       body,
		Warnings:      []string{},
	}
	if dryRun {
		result.Warnings = append(result.Warnings, "dry-run only: no content was written to Feishu/Lark")
		return result, nil
	}

	path := fmt.Sprintf(s.cfg.DocxAppendChildrenPathTemplate, url.PathEscape(identity.Token), url.PathEscape(blockID))
	var raw map[string]any
	if err := s.client.PostJSON(ctx, path, body, &raw); err != nil {
		return result, err
	}
	result.ChangedBlocks = changedBlockIDs(raw, ids)
	return result, nil
}

type readState struct {
	seen       int
	maxBlocks  int
	maxDepth   int
	includeRaw bool
	truncated  bool
	warnings   []string
}

func (s *Service) readChildren(ctx context.Context, identity DocumentIdentity, blockID string, depth int, state *readState) ([]NormalizedBlock, error) {
	if depth > state.maxDepth {
		state.truncated = true
		state.warnings = append(state.warnings, fmt.Sprintf("max depth %d reached at block %s", state.maxDepth, blockID))
		return nil, nil
	}
	if state.seen >= state.maxBlocks {
		state.truncated = true
		return nil, nil
	}

	var blocks []NormalizedBlock
	pageToken := ""
	for {
		if state.seen >= state.maxBlocks {
			state.truncated = true
			break
		}
		rawPage, err := s.listBlockChildren(ctx, identity, blockID, pageToken)
		if err != nil {
			return nil, err
		}
		children, next, hasMore := pageBlocks(rawPage)
		for _, rawBlock := range children {
			if state.seen >= state.maxBlocks {
				state.truncated = true
				break
			}
			block := normalizeBlock(identity.Provider, rawBlock, state.includeRaw)
			state.seen++
			if block.ID != "" && hasChildren(rawBlock) && depth < state.maxDepth {
				nested, err := s.readChildren(ctx, identity, block.ID, depth+1, state)
				if err != nil {
					state.warnings = append(state.warnings, fmt.Sprintf("failed to read children for block %s: %v", block.ID, err))
				} else if len(nested) > 0 {
					block.Children = nested
				}
			}
			blocks = append(blocks, block)
		}
		if !hasMore || next == "" {
			break
		}
		pageToken = next
	}
	return blocks, nil
}

func (s *Service) listBlockChildren(ctx context.Context, identity DocumentIdentity, blockID string, pageToken string) (map[string]any, error) {
	path := fmt.Sprintf(s.cfg.DocxChildrenPathTemplate, url.PathEscape(identity.Token), url.PathEscape(blockID))
	q := url.Values{}
	q.Set("page_size", "500")
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var raw map[string]any
	if err := s.client.GetJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func documentIdentityFromCreateResponse(raw map[string]any, provider string) DocumentIdentity {
	p := ProviderFeishu
	if provider == string(ProviderLark) {
		p = ProviderLark
	}
	data := asMap(raw["data"])
	doc := firstMap(data, "document", "doc")
	if len(doc) == 0 {
		doc = data
	}
	token := firstString(doc, "document_id", "documentId", "docx_token", "token")
	return DocumentIdentity{Provider: p, ResourceType: ResourceDocx, Token: token, NormalizedURL: firstString(doc, "url")}
}

func changedBlockIDs(raw map[string]any, fallback []string) []string {
	data := asMap(raw["data"])
	items := firstSlice(data, "children", "items", "blocks")
	ids := make([]string, 0, len(items))
	for _, item := range items {
		m := asMap(item)
		if id := firstString(m, "block_id", "blockId", "id"); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return ids
	}
	return fallback
}
