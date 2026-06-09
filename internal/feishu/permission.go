package feishu

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (s *Service) CheckPermission(ctx context.Context, input string) (PermissionSnapshot, error) {
	return s.CheckPermissionWithActor(ctx, input, ActorContext{})
}

func (s *Service) CheckPermissionWithActor(ctx context.Context, input string, actor ActorContext) (PermissionSnapshot, error) {
	identity, err := s.Resolve(input)
	if err != nil {
		return PermissionSnapshot{}, err
	}
	return s.CheckPermissionByIdentityWithActor(ctx, identity, actor)
}

func (s *Service) CheckPermissionByIdentityWithActor(ctx context.Context, identity DocumentIdentity, actor ActorContext) (PermissionSnapshot, error) {
	return s.checkPermissionByIdentityWithActor(ctx, identity, actor, true)
}

func (s *Service) checkPermissionByIdentityWithActor(ctx context.Context, identity DocumentIdentity, actor ActorContext, canonicalize bool) (PermissionSnapshot, error) {
	if canonicalize {
		var err error
		identity, err = s.CanonicalizeIdentity(ctx, identity, actor)
		if err != nil {
			return PermissionSnapshot{}, err
		}
	}
	if strings.TrimSpace(identity.Token) == "" {
		return PermissionSnapshot{}, newError(ErrInvalidInput, "document token is required for permission check", nil)
	}
	pathTemplate := strings.TrimSpace(s.cfg.DocxPermissionPathTemplate)
	if !canonicalize && identity.ResourceType == ResourceDriveFile {
		pathTemplate = strings.TrimSpace(s.cfg.FolderPermissionPathTemplate)
	}
	if pathTemplate == "" {
		pathTemplate = "/open-apis/drive/v1/permissions/%s/public?type=docx"
		if !canonicalize && identity.ResourceType == ResourceDriveFile {
			pathTemplate = "/open-apis/drive/v1/permissions/%s/public?type=folder"
		}
	}
	path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token))
	var raw map[string]any
	if err := s.client.GetJSONWithActor(ctx, path, nil, &raw, actor); err != nil {
		return PermissionSnapshot{}, err
	}
	return permissionSnapshotFromRaw(raw), nil
}

func permissionSnapshotFromRaw(raw map[string]any) PermissionSnapshot {
	data := asMap(raw["data"])
	perm := firstMap(data, "permission", "permissions", "capability", "capabilities")
	if len(perm) == 0 {
		perm = data
	}
	if len(perm) == 0 {
		perm = raw
	}

	snapshot := PermissionSnapshot{
		CanRead:        firstBool(perm, "can_read", "canRead", "readable", "can_view", "canView"),
		CanWrite:       firstBool(perm, "can_write", "canWrite", "editable", "can_edit", "canEdit"),
		CanComment:     firstBool(perm, "can_comment", "canComment", "commentable", "can_add_comment", "canAddComment"),
		Visibility:     firstString(perm, "visibility", "share_level", "shareLevel"),
		Reason:         firstString(perm, "reason", "deny_reason", "denyReason", "msg", "message"),
		RequiredScopes: firstStringSlice(perm, "required_scopes", "requiredScopes", "scopes"),
	}
	if !snapshot.CanRead || !snapshot.CanWrite || !snapshot.CanComment {
		snapshot.SuggestedAction = firstString(perm, "suggested_action", "suggestedAction")
		if snapshot.SuggestedAction == "" {
			snapshot.SuggestedAction = "Ask the document owner to grant the required read/write/comment permission or authorize with a credential that has access."
		}
	}
	return snapshot
}

func permissionDeniedError(operation string, snapshot PermissionSnapshot) *ConnectorError {
	message := strings.TrimSpace(snapshot.Reason)
	if message == "" {
		message = strings.TrimSpace(snapshot.SuggestedAction)
	}
	if message == "" {
		message = "permission denied"
	}
	return newError(ErrPermissionDenied, fmt.Sprintf("%s requires write permission: %s", operation, message), nil)
}

func firstBool(parent map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch v := parent[key].(type) {
		case bool:
			return v
		case string:
			lower := strings.ToLower(strings.TrimSpace(v))
			if lower == "true" || lower == "1" || lower == "yes" || lower == "allow" || lower == "allowed" {
				return true
			}
			if lower == "false" || lower == "0" || lower == "no" || lower == "deny" || lower == "denied" {
				return false
			}
		case float64:
			return v != 0
		case int:
			return v != 0
		}
	}
	return false
}

func firstStringSlice(parent map[string]any, keys ...string) []string {
	for _, key := range keys {
		switch v := parent[key].(type) {
		case []string:
			return append([]string(nil), v...)
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				if s := scalarString(item); strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if strings.TrimSpace(v) != "" {
				parts := strings.Split(v, ",")
				out := make([]string, 0, len(parts))
				for _, part := range parts {
					if item := strings.TrimSpace(part); item != "" {
						out = append(out, item)
					}
				}
				return out
			}
		}
	}
	return nil
}
