package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/testdatabase"

	"github.com/gin-gonic/gin"
)

func TestOAuthListIsAuthenticatedIsolatedAndContainsOnlyMetadata(t *testing.T) {
	db := testdatabase.Open(t)
	if err := db.AutoMigrate(&OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	rows := []OAuthInfo{
		{UserID: 1, AccountID: "first", Email: "owner@example.com", Type: "codex", AccessToken: "access-secret", RefreshToken: "refresh-secret", IDToken: "id-secret"},
		{UserID: 2, AccountID: "other-owner", Type: "codex"},
		{UserID: 1, AccountID: "latest", Type: "codex"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	// The list remains available with no configured OIDC provider.
	service := NewOauthService(nil, nil, nil, NewRepository(db), nil)
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterOauthRoutes(router.Group("/oauth"), NewOauthHandler(service), manager)
	request := func(owner uint64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/oauth/list?user_id=2&provider=other", nil)
		if owner != 0 {
			token, err := manager.GenerateToken(owner, "owner", "user")
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := request(0); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	w := request(1)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"oauthlist"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || len(body.Data.Items) != 2 || body.Data.Items[0]["accountId"] != "latest" || body.Data.Items[1]["accountId"] != "first" {
		t.Fatalf("unexpected list: %s", w.Body)
	}
	for _, item := range body.Data.Items {
		if len(item) != 9 {
			t.Fatalf("unexpected public fields: %v", item)
		}
	}
	for _, forbidden := range []string{"secret", "Token", "token", "UserID", "userId", "other-owner"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("private data leaked: %s", forbidden)
		}
	}
	if w := request(3); w.Code != 200 || !strings.Contains(w.Body.String(), `"oauthlist":[]`) {
		t.Fatalf("empty list: %d %s", w.Code, w.Body)
	}
	if err := db.Migrator().DropTable(&OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	if w := request(1); w.Code != 500 || strings.Contains(w.Body.String(), "SELECT") {
		t.Fatalf("query failure: %d %s", w.Code, w.Body)
	}
}
