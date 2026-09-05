package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erent/internal/modules/oauth/oidc"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func testOAuthCallback(service *OauthService, target string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/callback", NewOauthHandler(service).Callback)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func TestCallbackRejectsMissingAndInvalidState(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)

	for _, target := range []string{"/callback", "/callback?state=invalid"} {
		response := testOAuthCallback(service, target)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, body = %s", target, response.Code, response.Body.String())
		}
	}
}

func TestCallbackReportsStateBackendFailure(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	redisServer.Close()

	response := testOAuthCallback(service, "/callback?state=state")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCallbackRejectsExpiredState(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	if err := service.StoreFlow("state", oidc.LoginFlow{
		Verifier:  "verifier",
		ExpiresAt: time.Now().Add(-time.Second),
	}, context.Background()); err != nil {
		t.Fatalf("store expired flow: %v", err)
	}

	response := testOAuthCallback(service, "/callback?state=state")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := service.PopFlow("state", context.Background()); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("expired state was not consumed: %v", err)
	}
}
