package feishu

import (
	"context"
	"strings"
)

type TokenSource interface {
	Token(ctx context.Context, actor ActorContext) (token string, source string, err error)
}

type TenantTokenSource struct {
	Client *HTTPClient
}

func (s TenantTokenSource) Token(ctx context.Context, actor ActorContext) (string, string, error) {
	if s.Client == nil {
		return "", "", newError(ErrInvalidInput, "tenant token source requires HTTP client", nil)
	}
	token, err := s.Client.TenantToken(ctx)
	if err != nil {
		return "", "", err
	}
	return token, "tenant", nil
}

type UserFirstTokenSource struct {
	Refresher *UserTokenRefresher
	Tenant    TokenSource
}

func (s UserFirstTokenSource) Token(ctx context.Context, actor ActorContext) (string, string, error) {
	credentialID := strings.TrimSpace(actor.CredentialID)
	if credentialID == "" {
		if s.Tenant == nil {
			return "", "", newError(ErrAuthRequired, "tenant token source is required", nil)
		}
		return s.Tenant.Token(ctx, actor)
	}
	if s.Refresher == nil {
		return "", "", newError(ErrAuthRequired, "user credential store is not configured", nil)
	}
	binding, err := s.Refresher.Credential(ctx, credentialID)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(binding.AccessToken) == "" {
		return "", "", newError(ErrAuthRequired, "user credential is missing access token", nil)
	}
	return binding.AccessToken, "user", nil
}
