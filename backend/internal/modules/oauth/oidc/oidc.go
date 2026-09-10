package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"erent/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type LoginFlow struct {
	Provider  string
	UserID    uint64
	Nonce     string
	Verifier  string
	ExpiresAt time.Time
}

type OIDCAuth struct {
	Provider      *oidc.Provider
	OauthConfig   *oauth2.Config
	AuthURLParams map[string]string
	Verifier      *oidc.IDTokenVerifier
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

	var rawClaims json.RawMessage
	if err := provider.Claims(&rawClaims); err != nil {
		return nil, err
	}
	oauthConfig, err := buildOAuthConfig(OauthConfig, provider.Endpoint(), rawClaims, scopes)
	if err != nil {
		return nil, err
	}

	return &OIDCAuth{
		Provider:      provider,
		OauthConfig:   oauthConfig,
		AuthURLParams: authURLParams,
		Verifier: provider.Verifier(&oidc.Config{
			ClientID: OauthConfig.ClientID,
		}),
	}, nil
}

func buildOAuthConfig(OauthConfig config.OIDCConfig, endpoint oauth2.Endpoint, rawClaims []byte, scopes []string) (*oauth2.Config, error) {
	// Select an advertised method once; avoid retrying an authorization code
	// merely to probe whether the provider accepts Basic or body credentials.
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	if OauthConfig.ClientSecret != "" {
		var metadata struct {
			AuthMethods []string `json:"token_endpoint_auth_methods_supported"`
		}
		if err := json.Unmarshal(rawClaims, &metadata); err != nil {
			return nil, fmt.Errorf("read OIDC authentication methods: %w", err)
		}
		endpoint.AuthStyle = oauth2.AuthStyleInHeader
		if len(metadata.AuthMethods) > 0 {
			supported := false
			for _, method := range metadata.AuthMethods {
				if method == "client_secret_basic" {
					endpoint.AuthStyle = oauth2.AuthStyleInHeader
					supported = true
					break
				}
				if method == "client_secret_post" {
					endpoint.AuthStyle = oauth2.AuthStyleInParams
					supported = true
				}
			}
			if !supported {
				return nil, fmt.Errorf("OIDC provider has no supported client secret authentication method")
			}
		}
	}
	return &oauth2.Config{
		ClientID:     OauthConfig.ClientID,
		ClientSecret: OauthConfig.ClientSecret,
		RedirectURL:  OauthConfig.RedirectURL,
		Endpoint:     endpoint,
		Scopes:       scopes,
	}, nil
}
