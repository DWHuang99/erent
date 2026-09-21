// Package request contains HTTP request DTOs.
package request

import "encoding/json"

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type RegisterRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	CheckPassword string `json:"check_password"`
	Code          string `json:"code"`
	IAgree        bool   `json:"iAgree"`
}

// OAuthRefreshRequest identifies a credential owned by the authenticated user.
type OAuthRefreshRequest struct {
	ID uint64 `json:"id" binding:"required"`
}

type OAuthPollRequest struct {
	DeviceAuthID string `json:"device_auth_id" binding:"required"`
	UserCode     string `json:"user_code"`
	Interval     uint32 `json:"interval"`
}

type ChatRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type CreatApikeyRequest struct {
	OAuthInfo         []uint64 `json:"oauth_info"`
	ExternalApiKeyIDs []uint64 `json:"external_api_key_ids"`
}

type UpdateApikeyRequest struct {
	Disabled  *bool           `json:"disabled"`
	ExpiresAt json.RawMessage `json:"expires_at"`
}

type ReplaceApikeyAccountsRequest struct {
	OAuthInfo         *[]uint64 `json:"oauth_info" binding:"required"`
	ExternalApiKeyIDs *[]uint64 `json:"external_api_key_ids" binding:"required"`
}

type ExternalApikeyRequest struct {
	ExternalApikey string               `json:"external_apikey" binding:"required"`
	Endpoint       string               `json:"endpoint" binding:"required"`
	Suffix         ExternalApikeySuffix `json:"suffix"`
}

type ExternalApikeySuffix struct {
	ChatCompletions string `json:"chat_completions"`
	Responses       string `json:"responses"`
	Messages        string `json:"messages"`
}
