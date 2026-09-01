package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/DWHuang99/erent/internal/config"
	dbconnect "github.com/DWHuang99/erent/internal/database/connect"
	"github.com/DWHuang99/erent/internal/dto/response"
	"github.com/DWHuang99/erent/internal/middleware/httpserver"
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	rdb "github.com/DWHuang99/erent/internal/middleware/redis"
	"github.com/DWHuang99/erent/internal/modules/user"
	apirouter "github.com/DWHuang99/erent/internal/router"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/gin-gonic/gin"
)

const serviceName = "ai-gateway"

var version = "dev"

func main() {
	configuration, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	connectContext, cancelConnect := context.WithTimeout(context.Background(), configuration.DatabaseConnectTimeout)
	database, sqlDatabase, err := dbconnect.Connect(connectContext, configuration.DatabaseURL)
	cancelConnect()
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer sqlDatabase.Close()
	redisConnectContext, cancelRedisConnect := context.WithTimeout(context.Background(), configuration.DatabaseConnectTimeout)
	redisClient, err := rdb.Connect(
		redisConnectContext,
		configuration.RedisAddress,
		configuration.RedisPassword,
		configuration.RedisDB,
	)
	cancelRedisConnect()
	if err != nil {
		logger.Error("connect redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	userRepository := user.NewRepository(database)
	if err := userRepository.Migrate(context.Background()); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}
	if configuration.BootstrapUsername != "" {
		passwordHash, err := security.HashPassword(configuration.BootstrapPassword)
		if err != nil {
			logger.Error("hash bootstrap password", "error", err)
			os.Exit(1)
		}
		if err := userRepository.CreateBootstrapUser(
			context.Background(),
			configuration.BootstrapUsername,
			passwordHash,
			configuration.BootstrapRole,
		); err != nil {
			logger.Error("create bootstrap user", "error", err)
			os.Exit(1)
		}
	}

	jwtManager := jwtservice.NewJWTManager(
		configuration.JWTSecret,
		configuration.JWTIssuer,
		configuration.JWTAudience,
		configuration.JWTAccessTTL,
		configuration.JWTRefreshTTL,
	)

	if configuration.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(httpserver.RequestID(), httpserver.Recovery(logger), httpserver.AccessLog(logger))

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
		if err := sqlDatabase.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		health(c)
	})
	router.GET("/ping", func(c *gin.Context) {
		response.Success(c, gin.H{"message": "pong"})
	})

	api := router.Group("/api/v1")
	apirouter.AuthRouter(api, userRepository, jwtManager, redisClient, configuration.CookieSecure)
	apirouter.UserRouter(api, userRepository, jwtManager)

	logger.Info("http server starting", "address", configuration.HTTPAddress, "environment", configuration.Environment, "version", version)
	if err := router.Run(configuration.HTTPAddress); err != nil {
		logger.Error("run gin server", "error", err)
		os.Exit(1)
	}
}
