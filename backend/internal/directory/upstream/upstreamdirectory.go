package upstreamdirectory

import (
	"context"
	"strings"
	"time"

	"erent/internal/rpc/upstream"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Directory struct {
	client  upstream.UpstreamServiceClient
	timeout time.Duration
}

func New(client upstream.UpstreamServiceClient, timeout time.Duration) *Directory {
	return &Directory{client: client, timeout: timeout}
}

func (d *Directory) GetProvider(ctx context.Context, issuer string) (*upstream.ProviderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	response, err := d.client.GetProvider(ctx, &upstream.ProviderRequest{Issuer: issuer})
	if err != nil {
		return nil, providerError(err)
	}
	if response == nil {
		return nil, ErrProviderUnavailable
	}
	return response, nil
}

func (d *Directory) Verifier(ctx context.Context, rawIDToken, provider string) (*upstream.VerifyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	response, err := d.client.Verifier(ctx, &upstream.VerifyRequest{Rawidtoken: rawIDToken, Provider: provider})
	if err != nil {
		return nil, providerError(err)
	}
	if response == nil {
		return nil, ErrInvalidIDToken
	}
	return response, nil
}

func providerError(err error) error {
	switch status.Code(err) {
	case codes.FailedPrecondition, codes.NotFound:
		return ErrProviderUnavailable
	case codes.InvalidArgument, codes.Unauthenticated:
		return ErrInvalidIDToken
	case codes.Canceled:
		return context.Canceled
	default:
		return ErrUpstreamUnavailable
	}
}

func (d *Directory) Exchange(ctx context.Context, code, verifier, provider string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	response, err := d.client.ExchangeCode(ctx, &upstream.ExchangeCodeRequest{Code: code, CodeVerifier: verifier, Provider: provider})
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil, ErrInvalidExchange
		case codes.FailedPrecondition, codes.NotFound:
			return nil, ErrProviderUnavailable
		case codes.Unauthenticated:
			return nil, ErrExchangeRejected
		case codes.Unavailable:
			return nil, ErrUpstreamUnavailable
		case codes.DeadlineExceeded:
			return nil, ErrExchangeTimeout
		case codes.Canceled:
			return nil, context.Canceled
		default:
			return nil, ErrExchangeFailed
		}
	}
	if response == nil || response.AccessToken == "" {
		return nil, ErrExchangeFailed
	}
	token := &oauth2.Token{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, TokenType: response.TokenType}
	if response.ExpiresAt != nil {
		if err := response.ExpiresAt.CheckValid(); err != nil {
			return nil, ErrExchangeFailed
		}
		token.Expiry = response.ExpiresAt.AsTime()
	}
	return token.WithExtra(map[string]any{"id_token": response.IdToken}), nil
}

func (d *Directory) RefreshToken(ctx context.Context, refreshtoken string, provider string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	response, err := d.client.RefreshToken(ctx, &upstream.RefreshTokenRequest{RefreshToken: refreshtoken, Provider: provider})
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil, ErrInvalidRefresh
		case codes.FailedPrecondition, codes.NotFound:
			return nil, ErrProviderUnavailable
		case codes.Unauthenticated:
			return nil, ErrRefreshRejected
		case codes.Unavailable:
			return nil, ErrUpstreamUnavailable
		case codes.DeadlineExceeded:
			return nil, ErrRefreshTimeout
		case codes.Canceled:
			return nil, context.Canceled
		default:
			return nil, ErrRefreshFailed
		}
	}
	if response == nil || strings.TrimSpace(response.AccessToken) == "" {
		return nil, ErrRefreshFailed
	}
	token := &oauth2.Token{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, TokenType: response.TokenType}
	if response.ExpiresAt != nil {
		if err := response.ExpiresAt.CheckValid(); err != nil {
			return nil, ErrRefreshFailed
		}
		token.Expiry = response.ExpiresAt.AsTime()
	}
	return token.WithExtra(map[string]any{"id_token": response.IdToken}), nil
}
