package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erent/internal/config"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

type refreshExchange struct {
	refresh func(context.Context, string, string) (*oauth2.Token, error)
}

func (s *refreshExchange) Exchange(context.Context, string, string, string) (*oauth2.Token, error) {
	return nil, ErrExchangeFailed
}
func (s *refreshExchange) RefreshToken(ctx context.Context, token, provider string) (*oauth2.Token, error) {
	return s.refresh(ctx, token, provider)
}

func seedRefreshAccount(t *testing.T, service *OauthService) OAuthInfo {
	t.Helper()
	expiry := time.Now().Add(-time.Hour)
	model, err := service.toOauthInfo(0, 1, "account", "owner@example.com", "old-access", "old-refresh", "old-id", &expiry)
	if err != nil {
		t.Fatal(err)
	}
	model.Disabled = true
	if err := service.repository.SaveToken(t.Context(), &model); err != nil {
		t.Fatal(err)
	}
	return model
}

func TestRefreshPersistsNewTokensAndPreservesAccount(t *testing.T) {
	service, db, key := persistenceService(t)
	old := seedRefreshAccount(t, service)
	fresh := signedToken(t, key, jwt.MapClaims{"nonce": "different-refresh-nonce"})
	fresh.AccessToken = "new-access"
	fresh.RefreshToken = "new-refresh"
	calls := 0
	service.exchanger = &refreshExchange{refresh: func(ctx context.Context, token, provider string) (*oauth2.Token, error) {
		calls++
		if token != "old-refresh" || provider != "oai" {
			t.Fatal("incorrect stored refresh input")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("refresh lacks deadline")
		}
		return fresh, nil
	}}
	if err := service.RefreshToken(t.Context(), 1, old.ID); err != nil {
		t.Fatal(err)
	}
	var saved OAuthInfo
	if err := db.First(&saved, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	for ciphertext, want := range map[string]string{saved.AccessToken: "new-access", saved.RefreshToken: "new-refresh", saved.IDToken: fresh.Extra("id_token").(string)} {
		plain, err := security.Decrypt(service.encryptionKey, []byte(ciphertext))
		if err != nil || string(plain) != want || ciphertext == want {
			t.Fatal("fresh token was not encrypted and persisted")
		}
	}
	if calls != 1 || !saved.Disabled || saved.UserID != old.UserID || saved.AccountID != old.AccountID || saved.Email != old.Email || saved.Type != old.Type || !saved.CreatedAt.Equal(old.CreatedAt) || saved.Expired == nil || !saved.Expired.Equal(fresh.Expiry) || saved.LastRefresh == nil {
		t.Fatal("account metadata or expiry corrupted")
	}
	// The next call reads the newly persisted token, preserving omitted optional credentials.
	service.exchanger = &refreshExchange{refresh: func(_ context.Context, token, _ string) (*oauth2.Token, error) {
		if token != "new-refresh" {
			t.Fatal("did not reread latest refresh token")
		}
		return &oauth2.Token{AccessToken: "another-access"}, nil
	}}
	if err := service.RefreshToken(t.Context(), 1, old.ID); err != nil {
		t.Fatal(err)
	}
	var latest OAuthInfo
	if err := db.First(&latest, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if latest.RefreshToken != saved.RefreshToken || latest.IDToken != saved.IDToken || latest.Expired != nil {
		t.Fatal("optional credentials lost or stale expiry retained")
	}
}

func TestRefreshHTTPUsesJWTAndReportsErrors(t *testing.T) {
	service, db, _ := persistenceService(t)
	old := seedRefreshAccount(t, service)
	calls := 0
	var upstreamErr error
	service.exchanger = &refreshExchange{refresh: func(_ context.Context, token, provider string) (*oauth2.Token, error) {
		calls++
		if token != "old-refresh" || provider != "oai" {
			t.Fatal("request overrode stored credentials")
		}
		return &oauth2.Token{AccessToken: "fresh-access"}, upstreamErr
	}}
	manager := jwtservice.NewJWTManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", Audience: "test", AccessTTL: time.Hour})
	router := gin.New()
	RegisterOauthRoutes(router.Group("/oauth"), NewOauthHandler(service), manager)
	request := func(owner uint64, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/oauth/refresh?provider=evil", strings.NewReader(body))
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
		return w
	}
	body, _ := json.Marshal(map[string]any{"id": old.ID, "userId": 1, "refreshtoken": "attacker-token", "accountId": "attacker", "expired": nil})
	for _, tc := range []struct {
		owner uint64
		body  string
		want  int
	}{
		{0, string(body), 401}, {2, string(body), 404}, {1, `{"id":999999}`, 404},
		{1, `{`, 400}, {1, `{}`, 400}, {1, `{"id":0}`, 400}, {1, `{"id":-1}`, 400},
	} {
		if w := request(tc.owner, tc.body); w.Code != tc.want {
			t.Fatalf("got %d: %s, want %d", w.Code, w.Body, tc.want)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized or invalid request reached upstream")
	}
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrRefreshRejected, 400}, {ErrUpstreamUnavailable, 503}, {ErrRefreshTimeout, 504}, {ErrRefreshFailed, 502}, {errors.New("secret-database-error"), 500},
	} {
		upstreamErr = tc.err
		w := request(1, string(body))
		if w.Code != tc.want || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "refresh success") {
			t.Fatalf("failed refresh: %d %s", w.Code, w.Body)
		}
	}
	var unchanged OAuthInfo
	if err := db.First(&unchanged, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.AccessToken != old.AccessToken || unchanged.RefreshToken != old.RefreshToken {
		t.Fatal("failed refresh changed credentials")
	}
	upstreamErr = nil
	w := request(1, string(body))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "fresh-access") {
		t.Fatalf("success: %d %s", w.Code, w.Body)
	}
}

