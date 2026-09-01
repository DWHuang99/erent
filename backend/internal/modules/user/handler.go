package user

import (
	"context"
	"errors"
	"net/http"

	"erent/internal/dto/response"
	jwtservice "erent/internal/middleware/jwt"

	"github.com/gin-gonic/gin"
)

type CurrentUserService interface {
	GetUserByID(context.Context, uint64) (*CurrentUser, error)
}

type UserHandler struct {
	service CurrentUserService
}

func NewUserHandler(service CurrentUserService) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	value, exists := c.Get(jwtservice.UserIDContextKey)
	userID, ok := value.(uint64)
	if !exists || !ok || userID == 0 {
		response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
		return
	}

	currentUser, err := h.service.GetUserByID(c.Request.Context(), userID)
	switch {
	case err == nil:
		response.Success(c, ToUserInfoResponse(currentUser))
	case errors.Is(err, ErrUserNotExists):
		response.Error(c, http.StatusNotFound, 40401, "user not found")
	case errors.Is(err, ErrUserDisabled):
		response.Error(c, http.StatusForbidden, 40301, "user is disabled")
	default:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	}
}

func ToUserInfoResponse(currentUser *CurrentUser) *response.UserInfo {
	return &response.UserInfo{
		ID:          currentUser.ID,
		Username:    currentUser.Username,
		RoleCode:    currentUser.RoleCode,
		RoleName:    currentUser.RoleName,
		Roles:       currentUser.Roles,
		Permissions: currentUser.Permissions,
		IsActive:    currentUser.IsActive,
		CreatedAt:   currentUser.CreatedAt,
		UpdatedAt:   currentUser.UpdatedAt,
	}
}
