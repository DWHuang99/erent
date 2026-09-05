package main

import (
	"net/http"

	"erent/internal/config"

	"github.com/gin-gonic/gin"
)

func registerHealthRoutes(router *gin.Engine, configuration config.Config, instances *applicationInstances) {
	health := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"environment": configuration.Environment,
			"service":     serviceName,
			"status":      "ok",
			"version":     version,
		})
	}

	router.GET("/", health)
	router.GET("/api/health", health)
	router.GET("/health/live", health)
	router.GET("/health/ready", func(c *gin.Context) {
		if err := instances.sqlDatabase.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		if err := instances.redisClient.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		health(c)
	})
}
