package feishu

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const maxCommentContentLength = 20000

func (req CreateCommentRequest) Validate() error {
	return validateCommentContent(req.Content)
}

func (req ReplyCommentRequest) Validate() error {
	return validateCommentContent(req.Content)
}

func validateCommentContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return newError(ErrInvalidInput, "comment content is required", nil)
	}
	if len([]rune(trimmed)) > maxCommentContentLength {
		return newError(ErrInvalidInput, fmt.Sprintf("comment content exceeds %d characters", maxCommentContentLength), nil)
	}
	return nil
}

func (s *Service) ListComments(ctx context.Context, input string, req ListCommentsRequest, actor ActorContext) (CommentListResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return CommentListResult{}, err
	}
	pathTemplate := strings.TrimSpace(s.cfg.DocxCommentsPathTemplate)
	if pathTemplate == "" {
		pathTemplate = "/open-apis/drive/v1/files/%s/comments"
	}
	path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token))
	query := url.Values{}
	if req.PageSize > 0 {
		query.Set("page_size", fmt.Sprintf("%d", req.PageSize))
	}
	if strings.TrimSpace(req.PageToken) != "" {
		query.Set("page_token", strings.TrimSpace(req.PageToken))
	}
	var raw map[string]any
	if err := s.client.GetJSONWithActor(ctx, path, query, &raw, actor); err != nil {
		return CommentListResult{}, err
	}
	result := commentListFromRaw(identity.Token, raw)
	return result, nil
}

func (s *Service) CreateComment(ctx context.Context, input string, req CreateCommentRequest, actor ActorContext) (CommentWriteResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return CommentWriteResult{}, err
	}
	if err := req.Validate(); err != nil {
		return CommentWriteResult{}, err
	}
	body := buildCreateCommentBody(req)
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID("comment-create", identity.Token, req.BlockID, req.Content, req.Quote)
	}
	return s.executeCommentMutation(ctx, identity, "create comment", operationID, req.DryRun, body, actor, func() (map[string]any, error) {
		pathTemplate := strings.TrimSpace(s.cfg.DocxCommentsPathTemplate)
		if pathTemplate == "" {
			pathTemplate = "/open-apis/drive/v1/files/%s/comments"
		}
		path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token))
		var raw map[string]any
		err := s.client.PostJSONWithActor(ctx, path, body, &raw, actor)
		return raw, err
	})
}

func (s *Service) ReplyComment(ctx context.Context, input string, commentID string, req ReplyCommentRequest, actor ActorContext) (CommentWriteResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return CommentWriteResult{}, err
	}
	commentID = strings.TrimSpace(commentID)
	if commentID == "" {
		return CommentWriteResult{}, newError(ErrInvalidInput, "comment id is required", nil)
	}
	if err := req.Validate(); err != nil {
		return CommentWriteResult{}, err
	}
	body := map[string]any{"content": strings.TrimSpace(req.Content)}
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID("comment-reply", identity.Token, commentID, req.Content)
	}
	return s.executeCommentMutation(ctx, identity, "reply to comment", operationID, req.DryRun, body, actor, func() (map[string]any, error) {
		pathTemplate := strings.TrimSpace(s.cfg.DocxCommentRepliesPathTemplate)
		if pathTemplate == "" {
			pathTemplate = "/open-apis/drive/v1/files/%s/comments/%s/replies"
		}
		path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token), url.PathEscape(commentID))
		var raw map[string]any
		err := s.client.PostJSONWithActor(ctx, path, body, &raw, actor)
		return raw, err
	})
}

