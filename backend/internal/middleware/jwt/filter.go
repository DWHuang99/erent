package jwtservice

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/DWHuang99/erent/internal/dto/response"
	"github.com/gin-gonic/gin"
)

const (
	UserIDContextKey      = "userID"
	UsernameContextKey    = "username"
	RoleContextKey        = "role"
	PermissionsContextKey = "permissions"
)

func JwtFilter(jwtManager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Error(c, http.StatusUnauthorized, 40100, "missing or invalid authorization header")
			c.Abort()
			return
		}

		claims, err := jwtManager.ParseToken(parts[1])
		if err != nil {
			response.Error(c, http.StatusUnauthorized, 40100, "invalid or expired token")
			c.Abort()
			return
		}
		userID, err := strconv.ParseUint(claims.Subject, 10, 64)
		if err != nil || userID == 0 {
			response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
			c.Abort()
			return
		}

		c.Set(UserIDContextKey, userID)
		c.Set(UsernameContextKey, claims.Username)
		c.Set(RoleContextKey, claims.Role)
		c.Set(PermissionsContextKey, claims.Permissions)
		c.Next()
	}
}
