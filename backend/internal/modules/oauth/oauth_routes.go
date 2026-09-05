package oauth

import "github.com/gin-gonic/gin"

func RegisterOauthRoutes(api *gin.RouterGroup, handler *OauthHandler) {
	api.GET("/login", handler.Login)
	api.GET("/callback", handler.Callback)
}
