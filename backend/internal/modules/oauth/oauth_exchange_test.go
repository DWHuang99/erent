package oauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"erent/internal/modules/oauth/oidc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

type exchangeStub struct {
	err                      error
	provider, code, verifier string
}

func (s *exchangeStub) Exchange(_ context.Context, code, verifier, provider string) (*oauth2.Token, error) {
	s.code, s.verifier, s.provider = code, verifier, provider
	return nil, s.err
}
func TestCallbackMapsExchangeFailures(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{ErrInvalidExchange, 400}, {ErrExchangeRejected, 400}, {ErrProviderUnavailable, 503}, {ErrUpstreamUnavailable, 503}, {ErrExchangeTimeout, 504}, {ErrExchangeFailed, 502}, {errors.New("secret-provider-response"), 502},
	} {
		t.Run(test.err.Error(), func(t *testing.T) {
			redisServer := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
			defer client.Close()
			service := newTestOAuthService(client)
			exchanger := &exchangeStub{err: test.err}
			service.exchanger = exchanger
			if err := service.StoreFlow("state", oidc.LoginFlow{Verifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}, t.Context()); err != nil {
				t.Fatal(err)
			}
			response := testOAuthCallback(service, "/callback?state=state&code=code")
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d, body=%s", response.Code, test.status, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatal("raw provider error leaked")
			}
			if exchanger.code != "code" || exchanger.verifier != "verifier" || exchanger.provider != "oai" {
				t.Fatalf("wrong exchange input: %+v", exchanger)
			}
			if _, err := service.PopFlow("state", t.Context()); !errors.Is(err, ErrInvalidOAuthState) {
				t.Fatalf("state not consumed: %v", err)
			}
		})
	}
}
func TestServiceWithoutExchangerReportsUnavailable(t *testing.T) {
	if _, err := newTestOAuthService(nil).Exchange(context.Background(), "code", "verifier"); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatal(err)
	}
}
