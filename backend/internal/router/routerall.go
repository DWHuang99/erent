// Package apirouter composes all HTTP modules into the monolithic Gin router.
package apirouter

import (
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/modules/auth"
	"erent/internal/modules/oauth"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/modules/user"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func AuthRouter(
	api *gin.RouterGroup,
	userRepository *user.Repository,
	jwtManager *jwtservice.JWTManager,
	redisClient *redis.Client,
	enforcer *casbin.SyncedEnforcer,
	cookieSecure bool,
) {
	auth.RegisterAuthRoutes(
		api,
		auth.NewAuthHandler(
			auth.NewService(userRepository, jwtManager, redisClient, enforcer),
			cookieSecure,
		),
	)
}

func UserRouter(
	api *gin.RouterGroup,
	userRepository *user.Repository,
	jwtManager *jwtservice.JWTManager,
	enforcer *casbin.SyncedEnforcer,
) {
	user.RegisterUserRoutes(
		api,
		user.NewUserHandler(user.NewService(userRepository, enforcer)),
		jwtManager,
	)
}

func OauthRouter(
	api *gin.RouterGroup,
	redisClient *redis.Client,
	oidcAuth *oidc.OIDCAuth,
) {
	oauth.RegisterOauthRoutes(
		api,
		oauth.NewOauthHandler(oauth.NewOauthService(redisClient, oidcAuth)),
	)
}
