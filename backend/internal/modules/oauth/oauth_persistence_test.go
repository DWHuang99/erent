package oauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/modules/user"
	"erent/internal/security"
	"erent/internal/testdatabase"

	"github.com/alicebob/miniredis/v2"
	oidcgo "github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type tokenExchange struct{ token *oauth2.Token }

func (e *tokenExchange) Exchange(context.Context, string, string, string) (*oauth2.Token, error) {
	return e.token, nil
}

func persistenceService(t *testing.T) (*OauthService, *gorm.DB, *rsa.PrivateKey) {
	t.Helper()
	db := testdatabase.Open(t)
	if err := db.AutoMigrate(&user.User{}, &OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"owner", "other"} {
		if err := db.Create(&user.User{Username: name, PasswordHash: "unused", RoleCode: "user", IsActive: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	service := newTestOAuthService(nil)
	service.repository = NewRepository(db)
	service.oidcAuth["oai"].Verifier = oidcgo.NewVerifier("https://issuer.example", &oidcgo.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidcgo.Config{ClientID: "client-id"})
	service.directory = testDirectory(service, nil)
	return service, db, key
}

func signedToken(t *testing.T, key *rsa.PrivateKey, changes jwt.MapClaims) *oauth2.Token {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": "https://issuer.example", "aud": "client-id", "sub": "upstream-user",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": "nonce",
		"email": "owner@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "account"},
	}
	for k, v := range changes {
		claims[k] = v
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return (&oauth2.Token{AccessToken: "access-secret", RefreshToken: "refresh-secret", Expiry: time.Now().Add(time.Hour).UTC().Truncate(time.Second)}).WithExtra(map[string]any{"id_token": raw})
}

func TestSaveTokenPersistsAndIsolatesOwners(t *testing.T) {
	service, db, key := persistenceService(t)
	flow := oidc.LoginFlow{Provider: "oai", UserID: 1, Nonce: "nonce"}
	token := signedToken(t, key, nil)
	if err := service.SaveToken(t.Context(), token, flow, true); err != nil {
		t.Fatal(err)
	}
	var saved OAuthInfo
	if err := db.First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.UserID != 1 || saved.AccountID != "account" || saved.Email != "owner@example.com" || saved.Type != "codex" || saved.Disabled || saved.Expired == nil || !saved.Expired.Equal(token.Expiry) || saved.LastRefresh == nil {
		t.Fatal("stored credential fields do not match the verified token and owner")
	}
	for ciphertext, want := range map[string]string{saved.AccessToken: token.AccessToken, saved.RefreshToken: token.RefreshToken, saved.IDToken: token.Extra("id_token").(string)} {
		plain, err := security.Decrypt(service.encryptionKey, []byte(ciphertext))
		if err != nil || ciphertext == want || string(plain) != want {
			t.Fatal("stored token was not encrypted correctly")
		}
	}
	encoded, err := json.Marshal(saved)
	if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), saved.IDToken) {
		t.Fatal("credentials leaked through JSON")
	}
	token.AccessToken = "replacement-secret"
	token.RefreshToken = ""
	token.Expiry = time.Time{}
	if err := service.SaveToken(t.Context(), token, flow, true); !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate insert got %v", err)
	}
	var updated OAuthInfo
	if err := db.First(&updated, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.AccessToken != saved.AccessToken || updated.RefreshToken != saved.RefreshToken || updated.IDToken != saved.IDToken || updated.Expired == nil || !updated.Expired.Equal(*saved.Expired) || !updated.UpdatedAt.Equal(saved.UpdatedAt) {
		t.Fatal("duplicate insert overwrote the existing credentials")
	}
	flow.UserID = 2
	if err := service.SaveToken(t.Context(), token, flow, true); err != nil {
		t.Fatal(err)
	}
	var rows []OAuthInfo
	if err := db.Order("user_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserID != 1 || rows[1].UserID != 2 || rows[1].Disabled {
		t.Fatal("credentials were duplicated or reassigned across owners")
	}
	plain, err := security.Decrypt(service.encryptionKey, []byte(rows[1].RefreshToken))
	if err != nil || string(plain) != "" || rows[1].Expired != nil {
		t.Fatal("empty refresh token or unknown expiry was not preserved on insert")
	}
}

func TestSaveTokenRejectsInvalidEncryptionKey(t *testing.T) {
	service, db, key := persistenceService(t)
	service.encryptionKey = nil
	if err := service.SaveToken(t.Context(), signedToken(t, key, nil), oidc.LoginFlow{Provider: "oai", UserID: 1, Nonce: "nonce"}, true); err == nil {
		t.Fatal("invalid encryption key accepted")
	}
	var count int64
	if err := db.Model(&OAuthInfo{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("credentials saved after encryption failed")
	}
}

func TestSaveTokenRejectsInvalidIdentityAndState(t *testing.T) {
	service, db, key := persistenceService(t)
	for _, tc := range []struct {
		name    string
		changes jwt.MapClaims
	}{
		{"issuer", jwt.MapClaims{"iss": "https://wrong.example"}},
		{"audience", jwt.MapClaims{"aud": "wrong-client"}},
		{"expired", jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}},
		{"nonce", jwt.MapClaims{"nonce": "wrong-nonce"}},
		{"missing nonce", jwt.MapClaims{"nonce": ""}},
		{"account", jwt.MapClaims{"https://api.openai.com/auth": map[string]any{}}},
		{"malformed claims", jwt.MapClaims{"email": 123}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := service.SaveToken(t.Context(), signedToken(t, key, tc.changes), oidc.LoginFlow{Provider: "oai", UserID: 1, Nonce: "nonce"}, true); !errors.Is(err, ErrInvalidIDToken) {
				t.Fatalf("got %v", err)
			}
		})
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	flow := oidc.LoginFlow{Provider: "oai", UserID: 1, Nonce: "nonce"}
	for _, token := range []*oauth2.Token{nil, {AccessToken: "secret"}, signedToken(t, otherKey, nil)} {
		if err := service.SaveToken(t.Context(), token, flow, true); !errors.Is(err, ErrInvalidIDToken) {
			t.Fatalf("invalid token got %v", err)
		}
	}
	if err := service.SaveToken(t.Context(), signedToken(t, key, nil), oidc.LoginFlow{Provider: "oai", Nonce: "nonce"}, true); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("missing state user ID got %v", err)
	}
	var count int64
	if err := db.Model(&OAuthInfo{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid credentials were saved")
	}
}

func TestLoginCallbackBindsAuthenticatedUserAndConsumesState(t *testing.T) {
	service, db, key := persistenceService(t)
	redisServer := miniredis.RunT(t)
	service.redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = service.redisClient.Close() })
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "local-test-secret", Issuer: "local", Audience: "local-api", AccessTTL: time.Hour})
	localToken, err := manager.GenerateToken(1, "owner", "user")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	// A valid local JWT is sufficient; OAuth does not recheck the user's status.
	if err := db.Model(&user.User{}).Where("id = ?", 1).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterOauthRoutes(router.Group("/oauth"), NewOauthHandler(service), manager)
	request := func(target, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if response := request("/oauth/login?provider=oai&user_id=2", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated login = %d", response.Code)
	}
	login := request("/oauth/login?provider=oai&user_id=2", localToken)
	if login.Code != http.StatusFound {
		t.Fatalf("login = %d: %s", login.Code, login.Body.String())
	}
	location, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if query.Get("nonce") == "" || query.Get("state") == "" || query.Get("code_challenge") == "" {
		t.Fatal("login correlation fields missing")
	}
	service.directory = testDirectory(service, &tokenExchange{token: signedToken(t, key, jwt.MapClaims{"nonce": query.Get("nonce"), "user_id": 2})})
	callback := "/oauth/callback?state=" + query.Get("state") + "&code=code&user_id=2"
	response := request(callback, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "oauth credentials saved") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("callback = %d: %s", response.Code, response.Body.String())
	}
	var saved OAuthInfo
	if err := db.First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.UserID != 1 {
		t.Fatal("callback user input overrode authenticated flow owner")
	}
	if response := request(callback, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("replayed callback = %d", response.Code)
	}
}

