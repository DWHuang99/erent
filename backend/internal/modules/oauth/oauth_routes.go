package oauth

import (
	jwtservice "erent/internal/middleware/jwt"

	"github.com/gin-gonic/gin"
)

func RegisterOauthRoutes(api *gin.RouterGroup, handler *OauthHandler, jwtManager *jwtservice.JWTManager) {
	api.GET("/login", jwtservice.JwtFilter(jwtManager), handler.Login)
	api.GET("/callback", handler.Callback)
	api.GET("/list", jwtservice.JwtFilter(jwtManager), handler.OauthList)
	api.POST("/refresh", jwtservice.JwtFilter(jwtManager), handler.RefreshToken)
	api.POST("/delete", jwtservice.JwtFilter(jwtManager), handler.Delete)
}
