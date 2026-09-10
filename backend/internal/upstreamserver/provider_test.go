package upstreamserver

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"testing"
	"time"

	localoidc "erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/upstream"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetProviderRPC(t *testing.T) {
	auth := testProvider(t, nil, "")
	client := testRPC(t, auth, time.Second)
	var metadata struct {
		Issuer string `json:"issuer"`
	}
	if err := auth.Provider.Claims(&metadata); err != nil {
		t.Fatal(err)
	}
	response, err := client.GetProvider(t.Context(), &upstream.ProviderRequest{Issuer: metadata.Issuer})
	if err != nil {
		t.Fatal(err)
	}
	if response.Issuer != metadata.Issuer || response.AuthURL != auth.OauthConfig.Endpoint.AuthURL || response.TokenURL != auth.OauthConfig.Endpoint.TokenURL || !json.Valid(response.RawClaims) {
		t.Fatalf("invalid provider response: %v", response)
	}
	for _, tc := range []struct {
		issuer string
		code   codes.Code
	}{{"", codes.InvalidArgument}, {"http://127.0.0.1:1", codes.FailedPrecondition}} {
		_, err := client.GetProvider(t.Context(), &upstream.ProviderRequest{Issuer: tc.issuer})
		if status.Code(err) != tc.code {
			t.Fatalf("%q: %v", tc.issuer, err)
		}
	}
}

func TestVerifierRPC(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := gooidc.NewVerifier("https://issuer.example", &gooidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &gooidc.Config{ClientID: "client"})
	client := testRPC(t, &localoidc.OIDCAuth{Verifier: verifier}, time.Second)
	now := time.Now().Unix()
	claims := jwt.MapClaims{"iss": "https://issuer.example", "sub": "subject", "aud": []string{"client"}, "iat": now, "exp": now + 3600, "nonce": "login-nonce", "email": "user@example.com"}
	sign := func(c jwt.MapClaims) string {
		raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	response, err := client.Verifier(t.Context(), &upstream.VerifyRequest{Provider: "oai", Rawidtoken: sign(claims)})
	if err != nil {
		t.Fatal(err)
	}
	if response.Issuer != claims["iss"] || response.Subject != "subject" || len(response.Audience) != 1 || response.Audience[0] != "client" || response.Nonce != "login-nonce" || response.ExpiresAt.AsTime().Unix() != now+3600 || response.IssuedAt.AsTime().Unix() != now {
		t.Fatalf("invalid response: %v", response)
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.ClaimsJson, &decoded); err != nil || decoded["email"] != "user@example.com" {
		t.Fatalf("claims not preserved: %v", err)
	}
	for _, field := range []string{"iss", "aud", "exp"} {
		bad := jwt.MapClaims{}
		for k, v := range claims {
			bad[k] = v
		}
		bad[field] = "wrong"
		if field == "exp" {
			bad[field] = now - 3600
		}
		_, err := client.Verifier(t.Context(), &upstream.VerifyRequest{Provider: "oai", Rawidtoken: sign(bad)})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: %v", field, err)
		}
	}
	_, err = client.Verifier(t.Context(), &upstream.VerifyRequest{Provider: "oai", Rawidtoken: "secret-malformed-token"})
	if status.Code(err) != codes.Unauthenticated || strings.Contains(err.Error(), "secret-malformed-token") {
		t.Fatalf("unsafe error: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.Verifier(ctx, &upstream.VerifyRequest{Provider: "oai", Rawidtoken: sign(claims)})
	if status.Code(err) != codes.Canceled {
		t.Fatal(err)
	}
}

func TestProviderValidation(t *testing.T) {
	s := NewServer(nil, time.Second)
	if _, err := s.GetProvider(t.Context(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.Verifier(t.Context(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.Verifier(t.Context(), &upstream.VerifyRequest{Provider: "missing", Rawidtoken: "token"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := toProviderResponse(nil); err == nil {
		t.Fatal("nil provider accepted")
	}
	if _, err := toVerifyResponse(nil); err == nil {
		t.Fatal("nil token accepted")
	}
}
