package upstreamdirectory

import (
	"context"
	"errors"
	"testing"
	"time"

	"erent/internal/rpc/upstream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type providerClient struct {
	upstream.UpstreamServiceClient
	err                   error
	issuer, raw, provider string
	deadline              bool
}

func (c *providerClient) GetProvider(ctx context.Context, r *upstream.ProviderRequest, _ ...grpc.CallOption) (*upstream.ProviderResponse, error) {
	c.issuer = r.Issuer
	_, c.deadline = ctx.Deadline()
	return &upstream.ProviderResponse{Issuer: r.Issuer}, c.err
}
func (c *providerClient) Verifier(ctx context.Context, r *upstream.VerifyRequest, _ ...grpc.CallOption) (*upstream.VerifyResponse, error) {
	c.raw, c.provider = r.Rawidtoken, r.Provider
	_, c.deadline = ctx.Deadline()
	return &upstream.VerifyResponse{Subject: "subject"}, c.err
}
func TestProviderDirectory(t *testing.T) {
	c := &providerClient{}
	d := New(c, time.Second)
	p, err := d.GetProvider(t.Context(), "https://issuer.example")
	if err != nil || p.Issuer != c.issuer || !c.deadline {
		t.Fatalf("provider forwarding: %v", err)
	}
	v, err := d.Verifier(t.Context(), "token", "oai")
	if err != nil || v.Subject != "subject" || c.raw != "token" || c.provider != "oai" || !c.deadline {
		t.Fatalf("verification forwarding: %v", err)
	}
	for _, tc := range []struct {
		code codes.Code
		want error
	}{
		{codes.Unauthenticated, ErrInvalidIDToken},
		{codes.FailedPrecondition, ErrProviderUnavailable},
		{codes.DeadlineExceeded, ErrUpstreamUnavailable},
		{codes.Unavailable, ErrUpstreamUnavailable},
		{codes.Canceled, context.Canceled},
	} {
		c.err = status.Error(tc.code, "sensitive upstream error")
		if _, err := d.Verifier(t.Context(), "token", "oai"); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if _, err := d.GetProvider(t.Context(), "issuer"); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.code, err)
		}
	}
}
