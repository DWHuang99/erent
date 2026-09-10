package oauth

import (
	"errors"

	upstreamdirectory "erent/internal/directory/upstream"
)

var (
	ErrOAuthNotFound       = errors.New("oauth credential not found")
	ErrInvalidRefresh      = upstreamdirectory.ErrInvalidRefresh
	ErrRefreshRejected     = upstreamdirectory.ErrRefreshRejected
	ErrRefreshFailed       = upstreamdirectory.ErrRefreshFailed
	ErrRefreshTimeout      = upstreamdirectory.ErrRefreshTimeout
	ErrInvalidExchange     = upstreamdirectory.ErrInvalidExchange
	ErrProviderUnavailable = upstreamdirectory.ErrProviderUnavailable
	ErrExchangeRejected    = upstreamdirectory.ErrExchangeRejected
	ErrUpstreamUnavailable = upstreamdirectory.ErrUpstreamUnavailable
	ErrExchangeTimeout     = upstreamdirectory.ErrExchangeTimeout
	ErrExchangeFailed      = upstreamdirectory.ErrExchangeFailed
	ErrInvalidIDToken      = upstreamdirectory.ErrInvalidIDToken
)
