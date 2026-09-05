package oauth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/oauth/oidc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

func newTestOAuthService(client *redis.Client) *OauthService {
	return NewOauthService(client, &oidc.OIDCAuth{
		OauthConfig: &oauth2.Config{
			ClientID:    "client-id",
			RedirectURL: "http://localhost/oauth/callback",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://issuer.example/authorize",
				TokenURL: "https://issuer.example/token",
			},
		},
		AuthURLParams: map[string]string{"prompt": "login"},
	}, nil, "oai")
}

func TestAuthCodeURLIncludesPKCEAndProviderParameters(t *testing.T) {
	service := newTestOAuthService(nil)
	verifier := oauth2.GenerateVerifier()
	authURL, err := url.Parse(service.AuthCodeURL("state-value", verifier))
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	query := authURL.Query()
	if query.Get("state") != "state-value" || query.Get("prompt") != "login" {
		t.Fatalf("unexpected auth URL query: %s", authURL.RawQuery)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(verifier) {
		t.Fatalf("missing PKCE challenge: %s", authURL.RawQuery)
	}
}

func TestStoreAndPopFlowConsumesStateOnce(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	flow := oidc.LoginFlow{
		Verifier:  "verifier",
		ExpiresAt: time.Now().Add(time.Minute).UTC().Truncate(time.Millisecond),
	}

	if err := service.StoreFlow("state", flow, context.Background()); err != nil {
		t.Fatalf("store flow: %v", err)
	}
	stored, err := service.PopFlow("state", context.Background())
	if err != nil {
		t.Fatalf("pop flow: %v", err)
	}
	if stored.Verifier != flow.Verifier || !stored.ExpiresAt.Equal(flow.ExpiresAt) {
		t.Fatalf("unexpected stored flow: %+v", stored)
	}
	if _, err := service.PopFlow("state", context.Background()); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("second pop error = %v, want ErrInvalidOAuthState", err)
	}
}

func TestPopFlowSeparatesInvalidStateFromRedisFailure(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)

	if _, err := service.PopFlow("missing", context.Background()); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("missing state error = %v, want ErrInvalidOAuthState", err)
	}

	redisServer.Close()
	if _, err := service.PopFlow("state", context.Background()); err == nil || errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("Redis failure error = %v, want infrastructure error", err)
	}
}

func TestPopFlowReportsMalformedStoredStateAsServerError(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)

	if err := rdb.SetState(client, context.Background(), "state", []byte("{"), time.Minute); err != nil {
		t.Fatalf("seed malformed state: %v", err)
	}
	if _, err := service.PopFlow("state", context.Background()); err == nil || errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("malformed state error = %v, want decode error", err)
	}
}
