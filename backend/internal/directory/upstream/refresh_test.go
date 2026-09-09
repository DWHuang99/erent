package upstreamdirectory

import (
	"context"
	"errors"
	"testing"
	"time"

	"erent/internal/modules/oauth"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type refreshClient struct {
	upstream.UpstreamServiceClient
	response *upstream.TokenResponse
	err      error
}

func (c *refreshClient) RefreshToken(ctx context.Context, req *upstream.RefreshTokenRequest, _ ...grpc.CallOption) (*upstream.TokenResponse, error) {
	return c.response, c.err
}

func TestDirectoryRefreshPreservesOptionalFields(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC()
	client := &refreshClient{response: &upstream.TokenResponse{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer", IdToken: "identity", ExpiresAt: timestamppb.New(expiry)}}
	directory := New(client, time.Second)
	token, err := directory.RefreshToken(t.Context(), "old-refresh", "oai")
	if err != nil || token.AccessToken != "access" || token.RefreshToken != "refresh" || token.TokenType != "Bearer" || !token.Expiry.Equal(expiry) || token.Extra("id_token") != "identity" {
		t.Fatalf("token fields lost: %v", err)
	}
	for _, response := range []*upstream.TokenResponse{nil, {}, {AccessToken: " "}, {AccessToken: "access", ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800}}} {
		client.response = response
		if token, err := directory.RefreshToken(t.Context(), "refresh", "oai"); token != nil || !errors.Is(err, oauth.ErrRefreshFailed) {
			t.Fatalf("invalid response accepted: %v", err)
		}
	}
}

func TestDirectoryRefreshMapsErrors(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want error
	}{
		{codes.InvalidArgument, oauth.ErrInvalidRefresh}, {codes.FailedPrecondition, oauth.ErrProviderUnavailable},
		{codes.Unauthenticated, oauth.ErrRefreshRejected}, {codes.Unavailable, oauth.ErrUpstreamUnavailable},
		{codes.DeadlineExceeded, oauth.ErrRefreshTimeout}, {codes.Canceled, context.Canceled}, {codes.Internal, oauth.ErrRefreshFailed},
	} {
		directory := New(&refreshClient{err: status.Error(tc.code, "secret-provider-details")}, time.Second)
		if token, err := directory.RefreshToken(t.Context(), "refresh", "oai"); token != nil || !errors.Is(err, tc.want) {
			t.Fatalf("code %v: %v", tc.code, err)
		}
	}
}
