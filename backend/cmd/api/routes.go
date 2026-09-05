package main

import (
	"log/slog"

	"erent/internal/dto/response"
	"erent/internal/middleware/httpserver"
	apirouter "erent/internal/router"

	"github.com/gin-gonic/gin"
)

func newRouter(configuration apiConfiguration, instances *applicationInstances, logger *slog.Logger) *gin.Engine {
	if configuration.runtime.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(httpserver.RequestID(), httpserver.Recovery(logger), httpserver.AccessLog(logger))

	registerHealthRoutes(router, configuration.runtime, instances)
	router.GET("/ping", func(c *gin.Context) {
		response.Success(c, gin.H{"message": "pong"})
	})

	api := router.Group("/api/v1")
	apirouter.AuthRouter(
		api,
		instances.userRepository,
		instances.jwtManager,
		instances.redisClient,
		instances.casbinEnforcer,
		configuration.runtime.CookieSecure,
	)
	apirouter.UserRouter(api, instances.userRepository, instances.jwtManager, instances.casbinEnforcer)
	if instances.oaiOIDCAuth != nil {
		apirouter.OauthRouter(router.Group("/oai"), instances.redisClient, instances.oaiOIDCAuth)
	}

	return router
}
