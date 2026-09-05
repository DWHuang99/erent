package main

import (
	"context"
	"net/http"
	"time"

	"erent/internal/config"
	"erent/internal/rpc/upstream"

	"github.com/gin-gonic/gin"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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
		if instances.upstreamConnection != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
			defer cancel()
			result, err := healthpb.NewHealthClient(instances.upstreamConnection).Check(ctx,
				&healthpb.HealthCheckRequest{Service: upstream.UpstreamService_ServiceDesc.ServiceName})
			if err != nil || result.GetStatus() != healthpb.HealthCheckResponse_SERVING {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
				return
			}
		}
		health(c)
	})
}
