package user

import (
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterUserRoutes(router *gin.RouterGroup, handler *UserHandler, jwtManager *jwtservice.JWTManager) {
	users := router.Group("/users")
	users.Use(jwtservice.JwtFilter(jwtManager))
	users.GET("/me", handler.GetCurrentUser)

	// Compatibility alias for clients that only need to validate the current access token.
	auth := router.Group("/auth")
	auth.Use(jwtservice.JwtFilter(jwtManager))
	auth.GET("/verify", handler.GetCurrentUser)
}
