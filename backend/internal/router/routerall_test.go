package apirouter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DWHuang99/erent/internal/middleware/httpserver"
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	"github.com/DWHuang99/erent/internal/modules/user"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type fakeUserRepository struct {
	passwordHash string
}

func (f fakeUserRepository) GetUserAuthByUsername(context.Context, string) (*user.UserAuth, error) {
	return &user.UserAuth{
		ID: 1, Username: "admin", PasswordHash: f.passwordHash, RoleCode: "admin", IsActive: true,
	}, nil
}

func (f fakeUserRepository) GetUserAuthByID(context.Context, uint64) (*user.UserAuth, error) {
	return &user.UserAuth{
		ID: 1, Username: "admin", PasswordHash: f.passwordHash, RoleCode: "admin", IsActive: true,
	}, nil
}

func (f fakeUserRepository) GetUserByID(context.Context, uint64) (*user.CurrentUser, error) {
	return &user.CurrentUser{
		ID: 1, Username: "admin", RoleCode: "admin", RoleName: "admin", Roles: []string{"admin"}, Permissions: []string{}, IsActive: true,
	}, nil
}

func newTestRouter(repository *fakeUserRepository, manager *jwtservice.JWTManager, redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := gin.New()
	router.Use(httpserver.RequestID(), httpserver.Recovery(logger), httpserver.AccessLog(logger))
	health := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ai-gateway", "version": "test-version", "environment": "test"})
	}
	router.GET("/", health)
	router.GET("/api/health", health)
	router.GET("/health/live", health)
	router.GET("/health/ready", health)
	api := router.Group("/api/v1")
	if repository != nil && manager != nil && redisClient != nil {
		AuthRouter(api, *repository, manager, redisClient, false)
	}
	if repository != nil && manager != nil {
		UserRouter(api, *repository, manager)
	} else if manager != nil {
		UserRouter(api, fakeUserRepository{}, manager)
	}
	return router
}

func TestLoginAndCurrentUserFlow(t *testing.T) {
	hash, _ := security.HashPassword("admin12345")
	repository := fakeUserRepository{passwordHash: hash}
	manager := jwtservice.NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	router := newTestRouter(&repository, manager, redisClient)

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"admin12345"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	var loginPayload struct {
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginPayload); err != nil || loginPayload.Data.AccessToken == "" {
		t.Fatalf("login payload = %+v, err = %v", loginPayload, err)
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "refresh_token" || !cookies[0].HttpOnly {
		t.Fatalf("unexpected refresh cookies: %+v", cookies)
	}

	for _, path := range []string{"/api/v1/users/me", "/api/v1/auth/verify"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+loginPayload.Data.AccessToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"username":"admin"`) {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshRequest.AddCookie(cookies[0])
	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	rotatedCookies := refreshResponse.Result().Cookies()
	if len(rotatedCookies) != 1 || rotatedCookies[0].Value == cookies[0].Value {
		t.Fatalf("refresh token was not rotated: %+v", rotatedCookies)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.AddCookie(rotatedCookies[0])
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logoutResponse.Code, logoutResponse.Body.String())
	}
}

func TestHealthAndRequestID(t *testing.T) {
	router := newTestRouter(nil, nil, nil)
	for _, path := range []string{"/", "/api/health", "/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !httpserver.IsValidRequestID(response.Header().Get(httpserver.RequestIDHeader)) {
			t.Fatalf("%s status = %d, request id = %q", path, response.Code, response.Header().Get(httpserver.RequestIDHeader))
		}
	}
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	manager := jwtservice.NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	router := newTestRouter(nil, manager, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
