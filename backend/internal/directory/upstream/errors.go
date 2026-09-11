package upstreamdirectory

import "errors"

var (
	ErrInvalidDeviceFlow   = errors.New("invalid device authorization request")
	ErrDeviceFlowRejected  = errors.New("device authorization rejected")
	ErrDeviceFlowFailed    = errors.New("device authorization failed")
	ErrDeviceFlowTimeout   = errors.New("device authorization timed out")
	ErrInvalidRefresh      = errors.New("invalid token refresh request")
	ErrRefreshRejected     = errors.New("refresh token rejected; reauthorization required")
	ErrRefreshFailed       = errors.New("token refresh failed")
	ErrRefreshTimeout      = errors.New("token refresh timed out")
	ErrInvalidExchange     = errors.New("invalid token exchange request")
	ErrProviderUnavailable = errors.New("oauth provider unavailable")
	ErrExchangeRejected    = errors.New("authorization code rejected")
	ErrUpstreamUnavailable = errors.New("oauth upstream unavailable")
	ErrExchangeTimeout     = errors.New("token exchange timed out")
	ErrExchangeFailed      = errors.New("token exchange failed")
	ErrInvalidIDToken      = errors.New("invalid ID token")
)
