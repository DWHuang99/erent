package apikey

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestApikeyManagement(t *testing.T) {
	repo := testRepository(t)
	service := NewService(repo)
	if _, err := service.CreatApikey(t.Context(), 1, []uint64{10}); err != nil {
		t.Fatal(err)
	}
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterApikeyRoutes(router.Group("/api/v1"), NewHandler(service), manager)
	call := func(method, path, body string, owner uint64, status int) {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/api-keys"+path, strings.NewReader(body))
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
		if w.Code != status {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, status, w.Body)
		}
	}
	for _, tc := range []struct{ method, path, body string }{
		{"PATCH", "/1", `{"disabled":true}`},
		{"PUT", "/1/accounts", `{"oauth_info":[20]}`},
		{"DELETE", "/1", ``},
	} {
		call(tc.method, tc.path, tc.body, 0, 401)
		call(tc.method, tc.path, tc.body, 2, 404)
		call(tc.method, strings.Replace(tc.path, "/1", "/999", 1), tc.body, 1, 404)
		call(tc.method, strings.Replace(tc.path, "/1", "/0", 1), tc.body, 1, 400)
	}
	for _, body := range []string{`{}`, `null`, `{"disabled":null}`, `{"disabled":"false"}`, `{"expires_at":"bad"}`, `{"expires_at":123}`} {
		call("PATCH", "/1", body, 1, 400)
	}
	readKey := func() ApikeyInfo {
		t.Helper()
		var key ApikeyInfo
		if err := repo.database.First(&key, 1).Error; err != nil {
			t.Fatal(err)
		}
		return key
	}
	call("PATCH", "/1", `{"disabled":true,"expires_at":"2030-01-01T00:00:00Z"}`, 1, 200)
	key := readKey()
	if !key.Disabled || key.ExpiresAt == nil || key.ExpiresAt.Year() != 2030 {
		t.Fatal("status/expiry not saved")
	}
	call("PATCH", "/1", `{"disabled":false}`, 1, 200)
	key = readKey()
	if key.Disabled || key.ExpiresAt == nil {
		t.Fatal("false not saved or omitted expiry changed")
	}
	call("PATCH", "/1", `{"expires_at":null}`, 1, 200)
	if readKey().ExpiresAt != nil {
		t.Fatal("expiry not cleared")
	}
	for _, body := range []string{`{"oauth_info":[]}`, `{"oauth_info":[20]}`, `{"oauth_info":[999]}`, `{"oauth_info":[0]}`} {
		call("PUT", "/1/accounts", body, 1, 400)
	}
	assertAccounts := func(want uint64) {
		t.Helper()
		var accounts []Apikeyaccounts
		if err := repo.database.Where("api_key_id = ?", 1).Find(&accounts).Error; err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 1 || accounts[0].OAuthInfoID != want {
			t.Fatalf("unexpected associations: %+v", accounts)
		}
	}
	assertAccounts(10)
	call("PUT", "/1/accounts", `{"oauth_info":[11,11]}`, 1, 200)
	assertAccounts(11)
	// Fail insertion after old bindings were deleted: the transaction must restore them.
	if err := repo.database.Callback().Create().Before("gorm:create").Register("fail-bindings", func(tx *gorm.DB) {
		if tx.Statement.Table == "api_key_accounts" {
			tx.AddError(errors.New("forced write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	call("PUT", "/1/accounts", `{"oauth_info":[10]}`, 1, 500)
	assertAccounts(11)
	if err := repo.database.Callback().Create().Remove("fail-bindings"); err != nil {
		t.Fatal(err)
	}
	call("DELETE", "/1", "", 1, 200)
	var count int64
	if err := repo.database.Table("api_key_accounts").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cascade: %d %v", count, err)
	}
	call("DELETE", "/1", "", 1, 404)
}
