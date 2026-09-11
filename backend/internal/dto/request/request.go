// Package request contains HTTP request DTOs.
package request

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
