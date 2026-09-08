package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	jwtservice "erent/internal/middleware/jwt"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestLoginJSONReturnsAuthorizationURL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	router := gin.New()
	router.GET("/login", func(c *gin.Context) {
		c.Set(jwtservice.UserIDContextKey, uint64(7))
		NewOauthHandler(service).Login(c)
	})
	request := httptest.NewRequest(http.MethodGet, "/login?provider=oai", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Message != "success" {
		t.Fatalf("unexpected response envelope: %s", response.Body.String())
	}
	location, err := url.Parse(body.Data.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if query.Get("code_challenge") == "" || query.Get("nonce") == "" {
		t.Fatal("missing PKCE or nonce")
	}
	flow, err := service.PopFlow(query.Get("state"), t.Context())
	if err != nil || flow.UserID != 7 {
		t.Fatalf("flow not bound to user: %+v %v", flow, err)
	}
}
