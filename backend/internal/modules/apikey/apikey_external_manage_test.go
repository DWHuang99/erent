package apikey

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"github.com/gin-gonic/gin"
)

func TestExternalKeyManagementAndList(t *testing.T) {
	repo := externalKeyRepository(t)
	for _, sql := range []string{
		"ALTER TABLE oauth_infos ADD COLUMN email TEXT DEFAULT ''",
		"ALTER TABLE oauth_infos ADD COLUMN type TEXT DEFAULT 'codex'",
		"ALTER TABLE oauth_infos ADD COLUMN disabled BOOLEAN DEFAULT FALSE",
	} {
		if err := repo.database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(repo)
	if _, err := s.CreatApikey(t.Context(), 1, []uint64{10}, []uint64{100}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatApikey(t.Context(), 2, nil, []uint64{200}); err != nil {
		t.Fatal(err)
	}
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterApikeyRoutes(router.Group("/api/v1"), NewHandler(s), manager)
	call := func(method, path, body string, owner uint64, status int) *httptest.ResponseRecorder {
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
		return w
	}
	assertList := func(owner uint64, want []uint64) {
		t.Helper()
		w := call("GET", "", "", owner, 200)
		var body struct {
			Data struct {
				Items []ApikeyListItem `json:"apikeylist"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Data.Items) != 1 || !reflect.DeepEqual(body.Data.Items[0].ExternalApiKeyIDs, want) {
			t.Fatalf("unexpected external bindings: %s", w.Body)
		}
		if owner == 1 && (len(body.Data.Items[0].Accounts) != 1 || body.Data.Items[0].Accounts[0].ID != 10) {
			t.Fatalf("OAuth bindings changed: %s", w.Body)
		}
		for _, private := range []string{"ciphertext", "key_hash", "user_id"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatalf("private field leaked: %s", w.Body)
			}
		}
	}
	assertList(1, []uint64{100})
	assertList(2, []uint64{200})
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[101]}`, 0, 401)
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[200]}`, 2, 404)
	call("PUT", "/999/accounts", `{"oauth_info":[10],"external_api_key_ids":[]}`, 1, 404)
	call("PUT", "/0/accounts", `{"oauth_info":[10],"external_api_key_ids":[]}`, 1, 400)
	for _, body := range []string{`{}`, `null`, `{"oauth_info":[10],"external_api_key_ids":null}`, `{"oauth_info":[10],"external_api_key_ids":[0]}`, `{"oauth_info":[10],"external_api_key_ids":[200]}`, `{"oauth_info":[10],"external_api_key_ids":[999]}`, `{"oauth_info":[10],"external_api_key_ids":[9223372036854775808]}`, `{"oauth_info":[10],"external_api_key_ids":[-1]}`} {
		call("PUT", "/1/accounts", body, 1, 400)
		assertList(1, []uint64{100})
	}
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[101,100,101]}`, 1, 200)
	assertList(1, []uint64{100, 101})
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[101]}`, 1, 200)
	assertList(1, []uint64{101})
	if err := repo.database.Exec("CREATE TRIGGER fail_external BEFORE INSERT ON api_key_external_keys BEGIN SELECT RAISE(ABORT, 'write failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[100]}`, 1, 500)
	assertList(1, []uint64{101})
	if err := repo.database.Exec("DROP TRIGGER fail_external").Error; err != nil {
		t.Fatal(err)
	}
	call("PUT", "/1/accounts", `{"oauth_info":[10],"external_api_key_ids":[]}`, 1, 200)
	assertList(1, []uint64{})
	call("PUT", "/2/accounts", `{"oauth_info":[20],"external_api_key_ids":[]}`, 2, 200)
	assertList(2, []uint64{})
	var count int64
	if err := repo.database.Table("external_api_keys").Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("unbinding deleted external keys: %d %v", count, err)
	}
}

func TestCombinedBindingsAtomicRollback(t *testing.T) {
	repo := externalKeyRepository(t)
	s := NewService(repo)
	if _, err := s.CreatApikey(t.Context(), 1, []uint64{10}, []uint64{100}); err != nil {
		t.Fatal(err)
	}
	assertBindings := func(oauth, external uint64) {
		t.Helper()
		var accounts []Apikeyaccounts
		var keys []ApikeyExternalKey
		if err := repo.database.Find(&accounts).Error; err != nil {
			t.Fatal(err)
		}
		if err := repo.database.Find(&keys).Error; err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 1 || accounts[0].OAuthInfoID != oauth || len(keys) != 1 || keys[0].ExternalApiKeyID != external {
			t.Fatalf("bindings: %+v %+v", accounts, keys)
		}
	}
	if err := s.ReplaceApikeyAccounts(t.Context(), 1, 1, []uint64{11}, []uint64{200}); err == nil {
		t.Fatal("foreign external key accepted")
	}
	assertBindings(10, 100)
	if err := repo.database.Exec("CREATE TRIGGER fail_external BEFORE INSERT ON api_key_external_keys BEGIN SELECT RAISE(ABORT, 'write failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceApikeyAccounts(t.Context(), 1, 1, []uint64{11}, []uint64{101}); err == nil {
		t.Fatal("write failure ignored")
	}
	assertBindings(10, 100)
	if err := repo.database.Exec("DROP TRIGGER fail_external").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceApikeyAccounts(t.Context(), 1, 1, []uint64{11, 11}, []uint64{101, 101}); err != nil {
		t.Fatal(err)
	}
	assertBindings(11, 101)
	if err := s.ReplaceApikeyAccounts(t.Context(), 1, 1, nil, []uint64{100}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := repo.database.Model(&Apikeyaccounts{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("OAuth not cleared: %d %v", count, err)
	}
	if err := s.ReplaceApikeyAccounts(t.Context(), 1, 1, nil, nil); err == nil {
		t.Fatal("empty scope accepted")
	}
}
