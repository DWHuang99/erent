package apikey

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func authRepository(t *testing.T) (*ApikeyRepository, string) {
	t.Helper()
	repo := testRepository(t)
	for _, sql := range []string{
		"ALTER TABLE oauth_infos ADD COLUMN email TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE oauth_infos ADD COLUMN type TEXT NOT NULL DEFAULT 'codex'",
		"ALTER TABLE oauth_infos ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE oauth_infos ADD COLUMN access_token TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE oauth_infos ADD COLUMN expired DATETIME",
	} {
		if err := repo.database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, err := NewService(repo).CreatApikey(t.Context(), 1, []uint64{10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return repo, raw
}

func TestCheckOauthBoundAccountsAndKeyState(t *testing.T) {
	for _, tc := range []struct {
		name, sql, model string
		want             error
	}{
		{"valid", "", "gpt-5.5", nil},
		{"unknown model", "", "unknown", ErrUnsupportedModel},
		{"disabled key", "UPDATE api_keys SET disabled = TRUE", "gpt-5.5", ErrInvalidApikey},
		{"expired key", "UPDATE api_keys SET expires_at = '2000-01-01 00:00:00'", "gpt-5.5", ErrInvalidApikey},
		{"disabled binding with other usable accounts", "UPDATE oauth_infos SET disabled = TRUE WHERE id = 10", "gpt-5.5", ErrOauthForbidden},
		{"wrong type with other matching accounts", "UPDATE oauth_infos SET type = 'other' WHERE id = 10", "gpt-5.5", ErrOauthForbidden},
		{"deleted binding", "DELETE FROM api_key_accounts", "gpt-5.5", ErrOauthForbidden},
		{"wrong owner", "UPDATE oauth_infos SET user_id = 2 WHERE id = 10", "gpt-5.5", ErrOauthForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, raw := authRepository(t)
			if tc.name == "wrong owner" {
				if err := repo.database.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.sql != "" {
				if err := repo.database.Exec(tc.sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			checker := NewCheck()
			checker.inject(repo)
			accounts, err := checker.checkOauth(t.Context(), raw, tc.model)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want == nil && (len(accounts) != 1 || accounts[0].ID != 10) {
				t.Fatalf("unexpected accounts: %+v", accounts)
			}
		})
	}
}

func TestCheckOauthErrorsAndModels(t *testing.T) {
	checker := NewCheck()
	if _, err := checker.checkOauth(t.Context(), "key", "gpt-5.5"); !errors.Is(err, ErrCheckUnavailable) {
		t.Fatal(err)
	}
	repo, raw := authRepository(t)
	checker.inject(repo)
	if _, err := checker.checkOauth(t.Context(), "invalid", "gpt-5.5"); !errors.Is(err, ErrInvalidApikey) {
		t.Fatal(err)
	}
	for _, model := range []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.3-codex-spark"} {
		if _, err := checker.checkOauth(t.Context(), raw, model); err != nil {
			t.Fatalf("%s: %v", model, err)
		}
	}
	future := time.Now().Add(time.Hour)
	if err := repo.database.Model(&ApikeyInfo{}).Where("id = 1").Update("expires_at", future).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := checker.checkOauth(t.Context(), raw, "gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	if err := repo.database.Exec("DROP TABLE api_keys").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := checker.checkOauth(t.Context(), raw, "gpt-5.5"); err == nil || errors.Is(err, ErrInvalidApikey) {
		t.Fatalf("database error lost: %v", err)
	}
}

func TestApikeyFilterRejectsAndPreservesBody(t *testing.T) {
	repo, raw := authRepository(t)
	previous := defaultcheck.repository
	ApikeycheckInject(repo)
	t.Cleanup(func() { ApikeycheckInject(previous) })
	router := gin.New()
	called := false
	body := `{"model":"gpt-5.5","messages":[]}`
	router.POST("/chat", ApikeyFilter(), func(c *gin.Context) {
		called = true
		got, err := io.ReadAll(c.Request.Body)
		if err != nil || string(got) != body || c.GetString("apikey") != raw {
			t.Error("request changed")
		}
		value, ok := c.Get("apikey_accounts")
		if !ok || len(value.([]ApikeyAccountItem)) != 1 {
			t.Error("missing authorized accounts")
		}
		c.Status(http.StatusNoContent)
	})
	for _, tc := range []struct {
		name, token, body string
		status            int
	}{
		{"missing header", "", body, 401},
		{"invalid key", "Bearer invalid", body, 401},
		{"bad json", "Bearer " + raw, "{", 400},
		{"missing model", "Bearer " + raw, "{}", 400},
		{"unknown model", "Bearer " + raw, `{"model":"unknown"}`, 400},
		{"valid", "Bearer " + raw, body, 204},
	} {
		called = false
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/chat", strings.NewReader(tc.body))
		req.Header.Set("Authorization", tc.token)
		router.ServeHTTP(w, req)
		if w.Code != tc.status || called != (tc.status == 204) {
			t.Fatalf("%s: %d %s, called=%v", tc.name, w.Code, w.Body.String(), called)
		}
	}
	for _, tc := range []struct {
		sql    string
		status int
	}{
		{"UPDATE oauth_infos SET disabled = TRUE WHERE id = 10", 403},
		{"DROP TABLE api_keys", 500},
	} {
		if err := repo.database.Exec(tc.sql).Error; err != nil {
			t.Fatal(err)
		}
		called = false
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/chat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+raw)
		router.ServeHTTP(w, req)
		if w.Code != tc.status || called {
			t.Fatalf("status=%d, called=%v", w.Code, called)
		}
	}
}
