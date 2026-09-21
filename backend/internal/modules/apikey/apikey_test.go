package apikey

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/testdatabase"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func testRepository(t *testing.T) *ApikeyRepository {
	t.Helper()
	db := testdatabase.Open(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	for _, sql := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE users (id INTEGER PRIMARY KEY)",
		"INSERT INTO users VALUES (1), (2)",
		"CREATE TABLE oauth_infos (id INTEGER PRIMARY KEY, user_id BIGINT NOT NULL, UNIQUE (user_id, id))",
		"INSERT INTO oauth_infos VALUES (10, 1), (11, 1), (20, 2)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the migration's table constraints with SQLite-compatible identity syntax.
	migration, err := os.ReadFile("../../../migrations/000005_api_keys.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(migration), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" || statement == "BEGIN" || statement == "COMMIT" || strings.HasPrefix(statement, "ALTER TABLE") {
			continue
		}
		statement = strings.ReplaceAll(statement, "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY", "INTEGER PRIMARY KEY")
		statement = strings.ReplaceAll(statement, "TIMESTAMPTZ", "DATETIME")
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return NewRepository(db)
}

func TestCreateApikeyOwnershipAndHash(t *testing.T) {
	repo := testRepository(t)
	service := NewService(repo)
	for _, ids := range [][]uint64{nil, {0}, {999}, {10, 20}, {1 << 63}} {
		raw, err := service.CreatApikey(t.Context(), 1, ids, nil)
		if !errors.Is(err, ErrInvalidAccounts) || raw != "" {
			t.Fatalf("invalid accounts %v: %q %v", ids, raw, err)
		}
	}
	raw, err := service.CreatApikey(t.Context(), 1, []uint64{10, 11, 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var key ApikeyInfo
	if err := repo.database.First(&key).Error; err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(raw))
	if len(raw) != 46 || key.KeyHash != hex.EncodeToString(hash[:]) || key.KeyPrefix != raw[:12] {
		t.Fatal("incorrect generated key or stored hash")
	}
	var relations []Apikeyaccounts
	if err := repo.database.Find(&relations).Error; err != nil {
		t.Fatal(err)
	}
	if len(relations) != 2 {
		t.Fatalf("relations = %d", len(relations))
	}
	for _, rel := range relations {
		if rel.ApiKeyID != key.ID || rel.UserID != 1 {
			t.Fatal("incorrect association")
		}
	}
	second, err := service.CreatApikey(t.Context(), 1, []uint64{10}, nil)
	if err != nil || second == raw {
		t.Fatal("keys must differ and may share an account")
	}
}

func TestSaveApikeyRollsBackAndCascades(t *testing.T) {
	repo := testRepository(t)
	key := toApikeyinfo(strings.Repeat("a", 64), 1)
	// Deliberately bypass service validation: the database must enforce ownership.
	relations := toApikeyaccounts(1, []uint64{10, 20})
	if err := repo.SaveApikey(t.Context(), &key, &relations, nil); err == nil {
		t.Fatal("cross-owner association accepted")
	}
	for _, table := range []string{"api_keys", "api_key_accounts"} {
		var count int64
		if err := repo.database.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback %s: %d %v", table, count, err)
		}
	}
	if _, err := NewService(repo).CreatApikey(t.Context(), 1, []uint64{10, 11}, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.database.Exec("DELETE FROM oauth_infos WHERE id = 10").Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	repo.database.Table("api_key_accounts").Count(&count)
	if count != 1 {
		t.Fatal("account deletion did not remove association")
	}
	if err := repo.database.Exec("DELETE FROM api_keys").Error; err != nil {
		t.Fatal(err)
	}
	repo.database.Table("api_key_accounts").Count(&count)
	if count != 0 {
		t.Fatal("key deletion did not remove association")
	}
}

func TestCreateApikeyHTTP(t *testing.T) {
	repo := testRepository(t)
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret-with-at-least-32-characters", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	token, err := manager.GenerateToken(1, "alice", "user")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterApikeyRoutes(router.Group("/api/v1"), NewHandler(NewService(repo)), manager)
	for _, tc := range []struct {
		body, token string
		status      int
	}{
		{`{"oauth_info":[10]}`, "", 401},
		{`{"oauth_info":[10]}`, "invalid", 401},
		{`{`, token, 400},
		{`{"oauth_info":[]}`, token, 400},
		{`{"oauth_info":[20],"user_id":2}`, token, 400},
		{`{"oauth_info":[10],"user_id":2}`, token, 201},
	} {
		req := httptest.NewRequest("POST", "/api/v1/api-keys", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
		if tc.status == 201 {
			var result struct {
				Data struct {
					Key string `json:"api_key"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Data.Key == "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing secret response")
			}
		}
	}
	// A persistence failure must not produce a usable-looking success response.
	if err := repo.database.Callback().Create().Before("gorm:create").Register("fail", func(tx *gorm.DB) { tx.AddError(errors.New("write failed")) }); err != nil {
		t.Fatal(err)
	}
	raw, err := NewService(repo).CreatApikey(t.Context(), 1, []uint64{10}, nil)
	if err == nil || raw != "" {
		t.Fatal("returned secret after failed persistence")
	}
}
