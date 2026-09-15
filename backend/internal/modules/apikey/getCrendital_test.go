package apikey

import (
	"encoding/json"
	"erent/internal/security"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCredentialGetterUsesCheckResultWithoutDatabase(t *testing.T) {
	repo, raw := authRepository(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, err := security.Encrypt(key, []byte("first-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.database.Table("oauth_infos").Where("id = 10").Update("access_token", string(cipher)).Error; err != nil {
		t.Fatal(err)
	}
	checker := NewCheck()
	checker.inject(repo)
	accounts, err := checker.checkOauth(t.Context(), raw, "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	// Retrieval uses the returned snapshot without querying the database again.
	db, err := repo.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	getter := NewCredentialGetter()
	getter.inject(key)
	got, err := getter.getCredential(t.Context(), accounts)
	if err != nil || got.Secret != "first-token" || got.Mode != "access_token" {
		t.Fatal("incorrect credential", err)
	}
	encoded, err := json.Marshal(accounts)
	if err != nil || strings.Contains(string(encoded), string(cipher)) || strings.Contains(string(encoded), "access_token") {
		t.Fatal("credential exposed in JSON")
	}
	past := time.Now().Add(-time.Hour)
	for _, first := range []ApikeyAccountItem{
		{Type: "codex", AccessToken: string(cipher), Expired: &past},
		{Type: "codex", AccessToken: "broken"},
	} {
		got, err := getter.getCredential(t.Context(), []ApikeyAccountItem{first, accounts[0]})
		if !errors.Is(err, ErrCredentialUnavailable) || got.Secret != "" {
			t.Fatal("unexpected fallback", err)
		}
	}
	if _, err := getter.getCredential(t.Context(), nil); !errors.Is(err, ErrOauthForbidden) {
		t.Fatal(err)
	}
}
