package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/rpc/upstream"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Only test fixtures use this adapter; services always receive a concrete Directory.
type testTokenSource interface {
	Exchange(context.Context, string, string, string) (*oauth2.Token, error)
	RefreshToken(context.Context, string, string) (*oauth2.Token, error)
}
type serviceRPCClient struct {
	upstream.UpstreamServiceClient
	service *OauthService
	source  testTokenSource
}

func testDirectory(service *OauthService, source testTokenSource) *upstreamdirectory.Directory {
	return upstreamdirectory.New(&serviceRPCClient{service: service, source: source}, 15*time.Second)
}
func testRPCError(err error) error {
	if err == nil {
		return nil
	}
	code := codes.Internal
	switch {
	case errors.Is(err, ErrInvalidExchange), errors.Is(err, ErrInvalidRefresh):
		code = codes.InvalidArgument
	case errors.Is(err, ErrExchangeRejected), errors.Is(err, ErrRefreshRejected), errors.Is(err, ErrInvalidIDToken):
		code = codes.Unauthenticated
	case errors.Is(err, ErrProviderUnavailable):
		code = codes.FailedPrecondition
	case errors.Is(err, ErrUpstreamUnavailable):
		code = codes.Unavailable
	case errors.Is(err, ErrExchangeTimeout), errors.Is(err, ErrRefreshTimeout), errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	}
	return status.Error(code, "test upstream failure")
}
func testTokenResponse(token *oauth2.Token, err error) (*upstream.TokenResponse, error) {
	if err != nil {
		return nil, testRPCError(err)
	}
	if token == nil {
		return nil, nil
	}
	r := &upstream.TokenResponse{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType}
	r.IdToken, _ = token.Extra("id_token").(string)
	if !token.Expiry.IsZero() {
		r.ExpiresAt = timestamppb.New(token.Expiry)
	}
	return r, nil
}
func (c *serviceRPCClient) ExchangeCode(ctx context.Context, r *upstream.ExchangeCodeRequest, _ ...grpc.CallOption) (*upstream.TokenResponse, error) {
	if c.source == nil {
		return nil, status.Error(codes.Unavailable, "no test exchange")
	}
	return testTokenResponse(c.source.Exchange(ctx, r.Code, r.CodeVerifier, r.Provider))
}
func (c *serviceRPCClient) RefreshToken(ctx context.Context, r *upstream.RefreshTokenRequest, _ ...grpc.CallOption) (*upstream.TokenResponse, error) {
	if c.source == nil {
		return nil, status.Error(codes.Unavailable, "no test refresh")
	}
	return testTokenResponse(c.source.RefreshToken(ctx, r.RefreshToken, r.Provider))
}
func (c *serviceRPCClient) Verifier(ctx context.Context, r *upstream.VerifyRequest, _ ...grpc.CallOption) (*upstream.VerifyResponse, error) {
	auth := c.service.oidcAuth[r.Provider]
	if auth == nil || auth.Verifier == nil {
		return nil, status.Error(codes.FailedPrecondition, "no test verifier")
	}
	token, err := auth.Verifier.Verify(ctx, r.Rawidtoken)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "test verification failed")
	}
	var claims json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return nil, err
	}
	response := &upstream.VerifyResponse{Issuer: token.Issuer, Subject: token.Subject, Audience: token.Audience, Nonce: token.Nonce, ClaimsJson: claims, ExpiresAt: timestamppb.New(token.Expiry)}
	if !token.IssuedAt.IsZero() {
		response.IssuedAt = timestamppb.New(token.IssuedAt)
	}
	return response, nil
}
