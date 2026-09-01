package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"erent/internal/config"
	dbconnect "erent/internal/database/connect"
	"erent/internal/dto/response"
	applogger "erent/internal/logger"
	casbinrbac "erent/internal/middleware/casbin"
	"erent/internal/middleware/httpserver"
	jwtservice "erent/internal/middleware/jwt"
	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/user"
	apirouter "erent/internal/router"
	"erent/internal/security"

	"github.com/gin-gonic/gin"
)

const serviceName = "ai-gateway"

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	logger, logFile, err := applogger.NewLogger(os.Getenv("LOG_FILE"))
	if err != nil {
		slog.Error("initialize logger", "error", err)
		return 1
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close log file: %v\n", err)
		}
	}()
	slog.SetDefault(logger)

	configuration, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		return 1
	}
	connectContext, cancelConnect := context.WithTimeout(context.Background(), configuration.DatabaseConnectTimeout)
	database, sqlDatabase, err := dbconnect.Connect(connectContext, configuration.DatabaseURL)
	cancelConnect()
	if err != nil {
		slog.Error("connect database", "error", err)
		return 1
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
		slog.Error("connect redis", "error", err)
		return 1
	}
	defer redisClient.Close()

	userRepository := user.NewRepository(database)
	if configuration.BootstrapUsername != "" {
		if !casbinrbac.IsSupportedRole(configuration.BootstrapRole) {
			slog.Error("invalid bootstrap role", "role", configuration.BootstrapRole)
			return 1
		}
		passwordHash, err := security.HashPassword(configuration.BootstrapPassword)
		if err != nil {
			slog.Error("hash bootstrap password", "error", err)
			return 1
		}
		if err := userRepository.CreateBootstrapUser(
			context.Background(),
			configuration.BootstrapUsername,
			passwordHash,
			configuration.BootstrapRole,
		); err != nil {
			slog.Error("create bootstrap user", "error", err)
			return 1
		}
	}
	casbinEnforcer, err := casbinrbac.NewEnforcer(database)
	if err != nil {
		slog.Error("initialize Casbin authorization", "error", err)
		return 1
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
	apirouter.AuthRouter(api, userRepository, jwtManager, redisClient, casbinEnforcer, configuration.CookieSecure)
	apirouter.UserRouter(api, userRepository, jwtManager, casbinEnforcer)

	slog.Info("http server starting", "address", configuration.HTTPAddress, "environment", configuration.Environment, "version", version)
	if err := router.Run(configuration.HTTPAddress); err != nil {
		slog.Error("run gin server", "error", err)
		return 1
	}
	return 0
}
