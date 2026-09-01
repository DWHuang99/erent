// Package apirouter composes all HTTP modules into the monolithic Gin router.
package apirouter

import (
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	"github.com/DWHuang99/erent/internal/modules/auth"
	"github.com/DWHuang99/erent/internal/modules/user"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func AuthRouter(
	api *gin.RouterGroup,
	userRepository auth.UserAuthRepository,
	jwtManager *jwtservice.JWTManager,
	redisClient *redis.Client,
	cookieSecure bool,
) {
	auth.RegisterAuthRoutes(
		api,
		auth.NewAuthHandler(
			auth.NewService(userRepository, jwtManager, redisClient),
			cookieSecure,
		),
	)
}

func UserRouter(
	api *gin.RouterGroup,
	userRepository user.UserReader,
	jwtManager *jwtservice.JWTManager,
) {
	user.RegisterUserRoutes(
		api,
		user.NewUserHandler(user.NewService(userRepository)),
		jwtManager,
	)
}