func TestBrowserCallbackRedirectsAfterSaving(t *testing.T) {
	service, db, key := persistenceService(t)
	redisServer := miniredis.RunT(t)
	service.redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = service.redisClient.Close() })
	flow := oidc.LoginFlow{Provider: "oai", UserID: 1, Nonce: "nonce", Verifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}
	if err := service.StoreFlow("browser-state", flow, t.Context()); err != nil {
		t.Fatal(err)
	}
	service.directory = testDirectory(service, &tokenExchange{token: signedToken(t, key, nil)})
	router := gin.New()
	router.GET("/oauth/callback", NewOauthHandler(service).Callback)
	request := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=browser-state&code=code", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/authorized-accounts" {
		t.Fatalf("callback = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("callback must not cache or forward its URL as referrer")
	}
	var saved OAuthInfo
	if err := db.First(&saved).Error; err != nil || saved.UserID != 1 {
		t.Fatalf("credentials not saved before redirect: %v", err)
	}
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, request)
	if replay.Code != http.StatusBadRequest || replay.Header().Get("Location") != "" {
		t.Fatalf("replayed callback = %d", replay.Code)
	}
}

func TestCallbackReportsPersistenceAndIdentityFailures(t *testing.T) {
	service, db, key := persistenceService(t)
	redisServer := miniredis.RunT(t)
	service.redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = service.redisClient.Close() })
	for _, tc := range []struct {
		name  string
		token *oauth2.Token
		owner uint64
		want  int
	}{
		{"missing token", &oauth2.Token{AccessToken: "secret"}, 1, 502},
		{"bad nonce", signedToken(t, key, jwt.MapClaims{"nonce": "bad"}), 1, 502},
		{"database failure", signedToken(t, key, nil), 1, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == 500 {
				if err := db.Migrator().DropTable(&OAuthInfo{}); err != nil {
					t.Fatal(err)
				}
			}
			service.directory = testDirectory(service, &tokenExchange{token: tc.token})
			if err := service.StoreFlow("state", oidc.LoginFlow{Provider: "oai", UserID: tc.owner, Nonce: "nonce", Verifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}, t.Context()); err != nil {
				t.Fatal(err)
			}
			response := testOAuthCallback(service, "/callback?state=state&code=code")
			if response.Code != tc.want || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func (s *tokenExchange) RefreshToken(context.Context, string, string) (*oauth2.Token, error) {
	return nil, ErrRefreshFailed
}
