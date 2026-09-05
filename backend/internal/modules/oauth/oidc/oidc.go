package oidc

import (
	"context"
	"time"

	"erent/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type LoginFlow struct {
	Verifier  string
	ExpiresAt time.Time
}

type OIDCAuth struct {
	OauthConfig   *oauth2.Config
	AuthURLParams map[string]string
}

func NewOIDCAuth(
	ctx context.Context,
	OauthConfig config.OIDCConfig,
	authURLParams map[string]string,
	scopes []string,
) (*OIDCAuth, error) {
	// 通过 issuer 的 /.well-known/openid-configuration
	// 自动获取 authorization、token、JWKS 等地址。
	provider, err := oidc.NewProvider(ctx, OauthConfig.Issuer)
	if err != nil {
		return nil, err
	}

	oauthConfig := &oauth2.Config{
		ClientID:     OauthConfig.ClientID,
		ClientSecret: OauthConfig.ClientSecret,
		RedirectURL:  OauthConfig.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	return &OIDCAuth{
		OauthConfig:   oauthConfig,
		AuthURLParams: authURLParams,
	}, nil
}
