package oidc

import (
	"context"
	"errors"
	"net/url"

	"erent/internal/config"
	upstreamdirectory "erent/internal/directory/upstream"

	"golang.org/x/oauth2"
)

func NewRemoteOIDCAuth(ctx context.Context, cfg config.OIDCConfig, directory *upstreamdirectory.Directory, params map[string]string, scopes []string) (*OIDCAuth, error) {
	if directory == nil {
		return nil, errors.New("upstream directory is required")
	}
	response, err := directory.GetProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Issuer != cfg.Issuer {
		return nil, errors.New("upstream issuer mismatch")
	}
	for _, endpoint := range []string{response.AuthURL, response.TokenURL} {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, errors.New("invalid upstream endpoint")
		}
	}
	oauthConfig, err := buildOAuthConfig(cfg, oauth2.Endpoint{AuthURL: response.AuthURL, TokenURL: response.TokenURL, DeviceAuthURL: response.DeviceAuthURL}, response.RawClaims, scopes)
	if err != nil {
		return nil, err
	}
	return &OIDCAuth{OauthConfig: oauthConfig, AuthURLParams: params}, nil
}
