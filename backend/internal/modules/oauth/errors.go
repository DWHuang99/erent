package oauth

import "errors"

var (
	ErrInvalidExchange     = errors.New("invalid token exchange request")
	ErrProviderUnavailable = errors.New("oauth provider unavailable")
	ErrExchangeRejected    = errors.New("authorization code rejected")
	ErrUpstreamUnavailable = errors.New("oauth upstream unavailable")
	ErrExchangeTimeout     = errors.New("token exchange timed out")
	ErrExchangeFailed      = errors.New("token exchange failed")
)
