package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc"
)

type verificationClient struct {
	upstream.UpstreamServiceClient
	response      *upstream.VerifyResponse
	provider, raw string
}

func (c *verificationClient) Verifier(ctx context.Context, r *upstream.VerifyRequest, _ ...grpc.CallOption) (*upstream.VerifyResponse, error) {
	c.provider, c.raw = r.Provider, r.Rawidtoken
	return c.response, nil
}

func TestVerifyIDTokenReturnsUpstreamResponse(t *testing.T) {
	response := &upstream.VerifyResponse{Nonce: "nonce", ClaimsJson: []byte(`{"email":"user@example.com"}`)}
	client := &verificationClient{response: response}
	directory := upstreamdirectory.New(client, time.Second)
	got, err := verifyIDToken(t.Context(), directory, "raw-token", "oai")
	if err != nil {
		t.Fatal(err)
	}
	if got != response || client.provider != "oai" || client.raw != "raw-token" {
		t.Fatal("upstream response or request changed")
	}
	if _, err := verifyIDToken(t.Context(), nil, "raw", "oai"); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatal(err)
	}
}
