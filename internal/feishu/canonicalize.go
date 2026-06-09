package feishu

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (s *Service) CanonicalizeIdentity(ctx context.Context, identity DocumentIdentity, actor ActorContext) (DocumentIdentity, error) {
	if strings.TrimSpace(identity.Token) == "" {
		return DocumentIdentity{}, newError(ErrInvalidInput, "document token is required for canonicalization", nil)
	}
	switch identity.ResourceType {
	case ResourceDocx, ResourceUnknown:
		canonical := identity
		canonical.ResourceType = ResourceDocx
		return canonical, nil
	case ResourceWiki:
		return s.canonicalizeWikiIdentity(ctx, identity, actor)
	case ResourceDriveFile:
		return s.canonicalizeDriveFileIdentity(ctx, identity, actor)
	default:
		return DocumentIdentity{}, unsupportedCanonicalIdentityError(identity.ResourceType, identity.Token)
	}
}

func (s *Service) canonicalizeWikiIdentity(ctx context.Context, identity DocumentIdentity, actor ActorContext) (DocumentIdentity, error) {
	pathTemplate := strings.TrimSpace(s.cfg.WikiNodePathTemplate)
	if pathTemplate == "" {
		pathTemplate = "/open-apis/wiki/v2/spaces/get_node?token=%s"
	}
	path := fmt.Sprintf(pathTemplate, url.QueryEscape(identity.Token))
	var raw map[string]any
	if err := s.client.GetJSONWithActor(ctx, path, nil, &raw, actor); err != nil {
		return DocumentIdentity{}, err
	}
	data := asMap(raw["data"])
	node := firstMap(data, "node", "wiki_node", "wikiNode")
	if len(node) == 0 {
		node = data
	}
	return canonicalDocxIdentityFromRaw(identity, node)
}

func (s *Service) canonicalizeDriveFileIdentity(ctx context.Context, identity DocumentIdentity, actor ActorContext) (DocumentIdentity, error) {
	pathTemplate := strings.TrimSpace(s.cfg.DriveFileMetadataPathTemplate)
	if pathTemplate == "" {
		pathTemplate = "/open-apis/drive/v1/files/%s"
	}
	path := fmt.Sprintf(pathTemplate, url.PathEscape(identity.Token))
	var raw map[string]any
	if err := s.client.GetJSONWithActor(ctx, path, nil, &raw, actor); err != nil {
		return DocumentIdentity{}, err
	}
	data := asMap(raw["data"])
	file := firstMap(data, "file", "metadata", "meta")
	if len(file) == 0 {
		file = data
	}
	return canonicalDocxIdentityFromRaw(identity, file)
}

func canonicalDocxIdentityFromRaw(original DocumentIdentity, raw map[string]any) (DocumentIdentity, error) {
	resourceType := strings.ToLower(firstString(raw, "obj_type", "objType", "resource_type", "resourceType", "type", "file_type", "fileType"))
	docxToken := firstString(raw, "docx_token", "docxToken")
	token := docxToken
	if resourceType == string(ResourceDocx) {
		token = firstNonEmpty(docxToken, firstString(raw, "obj_token", "objToken", "document_id", "documentId", "token", "file_token", "fileToken"))
	}
	if resourceType != "" && resourceType != string(ResourceDocx) {
		return DocumentIdentity{}, unsupportedCanonicalIdentityError(ResourceType(resourceType), firstNonEmpty(token, firstString(raw, "obj_token", "objToken", "document_id", "documentId", "token", "file_token", "fileToken"), original.Token))
	}
	if strings.TrimSpace(token) == "" {
		return DocumentIdentity{}, newError(ErrUnsupportedDocumentType, fmt.Sprintf("unable to canonicalize %s token %s to a docx document; upstream response did not include a docx token", original.ResourceType, original.Token), nil)
	}
	canonical := original
	canonical.ResourceType = ResourceDocx
	canonical.Token = token
	if urlValue := firstString(raw, "url", "doc_url", "docUrl"); urlValue != "" {
		canonical.NormalizedURL = urlValue
	}
	return canonical, nil
}

func unsupportedCanonicalIdentityError(resourceType ResourceType, token string) *ConnectorError {
	return newError(ErrUnsupportedDocumentType, fmt.Sprintf("unsupported resource type %s for token %s; only docx documents can be read, permission-checked, or commented after canonicalization", resourceType, token), nil)
}
