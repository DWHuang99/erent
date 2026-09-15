package apikey

import (
	jwtservice "erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterApikeyRoutes(api *gin.RouterGroup, handler *ApikeyHandler, jwtManager *jwtservice.JWTManager) {
	keys := api.Group("/api-keys", jwtservice.JwtFilter(jwtManager))
	keys.POST("", handler.CreatApikey)
	keys.GET("", handler.ApikeyList)
	keys.PATCH("/:id", handler.UpdateApikey)
	keys.PUT("/:id/accounts", handler.ReplaceApikeyAccounts)
	keys.DELETE("/:id", handler.DeleteApikey)
}
