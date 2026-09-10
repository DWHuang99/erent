package oidc

import (
	"context"
	"erent/internal/config"
	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc"
	"testing"
	"time"
)

type remoteStub struct {
	upstream.UpstreamServiceClient
	metadata *upstream.ProviderResponse
}

func (s *remoteStub) GetProvider(ctx context.Context, request *upstream.ProviderRequest, _ ...grpc.CallOption) (*upstream.ProviderResponse, error) {
	return s.metadata, nil
}

func TestRemoteOIDCAuth(t *testing.T) {
	// Unreachable issuer proves initialization does not use HTTP.
	cfg := config.OIDCConfig{Provider: "oai", Issuer: "https://issuer.invalid", ClientID: "client", RedirectURL: "http://localhost/callback"}
	s := &remoteStub{metadata: &upstream.ProviderResponse{Issuer: cfg.Issuer, AuthURL: cfg.Issuer + "/auth", TokenURL: cfg.Issuer + "/token", RawClaims: []byte(`{}`)}}
	directory := upstreamdirectory.New(s, time.Second)
	auth, err := NewRemoteOIDCAuth(t.Context(), cfg, directory, map[string]string{"prompt": "login"}, []string{"openid"})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Provider != nil || auth.Verifier != nil || auth.OauthConfig.Endpoint.AuthURL != s.metadata.AuthURL {
		t.Fatal("unexpected local provider or endpoint")
	}
	s.metadata.Issuer = "https://other.invalid"
	if _, err := NewRemoteOIDCAuth(t.Context(), cfg, directory, nil, nil); err == nil {
		t.Fatal("issuer mismatch accepted")
	}
}