func (s *Service) ResolveComment(ctx context.Context, input string, commentID string, req ResolveCommentRequest, actor ActorContext) (CommentWriteResult, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return CommentWriteResult{}, err
	}
	commentID = strings.TrimSpace(commentID)
	if commentID == "" {
		return CommentWriteResult{}, newError(ErrInvalidInput, "comment id is required", nil)
	}
	body := map[string]any{"resolved": req.Resolved}
	operationID := strings.TrimSpace(req.OperationID)
	if operationID == "" {
		operationID = defaultOperationID("comment-resolve", identity.Token, commentID, fmt.Sprintf("%t", req.Resolved))
	}
	return s.executeCommentMutation(ctx, identity, "resolve comment", operationID, req.DryRun, body, actor, func() (map[string]any, error) {
		pathTemplate := strings.TrimSpace(s.cfg.DocxCommentResolvePathTemplate)
		if pathTemplate == "" {
			pathTemplate = "/open-apis/drive/v1/files/%s/comments/%s"
		}
		path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token), url.PathEscape(commentID))
		var raw map[string]any
		err := s.client.doJSON(ctx, http.MethodPatch, path, nil, body, &raw, true, actor)
		return raw, err
	})
}

func (s *Service) executeCommentMutation(ctx context.Context, identity DocumentIdentity, operation string, operationID string, dryRunOverride *bool, body map[string]any, actor ActorContext, mutate func() (map[string]any, error)) (CommentWriteResult, error) {
	dryRun := writeDryRun(s.cfg.WriteDryRunDefault, dryRunOverride)
	result := CommentWriteResult{OperationID: operationID, DocumentID: identity.Token, DryRun: dryRun, Request: body, Warnings: []string{}}
	if dryRun {
		result.Warnings = append(result.Warnings, "dry-run only: no comment mutation was sent to Feishu/Lark")
		return result, nil
	}
	permission, err := s.CheckPermissionByIdentityWithActor(ctx, identity, actor)
	if err != nil {
		return result, err
	}
	if !permission.CanComment {
		return result, permissionDeniedError(operation, permission)
	}
	raw, err := mutate()
	if err != nil {
		return result, err
	}
	comment := commentFromRaw(firstNonEmptyMap(raw, "comment", "reply", "data"))
	if comment.ID == "" && comment.Content == "" {
		comment = commentFromRaw(raw)
	}
	result.Comment = comment
	result.CommentID = comment.ID
	return result, nil
}

func buildCreateCommentBody(req CreateCommentRequest) map[string]any {
	body := map[string]any{"content": strings.TrimSpace(req.Content)}
	if strings.TrimSpace(req.BlockID) != "" {
		body["block_id"] = strings.TrimSpace(req.BlockID)
	}
	if strings.TrimSpace(req.Quote) != "" {
		body["quote"] = strings.TrimSpace(req.Quote)
	}
	return body
}

func commentListFromRaw(documentID string, raw map[string]any) CommentListResult {
	data := asMap(raw["data"])
	if len(data) == 0 {
		data = raw
	}
	items := firstSlice(data, "items", "comments", "comment_list")
	comments := make([]Comment, 0, len(items))
	for _, item := range items {
		if comment := commentFromRaw(asMap(item)); comment.ID != "" || comment.Content != "" {
			comments = append(comments, comment)
		}
	}
	return CommentListResult{
		DocumentID: documentID,
		Comments:   comments,
		HasMore:    firstBool(data, "has_more", "hasMore"),
		PageToken:  firstString(data, "page_token", "pageToken", "next_page_token", "nextPageToken"),
	}
}

func commentFromRaw(raw map[string]any) Comment {
	if len(raw) == 0 {
		return Comment{}
	}
	return Comment{
		ID:          firstString(raw, "comment_id", "commentId", "reply_id", "replyId", "id"),
		Content:     firstString(raw, "content", "text", "comment"),
		AuthorID:    firstString(raw, "author_id", "authorId", "user_id", "userId", "open_id", "openId"),
		CreatedTime: firstString(raw, "created_time", "createdTime", "create_time", "createTime"),
		UpdatedTime: firstString(raw, "updated_time", "updatedTime", "update_time", "updateTime"),
		Resolved:    firstBool(raw, "resolved", "is_resolved", "isResolved", "is_solved", "isSolved"),
		Quote:       firstString(raw, "quote", "quoted_text", "quotedText"),
	}
}

func firstNonEmptyMap(raw map[string]any, keys ...string) map[string]any {
	data := asMap(raw["data"])
	for _, parent := range []map[string]any{data, raw} {
		if len(parent) == 0 {
			continue
		}
		for _, key := range keys {
			if m := asMap(parent[key]); len(m) > 0 {
				return m
			}
		}
	}
	return nil
}
