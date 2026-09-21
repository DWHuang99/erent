package externalapikey

import (
	jwtservice "erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(api *gin.RouterGroup, handler *Handler, jwtManager *jwtservice.JWTManager) {
	keys := api.Group("/external-api-keys", jwtservice.JwtFilter(jwtManager))
	keys.POST("", handler.CreatExternalApiKey)
	keys.GET("", handler.List)
	keys.PUT("/:id", handler.UpdateExternalApiKey)
	keys.DELETE("/:id", handler.DeleteExternalApiKey)
}
