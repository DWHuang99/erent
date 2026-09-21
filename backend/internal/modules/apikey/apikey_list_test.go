package apikey

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"

	"github.com/gin-gonic/gin"
)

func TestApikeyListIsAuthenticatedIsolatedAndContainsOnlyMetadata(t *testing.T) {
	repo := externalKeyRepository(t)
	for _, statement := range []string{
		"ALTER TABLE oauth_infos ADD COLUMN email TEXT NOT NULL DEFAULT 'owner@example.com'",
		"ALTER TABLE oauth_infos ADD COLUMN type TEXT NOT NULL DEFAULT 'codex'",
		"ALTER TABLE oauth_infos ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE oauth_infos ADD COLUMN access_token TEXT DEFAULT 'access-secret'",
		"ALTER TABLE oauth_infos ADD COLUMN refresh_token TEXT DEFAULT 'refresh-secret'",
		"ALTER TABLE oauth_infos ADD COLUMN id_token TEXT DEFAULT 'id-secret'",
		"UPDATE oauth_infos SET disabled = TRUE WHERE id = 11",
	} {
		if err := repo.database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(repo)
	var secrets []string
	for _, seed := range []struct {
		owner uint64
		ids   []uint64
	}{
		{1, []uint64{10, 11}}, {2, []uint64{20}}, {1, []uint64{10}},
	} {
		raw, err := service.CreatApikey(t.Context(), seed.owner, seed.ids, nil)
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, raw)
	}
	if err := repo.database.Exec("UPDATE api_keys SET disabled = TRUE WHERE id = 3").Error; err != nil {
		t.Fatal(err)
	}
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterApikeyRoutes(router.Group("/api/v1"), NewHandler(service), manager)
	request := func(owner uint64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys?user_id=2", nil)
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
			Items []ApikeyListItem `json:"apikeylist"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	items := body.Data.Items
	if body.Code != 0 || len(items) != 2 || items[0].ID != 3 || items[1].ID != 1 || !items[0].Disabled {
		t.Fatalf("incorrect keys: %s", w.Body)
	}
	if len(items[0].Accounts) != 1 || items[0].Accounts[0].ID != 10 || len(items[1].Accounts) != 2 {
		t.Fatalf("incorrect bindings: %s", w.Body)
	}
	account := items[1].Accounts[1]
	if account.ID != 11 || account.Email != "owner@example.com" || account.Type != "codex" || !account.Disabled {
		t.Fatalf("incorrect account metadata: %+v", account)
	}
	for _, forbidden := range append(secrets, "secret", "key_hash", "token", "user_id", "UserID") {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("private data leaked: %s", forbidden)
		}
	}
	if w := request(3); w.Code != 200 || !strings.Contains(w.Body.String(), `"apikeylist":[]`) {
		t.Fatalf("empty list: %d %s", w.Code, w.Body)
	}
	if err := repo.database.Exec("DELETE FROM oauth_infos WHERE user_id = 1").Error; err != nil {
		t.Fatal(err)
	}
	if w := request(1); w.Code != 200 || strings.Count(w.Body.String(), `"accounts":[]`) != 2 {
		t.Fatalf("empty bindings: %d %s", w.Code, w.Body)
	}
	if err := repo.database.Exec("DROP TABLE oauth_infos").Error; err != nil {
		t.Fatal(err)
	}
	if w := request(1); w.Code != 500 || strings.Contains(w.Body.String(), "SELECT") {
		t.Fatalf("query failure: %d %s", w.Code, w.Body)
	}
}
