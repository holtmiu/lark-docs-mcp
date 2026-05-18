package feishu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func (s *Service) AppendDocument(ctx context.Context, input string, req AppendRequest) (DocumentWriteResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return DocumentWriteResult{}, err
	}
	dryRun := s.cfg.WriteDryRunDefault
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID(identity.Token, req.Markdown)
	}

	result := DocumentWriteResult{
		OperationID: operationID,
		DocumentID:  identity.Token,
		DryRun:      dryRun,
		Warnings:    []string{},
	}
	if dryRun {
		result.Warnings = append(result.Warnings, "dry-run only: no content was written to Feishu/Lark")
		return result, nil
	}
	return result, newError(ErrUnsupportedDocumentType, "real write execution is intentionally not implemented in this MVP; use dryRun=true", nil)
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
			childID := block.ID
			if childID != "" && depth < state.maxDepth {
				nested, err := s.readChildren(ctx, identity, childID, depth+1, state)
				if err != nil {
					state.warnings = append(state.warnings, fmt.Sprintf("failed to read children for block %s: %v", childID, err))
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

func defaultOperationID(documentID, content string) string {
	h := sha256.Sum256([]byte(documentID + "\x00" + content))
	return hex.EncodeToString(h[:16])
}
