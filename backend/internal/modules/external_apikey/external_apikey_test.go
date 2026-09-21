package externalapikey

import (
	"encoding/json"
	"erent/internal/config"
	"erent/internal/dto/request"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/security"
	"erent/internal/testdatabase"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db := testdatabase.Open(t)
	if err := db.AutoMigrate(&ExternalApikey{}); err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepo(db), []byte("0123456789abcdef0123456789abcdef"))
}

func TestManagement(t *testing.T) {
	s := testService(t)
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), NewHandler(s), manager)
	call := func(method, path, body string, owner uint64, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/external-api-keys"+path, strings.NewReader(body))
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
		return w.Body.String()
	}
	body := `{"external_apikey":"secret-upstream-key","endpoint":"https://example.com/base/","suffix":{"responses":"custom/responses"}}`
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		path := ""
		if method == "PUT" || method == "DELETE" {
			path = "/1"
		}
		call(method, path, body, 0, 401)
	}
	created := call("POST", "", body, 1, 201)
	for _, forbidden := range []string{"secret-upstream-key", "ciphertext", "key_hash", "user_id"} {
		if strings.Contains(created, forbidden) {
			t.Fatalf("credential leaked: %s", created)
		}
	}
	rows, err := s.repository.QueryByApiKey(t.Context(), 1, "secret-upstream-key")
	if err != nil || len(rows) != 1 {
		t.Fatalf("lookup: %v %v", rows, err)
	}
	plain, err := security.Decrypt(s.encryptionKey, []byte(rows[0].Ciphertext))
	if err != nil || string(plain) != "secret-upstream-key" || rows[0].Ciphertext == string(plain) {
		t.Fatal("encrypted credential did not round-trip", err)
	}
	other, err := s.repository.QueryByApiKey(t.Context(), 2, "secret-upstream-key")
	if err != nil || len(other) != 0 {
		t.Fatal("cross-owner lookup", err)
	}
	if list := call("GET", "", "", 2, 200); !strings.Contains(list, `"apikeylist":[]`) {
		t.Fatal(list)
	}
	list := call("GET", "", "", 1, 200)
	if strings.Contains(list, "secret-upstream-key") || strings.Contains(list, rows[0].Ciphertext) {
		t.Fatal("list leaked key")
	}
	for _, method := range []string{"PUT", "DELETE"} {
		call(method, "/1", body, 2, 404)
		call(method, "/999", body, 1, 404)
		call(method, "/0", body, 1, 400)
	}
	for _, invalid := range []string{`{}`, `null`, `{"external_apikey":"x"}`, `{"external_apikey":"x","endpoint":"file:///tmp"}`, `{"external_apikey":"x","endpoint":"https://example.com","suffix":{"responses":"https://evil.com"}}`} {
		call("POST", "", invalid, 1, 400)
	}
	call("PUT", "/1", `{"external_apikey":"replacement-key","endpoint":"https://example.org"}`, 1, 200)
	rows, err = s.repository.QueryByApiKey(t.Context(), 1, "replacement-key")
	if err != nil || len(rows) != 1 || rows[0].Suffix.Responses != "" {
		t.Fatalf("replace/clear failed: %v %v", rows, err)
	}
	old, err := s.repository.QueryByApiKey(t.Context(), 1, "secret-upstream-key")
	if err != nil || len(old) != 0 {
		t.Fatal("stale hash", err)
	}
	call("DELETE", "/1", "", 1, 200)
	if err := s.Delete(t.Context(), 1, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}

func TestSuffixesAndValidation(t *testing.T) {
	s := testService(t)
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		key := ExternalApikey{Endpoint: "https://example.com/base/"}
		got, err := s.ResolveURL(key, path)
		if err != nil || got != "https://example.com/base"+path {
			t.Fatal(got, err)
		}
		key.Suffix = request.ExternalApikeySuffix{ChatCompletions: "custom/chat", Responses: "/custom/responses", Messages: "custom/messages"}
		got, err = s.ResolveURL(key, path)
		expected := map[string]string{"/v1/chat/completions": "chat", "/v1/responses": "responses", "/v1/messages": "messages"}[path]
		if err != nil || got != "https://example.com/base/custom/"+expected {
			t.Fatal(got, err)
		}
	}
	for _, suffix := range []string{"https://evil.com", "//evil.com/x", "../secret", "%2e%2e/secret", "x?key=y", "x#fragment", "x\\y", "/"} {
		req := request.ExternalApikeyRequest{ExternalApikey: "key", Endpoint: "https://example.com", Suffix: request.ExternalApikeySuffix{Responses: suffix}}
		if _, err := s.Create(t.Context(), 1, req); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("accepted suffix %q: %v", suffix, err)
		}
	}
	if _, err := s.ResolveURL(ExternalApikey{}, "/unknown"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	// Missing encryption configuration must never result in plaintext persistence.
	unconfigured := NewService(s.repository, nil)
	_, err := unconfigured.Create(t.Context(), 1, request.ExternalApikeyRequest{ExternalApikey: "key", Endpoint: "https://example.com"})
	if err == nil {
		t.Fatal("accepted missing encryption key")
	}
	rows, err := s.List(t.Context(), 1)
	if err != nil || len(rows) != 0 {
		t.Fatal(fmt.Sprint(rows), err)
	}
	if encoded, err := json.Marshal(rows); err != nil || string(encoded) != "[]" {
		t.Fatal(string(encoded), err)
	}
}

func TestLegacyChatPathsRejected(t *testing.T) {
	s := testService(t)
	for _, path := range []string{"/chat/completions", "/v1/response"} {
		key := ExternalApikey{Endpoint: "https://example.com"}
		if _, err := s.ResolveURL(key, path); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("legacy path %s: %v", path, err)
		}
	}
}
