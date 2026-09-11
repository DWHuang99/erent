package oauth

import (
	"encoding/json"
	"fmt"
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

func TestOAuthDeleteAuthenticationOwnershipAndErrors(t *testing.T) {
	db := testdatabase.Open(t)
	if err := db.AutoMigrate(&OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	rows := []OAuthInfo{{UserID: 1, AccountID: "owner"}, {UserID: 2, AccountID: "other"}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	service := NewOauthService(nil, nil, nil, NewRepository(db), nil)
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterOauthRoutes(router.Group("/oauth"), NewOauthHandler(service), manager)
	check := func(owner uint64, payload string, status, code int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/oauth/delete", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if owner != 0 {
			token, err := manager.GenerateToken(owner, "owner", "user")
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var body struct {
			Code int `json:"code"`
		}
		// Unmarshal also rejects an error response followed by a second success JSON.
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid response: %s: %v", w.Body, err)
		}
		if w.Code != status || body.Code != code {
			t.Fatalf("delete %s: status=%d body=%s", payload, w.Code, w.Body)
		}
	}
	owned := fmt.Sprintf(`{"id":%d}`, rows[0].ID)
	check(0, owned, 401, 40100)
	for _, payload := range []string{`{}`, `{"id":0}`, `{"id":-1}`, `{"id":"bad"}`, `{`} {
		check(1, payload, 400, 400)
	}
	check(1, fmt.Sprintf(`{"id":%d,"user_id":2}`, rows[1].ID), 404, 404)
	check(1, `{"id":999999}`, 404, 404)
	var count int64
	if err := db.Model(&OAuthInfo{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("rejected requests changed records: count=%d err=%v", count, err)
	}
	check(1, owned, 200, 0)
	check(1, owned, 404, 404)
	var remaining []OAuthInfo
	if err := db.Find(&remaining).Error; err != nil || len(remaining) != 1 || remaining[0].ID != rows[1].ID {
		t.Fatalf("wrong records after delete: %v err=%v", remaining, err)
	}
	if err := db.Migrator().DropTable(&OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	check(1, owned, 500, 500)
}
