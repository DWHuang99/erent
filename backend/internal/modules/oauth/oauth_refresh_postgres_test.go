package oauth

import (
	"context"
	"os"
	"testing"
	"time"

	"erent/internal/security"
	"golang.org/x/oauth2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Requires a dedicated disposable PostgreSQL database; never point at application data.
func TestRefreshPostgresSerializesInstances(t *testing.T) {
	dsn := os.Getenv("OAUTH_REFRESH_TEST_DSN")
	if dsn == "" {
		t.Skip("OAUTH_REFRESH_TEST_DSN is not set")
	}
	open := func() *gorm.DB {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	db := open()
	if err := db.AutoMigrate(&OAuthInfo{}); err != nil {
		t.Fatal(err)
	}
	first, second := newTestOAuthService(nil), newTestOAuthService(nil)
	first.repository, second.repository = NewRepository(db), NewRepository(open())
	old := seedRefreshAccount(t, first)
	entered, release, secondEntered := make(chan struct{}), make(chan struct{}), make(chan string, 1)
	first.directory = testDirectory(first, &refreshExchange{refresh: func(ctx context.Context, token, provider string) (*oauth2.Token, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &oauth2.Token{AccessToken: "access-one", RefreshToken: "refresh-one"}, nil
	}})
	second.directory = testDirectory(second, &refreshExchange{refresh: func(_ context.Context, token, _ string) (*oauth2.Token, error) {
		secondEntered <- token
		return &oauth2.Token{AccessToken: "access-two", RefreshToken: "refresh-two"}, nil
	}})
	done1, done2 := make(chan error, 1), make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() { done1 <- first.RefreshToken(ctx, 1, old.ID) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first refresh did not start")
	}
	go func() { done2 <- second.RefreshToken(ctx, 1, old.ID) }()
	select {
	case <-secondEntered:
		close(release)
		t.Fatal("second instance bypassed row lock")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	for _, done := range []chan error{done1, done2} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("refresh did not finish")
		}
	}
	if token := <-secondEntered; token != "refresh-one" {
		t.Fatal("second instance used stale refresh token")
	}
	var saved OAuthInfo
	if err := db.First(&saved, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := security.Decrypt(first.encryptionKey, []byte(saved.RefreshToken))
	if err != nil || string(plain) != "refresh-two" {
		t.Fatal("latest rotation not persisted")
	}
}
