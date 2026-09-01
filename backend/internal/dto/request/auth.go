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
