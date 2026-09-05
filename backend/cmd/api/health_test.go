package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"erent/internal/config"
	"erent/internal/testdatabase"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRegisterHealthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := testdatabase.Open(t)
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("get database handle: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	router := gin.New()
	registerHealthRoutes(router, config.Config{Environment: "test"}, &applicationInstances{
		sqlDatabase: sqlDatabase,
		redisClient: redisClient,
	})

	for _, path := range []string{"/", "/api/health", "/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d, want %d", path, response.Code, http.StatusOK)
		}
		var body map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode GET %s response: %v", path, err)
		}
		if body["status"] != "ok" || body["environment"] != "test" {
			t.Fatalf("unexpected GET %s response: %v", path, body)
		}
	}

	redisServer.Close()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness returned %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
