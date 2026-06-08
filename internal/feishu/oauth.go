package feishu

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

const OAuthScopeMaxLength = 256

var ErrInvalidOAuthScope = errors.New("invalid oauth scope")

type OAuthAuthURLRequest struct {
	RedirectURI string
	State       string
	Scopes      []string
}

type OAuthAuthURLResult struct {
	URL         string   `json:"url"`
	Provider    Provider `json:"provider"`
	Scopes      []string `json:"scopes"`
	RedirectURI string   `json:"redirectUri"`
}

// BuildOAuthAuthURL builds an OAuth authorization URL after validating syntactic
// URL safety for the provided base URL and redirect URI. This generic builder
// does not enforce the service's configured redirect URI lock; service and MCP
// callers must use Service.BuildOAuthAuthURL so the configured redirect URI is
// required and caller-supplied redirect URIs cannot bypass that lock.
func BuildOAuthAuthURL(provider Provider, baseURL, appID, authPath string, req OAuthAuthURLRequest) (OAuthAuthURLResult, error) {
	redirectURI := strings.TrimSpace(req.RedirectURI)
	if redirectURI == "" {
		return OAuthAuthURLResult{}, errors.New("oauth redirect URI is required")
	}
	if err := validateOAuthRedirectURI(redirectURI); err != nil {
		return OAuthAuthURLResult{}, err
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return OAuthAuthURLResult{}, errors.New("oauth app id is required")
	}

	base, err := validateOAuthBaseURL(baseURL)
	if err != nil {
		return OAuthAuthURLResult{}, err
	}
	base.Path = joinURLPath(base.Path, authPath)
	base.RawQuery = ""
	base.Fragment = ""

	scopes, err := validateOAuthScopes(req.Scopes)
	if err != nil {
		return OAuthAuthURLResult{}, err
	}
	q := base.Query()
	q.Set("app_id", appID)
	q.Set("redirect_uri", redirectURI)
	if state := strings.TrimSpace(req.State); state != "" {
		q.Set("state", state)
	}
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	base.RawQuery = q.Encode()

	return OAuthAuthURLResult{URL: base.String(), Provider: provider, Scopes: scopes, RedirectURI: redirectURI}, nil
}

func (s *Service) BuildOAuthAuthURL(req OAuthAuthURLRequest) (OAuthAuthURLResult, error) {
	provider := ProviderFeishu
	if strings.EqualFold(s.cfg.Provider, string(ProviderLark)) {
		provider = ProviderLark
	}
	configuredRedirectURI := strings.TrimSpace(s.cfg.OAuthRedirectURI)
	requestedRedirectURI := strings.TrimSpace(req.RedirectURI)
	if configuredRedirectURI == "" {
		return OAuthAuthURLResult{}, errors.New("oauth redirect URI must be configured")
	}
	if requestedRedirectURI != "" && requestedRedirectURI != configuredRedirectURI {
		return OAuthAuthURLResult{}, errors.New("oauth redirect URI must match configured redirect URI")
	}
	req.RedirectURI = configuredRedirectURI
	cleanedScopes, err := validateOAuthScopes(req.Scopes)
	if err != nil {
		return OAuthAuthURLResult{}, err
	}
	if len(cleanedScopes) == 0 {
		req.Scopes = s.cfg.OAuthScopes
	} else {
		req.Scopes = cleanedScopes
	}
	return BuildOAuthAuthURL(provider, s.cfg.BaseURL, s.cfg.AppID, s.cfg.OAuthAuthPath, req)
}

func validateOAuthRedirectURI(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid oauth redirect URI: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("oauth redirect URI must include scheme and host")
	}
	if parsed.Fragment != "" {
		return errors.New("oauth redirect URI must not include fragment")
	}
	if !isSecureOAuthURL(parsed) {
		return errors.New("oauth redirect URI must use https except for localhost development")
	}
	return nil
}

func validateOAuthBaseURL(raw string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, errors.New("oauth base URL must include scheme and host")
	}
	if !isSecureOAuthURL(base) {
		return nil, errors.New("oauth base URL must use https except for localhost development")
	}
	return base, nil
}

func isSecureOAuthURL(parsed *url.URL) bool {
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "https" {
		return true
	}
	return scheme == "http" && isLocalOAuthHost(parsed.Hostname())
}

func isLocalOAuthHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func cleanScopes(scopes []string) []string {
	cleaned, _ := validateOAuthScopes(scopes)
	return cleaned
}

func validateOAuthScopes(scopes []string) ([]string, error) {
	cleaned := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > OAuthScopeMaxLength {
			return nil, fmt.Errorf("%w: scope exceeds max length %d", ErrInvalidOAuthScope, OAuthScopeMaxLength)
		}
		for _, r := range trimmed {
			if unicode.IsSpace(r) || unicode.IsControl(r) {
				return nil, fmt.Errorf("%w: scope must not contain whitespace or control characters", ErrInvalidOAuthScope)
			}
		}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned, nil
}

func joinURLPath(basePath, childPath string) string {
	basePath = strings.TrimRight(strings.TrimSpace(basePath), "/")
	childPath = strings.TrimLeft(strings.TrimSpace(childPath), "/")
	if childPath == "" {
		if basePath == "" {
			return "/"
		}
		return basePath
	}
	return basePath + "/" + childPath
}
