package feishu

import (
	"net/url"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,}$`)

type Resolver struct {
	DefaultProvider Provider
}

func NewResolver(provider string) Resolver {
	p := ProviderFeishu
	if strings.EqualFold(provider, string(ProviderLark)) {
		p = ProviderLark
	}
	return Resolver{DefaultProvider: p}
}

func (r Resolver) Resolve(input string) (DocumentIdentity, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return DocumentIdentity{}, newError(ErrInvalidInput, "document input is empty", nil)
	}

	if tokenPattern.MatchString(raw) && !strings.Contains(raw, "://") {
		return DocumentIdentity{Provider: r.DefaultProvider, ResourceType: ResourceDocx, Token: raw}, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return DocumentIdentity{}, newError(ErrInvalidInput, "input is neither a token nor a valid URL", err)
	}

	provider := r.providerFromHost(parsed.Host)
	resourceType, token := resourceFromPath(parsed.Path)
	if token == "" {
		token = firstQueryValue(parsed, "token", "document_id", "file_token", "wiki_token")
	}
	if token == "" {
		return DocumentIdentity{}, newError(ErrInvalidInput, "unable to extract document token from URL", nil)
	}

	normalized := url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: parsed.Path}
	return DocumentIdentity{
		Provider:      provider,
		ResourceType:  resourceType,
		Token:         token,
		OriginalURL:   raw,
		NormalizedURL: normalized.String(),
	}, nil
}

func (r Resolver) providerFromHost(host string) Provider {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "larksuite.com") || strings.Contains(h, "larksuite.cn"):
		return ProviderLark
	case strings.Contains(h, "feishu.cn"):
		return ProviderFeishu
	default:
		return r.DefaultProvider
	}
}

func resourceFromPath(path string) (ResourceType, string) {
	parts := splitPath(path)
	for i, part := range parts {
		switch part {
		case "docx", "docs", "doc":
			if i+1 < len(parts) {
				return ResourceDocx, parts[i+1]
			}
		case "wiki":
			if i+1 < len(parts) {
				return ResourceWiki, parts[i+1]
			}
		case "file":
			if i > 0 && parts[i-1] == "drive" && i+1 < len(parts) {
				return ResourceDriveFile, parts[i+1]
			}
		}
	}
	if len(parts) > 0 {
		return ResourceUnknown, parts[len(parts)-1]
	}
	return ResourceUnknown, ""
}

func splitPath(path string) []string {
	rawParts := strings.Split(path, "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func firstQueryValue(u *url.URL, names ...string) string {
	q := u.Query()
	for _, name := range names {
		if value := strings.TrimSpace(q.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
