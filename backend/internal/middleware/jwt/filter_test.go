package jwtservice

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestJwtFilterStoresVerifiedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := newTestJWTManager("test-secret-with-at-least-32-characters", time.Minute)
	token, _ := manager.GenerateToken(7, "alice", "admin")
	router := gin.New()
	router.GET("/protected", JwtFilter(manager), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"userID": c.MustGet(UserIDContextKey)})
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestJwtFilterRejectsMissingToken(t *testing.T) {
	manager := newTestJWTManager("test-secret-with-at-least-32-characters", time.Minute)
	router := gin.New()
	router.GET("/protected", JwtFilter(manager), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