func TestRefreshRejectsBadIdentityAndMissingCredentials(t *testing.T) {
	service, db, key := persistenceService(t)
	old := seedRefreshAccount(t, service)
	for _, token := range []*oauth2.Token{nil, {}, {AccessToken: "expired", Expiry: time.Now().Add(-time.Minute)}, signedToken(t, key, jwt.MapClaims{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "other-account"}})} {
		service.exchanger = &refreshExchange{refresh: func(context.Context, string, string) (*oauth2.Token, error) { return token, nil }}
		if err := service.RefreshToken(t.Context(), 1, old.ID); err == nil {
			t.Fatal("invalid refresh response accepted")
		}
	}
	service.exchanger = &refreshExchange{refresh: func(context.Context, string, string) (*oauth2.Token, error) {
		t.Fatal("corrupt credentials reached upstream")
		return nil, nil
	}}
	if err := db.Model(&OAuthInfo{}).Where("id = ?", old.ID).Update("refresh_token", "invalid ciphertext").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RefreshToken(t.Context(), 1, old.ID); err == nil {
		t.Fatal("corrupt ciphertext accepted")
	}
	empty, err := security.Encrypt(service.encryptionKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&OAuthInfo{}).Where("id = ?", old.ID).Update("refresh_token", string(empty)).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RefreshToken(t.Context(), 1, old.ID); !errors.Is(err, ErrRefreshRejected) {
		t.Fatalf("empty token: %v", err)
	}
}

func TestRefreshTransactionRollsBackWhenUpdateFails(t *testing.T) {
	service, db, _ := persistenceService(t)
	old := seedRefreshAccount(t, service)
	service.exchanger = &refreshExchange{refresh: func(context.Context, string, string) (*oauth2.Token, error) {
		return nil, ErrRefreshFailed
	}}
	err := service.RefreshToken(t.Context(), 1, old.ID)
	if !errors.Is(err, ErrRefreshFailed) {
		t.Fatal(err)
	}
	var saved OAuthInfo
	if err := db.First(&saved, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != old.AccessToken {
		t.Fatal("transaction persisted failed result")
	}
	if err := db.Exec("CREATE TRIGGER reject_token_update BEFORE UPDATE ON oauth_infos BEGIN SELECT RAISE(ABORT, 'update rejected'); END").Error; err != nil {
		t.Fatal(err)
	}
	service.exchanger = &refreshExchange{refresh: func(context.Context, string, string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "new-value", RefreshToken: "new-refresh"}, nil
	}}
	err = service.RefreshToken(t.Context(), 1, old.ID)
	if err == nil {
		t.Fatal("database update failure reported success")
	}
	if err := db.First(&saved, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != old.AccessToken {
		t.Fatal("failed update changed token")
	}
	if _, err := service.toOauthInfo(0, 1, "account", "email", "access", "refresh", "id", nil); err != nil {
		t.Fatal(err)
	}
}
