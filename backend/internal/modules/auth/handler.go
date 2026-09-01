package auth

import (
	"errors"
	"net/http"

	"github.com/DWHuang99/erent/internal/dto/request"
	"github.com/DWHuang99/erent/internal/dto/response"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service      *AuthService
	cookieSecure bool
}

func NewAuthHandler(service *AuthService, cookieSecure bool) *AuthHandler {
	return &AuthHandler{service: service, cookieSecure: cookieSecure}
}

const refreshCookiePath = "/api/v1/auth"

func (h *AuthHandler) setRefreshCookie(c *gin.Context, refreshToken string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token",
		refreshToken,
		int(h.service.RefreshTTL().Seconds()),
		refreshCookiePath,
		"",
		h.cookieSecure,
		true,
	)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, refreshCookiePath, "", h.cookieSecure, true)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var loginRequest request.LoginRequest
	if err := c.ShouldBindJSON(&loginRequest); err != nil {
		response.Error(c, http.StatusBadRequest, 40000, "invalid request")
		return
	}

	accessToken, refreshToken, exists, err := h.service.Login(c.Request.Context(), loginRequest)
	switch {
	case err == nil && exists:
		h.setRefreshCookie(c, refreshToken)
		response.Success(c, gin.H{
			"message":     "Login successful",
			"accessToken": accessToken,
			"exist":       true,
		})
	case errors.Is(err, ErrUserDisabled):
		response.Error(c, http.StatusForbidden, 40301, "user is disabled")
	case err != nil:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	default:
		response.Error(c, http.StatusUnauthorized, 40100, "invalid username or password")
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var registerRequest request.RegisterRequest
	if err := c.ShouldBindJSON(&registerRequest); err != nil {
		response.Error(c, http.StatusBadRequest, 10001, "invalid request")
		return
	}

	_, err := h.service.Register(c.Request.Context(), registerRequest)
	switch {
	case err == nil:
		response.SuccessWithStatus(c, http.StatusCreated, nil, "register successful")
	case errors.Is(err, ErrInvalidRequest):
		response.Error(c, http.StatusBadRequest, 10001, err.Error())
	case errors.Is(err, ErrUserExists):
		response.Error(c, http.StatusConflict, 10002, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, 10500, "internal server error")
	}
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		response.Error(c, http.StatusUnauthorized, 40101, "missing refresh token")
		return
	}
	accessToken, newRefreshToken, err := h.service.Refresh(c.Request.Context(), refreshToken)
	switch {
	case err == nil:
		h.setRefreshCookie(c, newRefreshToken)
		response.Success(c, gin.H{"accessToken": accessToken})
	case errors.Is(err, ErrInvalidRefreshToken):
		h.clearRefreshCookie(c)
		response.Error(c, http.StatusUnauthorized, 40101, "invalid or expired refresh token")
	case errors.Is(err, ErrUserDisabled):
		h.clearRefreshCookie(c)
		response.Error(c, http.StatusForbidden, 40301, "user is disabled")
	default:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	}
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err == nil && refreshToken != "" {
		if err := h.service.Logout(c.Request.Context(), refreshToken); err != nil {
			h.clearRefreshCookie(c)
			response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
			return
		}
	}
	h.clearRefreshCookie(c)
	response.Success(c, nil)
}
