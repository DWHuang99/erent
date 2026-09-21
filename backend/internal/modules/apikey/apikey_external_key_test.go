package apikey

import (
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	jwtservice "erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
)

func externalKeyRepository(t *testing.T) *ApikeyRepository {
	t.Helper()
	repo := testRepository(t)
	db := repo.database
	for _, sql := range []string{
		"CREATE TABLE external_api_keys (id INTEGER PRIMARY KEY, user_id BIGINT NOT NULL, UNIQUE (user_id, id))",
		"INSERT INTO external_api_keys VALUES (100, 1), (101, 1), (200, 2)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../../migrations/000007_api_key_external_keys.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// SQLite needs the parent UNIQUE constraint declared at table creation above.
	for _, statement := range strings.Split(string(migration), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" || statement == "BEGIN" || statement == "COMMIT" || strings.HasPrefix(statement, "ALTER TABLE") {
			continue
		}
		if err := db.Exec(strings.ReplaceAll(statement, "TIMESTAMPTZ", "DATETIME")).Error; err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestExternalKeyAssociationConstraints(t *testing.T) {
	db := externalKeyRepository(t).database
	if err := db.Exec("INSERT INTO api_keys (id, user_id, key_hash, key_prefix) VALUES (1, 1, 'a', 'a'), (2, 1, 'b', 'b'), (3, 2, 'c', 'c')").Error; err != nil {
		t.Fatal(err)
	}
	for _, relation := range []ApikeyExternalKey{
		{UserID: 1, ApiKeyID: 1, ExternalApiKeyID: 100},
		{UserID: 1, ApiKeyID: 1, ExternalApiKeyID: 101},
		{UserID: 1, ApiKeyID: 2, ExternalApiKeyID: 100},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, relation := range []ApikeyExternalKey{
		{UserID: 1, ApiKeyID: 1, ExternalApiKeyID: 100}, // duplicate
		{UserID: 1, ApiKeyID: 1, ExternalApiKeyID: 200}, // external key owned by another user
		{UserID: 1, ApiKeyID: 3, ExternalApiKeyID: 100}, // system key owned by another user
		{UserID: 2, ApiKeyID: 1, ExternalApiKeyID: 100}, // forged owner
		{UserID: 1, ApiKeyID: 999, ExternalApiKeyID: 100},
		{UserID: 1, ApiKeyID: 1, ExternalApiKeyID: 999},
	} {
		if err := db.Create(&relation).Error; err == nil {
			t.Fatalf("invalid association accepted: %+v", relation)
		}
	}
	for _, deletion := range []struct {
		sql  string
		want int64
	}{
		{"DELETE FROM external_api_keys WHERE id = 101", 2},
		{"DELETE FROM api_keys WHERE id = 1", 1},
		{"DELETE FROM external_api_keys WHERE id = 100", 0},
	} {
		if err := db.Exec(deletion.sql).Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&ApikeyExternalKey{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != deletion.want {
			t.Fatalf("after %s: associations = %d, want %d", deletion.sql, count, deletion.want)
		}
	}
}

func TestCreateExternalKeyHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body              string
		status, oauth, external int
	}{
		{"external only", `{"external_api_key_ids":[100,100,101]}`, 201, 0, 2},
		{"mixed", `{"oauth_info":[10,10],"external_api_key_ids":[100,100]}`, 201, 1, 1},
		{"empty", `{}`, 400, 0, 0},
		{"foreign external", `{"oauth_info":[10],"external_api_key_ids":[200]}`, 400, 0, 0},
		{"foreign oauth", `{"oauth_info":[20],"external_api_key_ids":[100]}`, 400, 0, 0},
		{"missing", `{"external_api_key_ids":[999]}`, 400, 0, 0},
		{"zero", `{"external_api_key_ids":[0]}`, 400, 0, 0},
		{"overflow", `{"external_api_key_ids":[9223372036854775808]}`, 400, 0, 0},
		{"negative", `{"external_api_key_ids":[-1]}`, 400, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := externalKeyRepository(t)
			router := gin.New()
			router.POST("/", func(c *gin.Context) {
				c.Set(jwtservice.UserIDContextKey, uint64(1))
				NewHandler(NewService(repo)).CreatApikey(c)
			})
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body)
			}
			keys := 0
			if tc.status == 201 {
				keys = 1
			}
			for table, want := range map[string]int{"api_keys": keys, "api_key_accounts": tc.oauth, "api_key_external_keys": tc.external} {
				var count int64
				if err := repo.database.Table(table).Count(&count).Error; err != nil || count != int64(want) {
					t.Fatalf("%s: count %d want %d: %v", table, count, want, err)
				}
			}
			var links []ApikeyExternalKey
			if err := repo.database.Find(&links).Error; err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				var key ApikeyInfo
				if err := repo.database.First(&key, link.ApiKeyID).Error; err != nil || key.UserID != 1 || link.UserID != 1 || link.CreatedAt.IsZero() {
					t.Fatalf("incorrect persisted association: %+v, %v", link, err)
				}
			}
		})
	}
}

func TestExternalKeyWriteFailureRollsBack(t *testing.T) {
	repo := externalKeyRepository(t)
	// Fail the final association write after the key and OAuth association were inserted.
	if err := repo.database.Exec("CREATE TRIGGER fail_external BEFORE INSERT ON api_key_external_keys BEGIN SELECT RAISE(ABORT, 'write failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	raw, err := NewService(repo).CreatApikey(t.Context(), 1, []uint64{10}, []uint64{100})
	if err == nil || raw != "" {
		t.Fatalf("write failure returned %q, %v", raw, err)
	}
	for _, table := range []string{"api_keys", "api_key_accounts", "api_key_external_keys"} {
		var count int64
		if err := repo.database.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback %s: %d %v", table, count, err)
		}
	}
	for _, userID := range []uint64{0, 1 << 63} {
		if _, err := NewService(repo).CreatApikey(t.Context(), userID, nil, []uint64{100}); !errors.Is(err, ErrInvalidAccounts) {
			t.Fatalf("invalid user: %v", err)
		}
	}
}
