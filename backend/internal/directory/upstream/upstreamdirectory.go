package upstreamdirectory

import (
	"context"
	"time"

	"erent/internal/modules/oauth"
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
func (d *Directory) Exchange(ctx context.Context, code, verifier, provider string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	response, err := d.client.ExchangeCode(ctx, &upstream.ExchangeCodeRequest{Code: code, CodeVerifier: verifier, Provider: provider})
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil, oauth.ErrInvalidExchange
		case codes.FailedPrecondition, codes.NotFound:
			return nil, oauth.ErrProviderUnavailable
		case codes.Unauthenticated:
			return nil, oauth.ErrExchangeRejected
		case codes.Unavailable:
			return nil, oauth.ErrUpstreamUnavailable
		case codes.DeadlineExceeded:
			return nil, oauth.ErrExchangeTimeout
		case codes.Canceled:
			return nil, context.Canceled
		default:
			return nil, oauth.ErrExchangeFailed
		}
	}
	if response == nil || response.AccessToken == "" {
		return nil, oauth.ErrExchangeFailed
	}
	token := &oauth2.Token{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, TokenType: response.TokenType}
	if response.ExpiresAt != nil {
		if err := response.ExpiresAt.CheckValid(); err != nil {
			return nil, oauth.ErrExchangeFailed
		}
		token.Expiry = response.ExpiresAt.AsTime()
	}
	return token, nil
}
