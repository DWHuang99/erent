package upstreamserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erent/internal/config"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/transport"
	"erent/internal/rpc/upstream"

	oidcgo "github.com/coreos/go-oidc/v3/oidc"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type server struct {
	upstream.UnimplementedUpstreamServiceServer
	oidcAuth map[string]*oidc.OIDCAuth
	timeout  time.Duration
}

func NewServer(oidcAuth map[string]*oidc.OIDCAuth, timeout time.Duration) *server {
	return &server{oidcAuth: oidcAuth, timeout: timeout}
}
func (s *server) ExchangeCode(ctx context.Context, request *upstream.ExchangeCodeRequest) (*upstream.TokenResponse, error) {
	if request == nil || strings.TrimSpace(request.Code) == "" || strings.TrimSpace(request.CodeVerifier) == "" || request.Provider == "" {
		return nil, status.Error(codes.InvalidArgument, "code, code_verifier and provider are required")
	}
	auth := s.oidcAuth[request.Provider]
	if auth == nil || auth.OauthConfig == nil {
		return nil, status.Error(codes.FailedPrecondition, "provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cfg := *auth.OauthConfig
	switch request.FlowType {
	case "", "browser":
	case "device":
		if request.Provider != "oai" {
			return nil, status.Error(codes.InvalidArgument, "unsupported device provider")
		}
		cfg.RedirectURL = "https://auth.openai.com/deviceauth/callback"
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid flow type")
	}
	token, err := cfg.Exchange(ctx, request.Code, oauth2.VerifierOption(request.CodeVerifier))
	if err != nil {
		code := exchangeErrorCode(ctx, err)
		// Provider response bodies can contain credentials. Log only classified metadata.
		slog.Warn("OAuth token exchange failed", "provider", request.Provider, "code", code.String())
		return nil, status.Error(code, "token exchange failed")
	}
	return tokenResponse(token), nil
}

func tokenResponse(token *oauth2.Token) *upstream.TokenResponse {
	response := &upstream.TokenResponse{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType}
	response.IdToken, _ = token.Extra("id_token").(string)
	if !token.Expiry.IsZero() {
		response.ExpiresAt = timestamppb.New(token.Expiry)
	}
	return response
}

func exchangeErrorCode(ctx context.Context, err error) codes.Code {
	if ctx.Err() != nil {
		return status.FromContextError(ctx.Err()).Code()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}
	var providerError *oauth2.RetrieveError
	if errors.As(err, &providerError) {
		if providerError.ErrorCode == "invalid_grant" {
			return codes.Unauthenticated
		}
		if providerError.Response != nil && (providerError.Response.StatusCode == http.StatusTooManyRequests || providerError.Response.StatusCode >= 500) {
			return codes.Unavailable
		}
		return codes.Internal
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return codes.Unavailable
	}
	return codes.Internal
}

func (s *server) RefreshToken(ctx context.Context, request *upstream.RefreshTokenRequest) (*upstream.TokenResponse, error) {
	if request == nil || strings.TrimSpace(request.RefreshToken) == "" || strings.TrimSpace(request.Provider) == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token and provider are required")
	}
	auth := s.oidcAuth[request.Provider]
	if auth == nil || auth.OauthConfig == nil {
		return nil, status.Error(codes.FailedPrecondition, "provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	token, err := auth.OauthConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: request.RefreshToken}).Token()
	if err != nil {
		code := exchangeErrorCode(ctx, err)
		// Never forward provider response bodies or credential-bearing error text.
		slog.Warn("OAuth token refresh failed", "provider", request.Provider, "code", code.String())
		return nil, status.Error(code, "token refresh failed")
	}
	return tokenResponse(token), nil
}

func (s *server) GetProvider(ctx context.Context, request *upstream.ProviderRequest) (*upstream.ProviderResponse, error) {
	if request == nil || strings.TrimSpace(request.Issuer) == "" {
		return nil, status.Error(codes.InvalidArgument, "issuer is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	for _, auth := range s.oidcAuth {
		if auth == nil || auth.Provider == nil {
			continue
		}
		response, err := toProviderResponse(auth.Provider)
		if err != nil {
			return nil, status.Error(codes.Internal, "invalid provider metadata")
		}
		if response.Issuer == request.Issuer {
			return response, nil
		}
	}
	return nil, status.Error(codes.FailedPrecondition, "issuer is not configured")
}

func toProviderResponse(provider *oidcgo.Provider) (*upstream.ProviderResponse, error) {
	if provider == nil {
		return nil, errors.New("missing provider")
	}
	// issuer, jwksURL and algorithms are unexported on oidcgo.Provider; recover them via the discovery document.
	var metadata struct {
		Issuer     string   `json:"issuer"`
		JWKSURL    string   `json:"jwks_uri"`
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, err
	}
	var rawClaims json.RawMessage
	if err := provider.Claims(&rawClaims); err != nil {
		return nil, err
	}
	endpoint := provider.Endpoint()
	return &upstream.ProviderResponse{
		Issuer:        metadata.Issuer,
		AuthURL:       endpoint.AuthURL,
		TokenURL:      endpoint.TokenURL,
		DeviceAuthURL: endpoint.DeviceAuthURL,
		UserInfoURL:   provider.UserInfoEndpoint(),
		JwksURL:       metadata.JWKSURL,
		Algorithms:    metadata.Algorithms,
		RawClaims:     rawClaims,
	}, nil
}

func (s *server) Verifier(ctx context.Context, request *upstream.VerifyRequest) (*upstream.VerifyResponse, error) {
	if request == nil || strings.TrimSpace(request.Provider) == "" || strings.TrimSpace(request.Rawidtoken) == "" {
		return nil, status.Error(codes.InvalidArgument, "rawidtoken and provider are required")
	}
	auth := s.oidcAuth[request.Provider]
	if auth == nil || auth.Verifier == nil {
		return nil, status.Error(codes.FailedPrecondition, "provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	verified, err := auth.Verifier.Verify(ctx, request.Rawidtoken)
	if err != nil {
		code := exchangeErrorCode(ctx, err)
		if code == codes.Internal {
			code = codes.Unauthenticated
		}
		return nil, status.Error(code, "ID token verification failed")
	}
	response, err := toVerifyResponse(verified)
	if err != nil {
		return nil, status.Error(codes.Internal, "invalid verified claims")
	}
	return response, nil
}

func toVerifyResponse(token *oidcgo.IDToken) (*upstream.VerifyResponse, error) {
	if token == nil {
		return nil, errors.New("missing ID token")
	}
	var claims json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return nil, err
	}
	response := &upstream.VerifyResponse{
		Issuer: token.Issuer, Subject: token.Subject,
		Audience: append([]string(nil), token.Audience...),
		Nonce:    token.Nonce, ClaimsJson: claims,
	}
	if !token.Expiry.IsZero() {
		response.ExpiresAt = timestamppb.New(token.Expiry)
		if err := response.ExpiresAt.CheckValid(); err != nil {
			return nil, err
		}
	}
	if !token.IssuedAt.IsZero() {
		response.IssuedAt = timestamppb.New(token.IssuedAt)
		if err := response.IssuedAt.CheckValid(); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func Serve(ctx context.Context, cfg config.UpstreamServerConfig, oidcAuth map[string]*oidc.OIDCAuth) error {
	credentials, err := transport.ServerCredentials(cfg.TLS)
	if err != nil {
		return err
	}
	var options []grpc.ServerOption
	if credentials != nil {
		options = append(options, grpc.Creds(credentials))
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return err
	}
	defer listener.Close()
	grpcServer := grpc.NewServer(options...)
	upstream.RegisterUpstreamServiceServer(grpcServer, NewServer(oidcAuth, cfg.RequestTimeout))
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	serviceStatus := healthpb.HealthCheckResponse_NOT_SERVING
	if auth := oidcAuth["oai"]; auth != nil && auth.OauthConfig != nil {
		serviceStatus = healthpb.HealthCheckResponse_SERVING
	}
	healthServer.SetServingStatus(upstream.UpstreamService_ServiceDesc.ServiceName, serviceStatus)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			healthServer.Shutdown()
			forceStop := time.AfterFunc(cfg.ShutdownTimeout, grpcServer.Stop)
			grpcServer.GracefulStop()
			forceStop.Stop()
		case <-done:
		}
	}()
	slog.Info("upstream gRPC server starting", "address", cfg.Address)
	err = grpcServer.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

const LoginDeviceFlowEndpoint = "https://auth.openai.com/api/accounts/deviceauth/usercode"
const PostTokenEndpoint = "https://auth.openai.com/api/accounts/deviceauth/token"

var httpClient = &http.Client{Timeout: 15 * time.Second}

func (s *server) GetDeviceFlowCode(ctx context.Context, request *upstream.DeviceFlowRequest) (*upstream.DeviceFlowResponse, error) {
	if request == nil || request.Provider == "" {
		return nil, status.Error(codes.InvalidArgument, "provider is required")
	}
	auth := s.oidcAuth[request.Provider]
	if request.Provider != "oai" || auth == nil || auth.OauthConfig == nil || auth.OauthConfig.ClientID == "" {
		return nil, status.Error(codes.FailedPrecondition, "device provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"client_id": auth.OauthConfig.ClientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LoginDeviceFlowEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, status.Error(codes.Internal, "invalid device request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, status.Error(exchangeErrorCode(ctx, err), "device authorization request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, deviceHTTPError(resp.StatusCode)
	}
	var result struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     string `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, status.Error(codes.Internal, "invalid device authorization response")
	}
	interval, err := strconv.ParseUint(result.Interval, 10, 32)
	if err != nil || interval == 0 || interval > 900 || result.DeviceAuthID == "" || result.UserCode == "" {
		return nil, status.Error(codes.Internal, "invalid device authorization response")
	}
	return &upstream.DeviceFlowResponse{DeviceAuthId: result.DeviceAuthID, UserCode: result.UserCode, IntervalSeconds: uint32(interval), VerificationUrl: "https://auth.openai.com/codex/device"}, nil
}

// A nil result with no error means approval is still pending.
func postToken(request *http.Request) (*upstream.DeviceAuthorizationResponse, error) {
	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, status.Error(exchangeErrorCode(request.Context(), err), "device authorization request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, deviceHTTPError(resp.StatusCode)
	}
	var result struct {
		Code         string `json:"authorization_code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Code == "" || result.CodeVerifier == "" {
		return nil, status.Error(codes.Internal, "invalid device authorization response")
	}
	return &upstream.DeviceAuthorizationResponse{AuthorizationCode: result.Code, CodeVerifier: result.CodeVerifier}, nil
}

func (s *server) PollDeviceFlow(ctx context.Context, request *upstream.PollDeviceFlowRequest) (*upstream.DeviceAuthorizationResponse, error) {
	if request == nil || request.Provider == "" || request.DeviceAuthId == "" || request.UserCode == "" || request.IntervalSeconds == 0 || request.IntervalSeconds > 900 {
		return nil, status.Error(codes.InvalidArgument, "device auth ID, user code and valid interval are required")
	}
	auth := s.oidcAuth[request.Provider]
	if request.Provider != "oai" || auth == nil || auth.OauthConfig == nil {
		return nil, status.Error(codes.FailedPrecondition, "device provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"device_auth_id": request.DeviceAuthId, "user_code": request.UserCode})
	for {
		requestCtx, requestCancel := context.WithTimeout(ctx, s.timeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, PostTokenEndpoint, bytes.NewReader(body))
		if err != nil {
			requestCancel()
			return nil, status.Error(codes.Internal, "invalid device request")
		}
		req.Header.Set("Content-Type", "application/json")
		result, err := postToken(req)
		requestCancel()
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}

		timer := time.NewTimer(time.Duration(request.IntervalSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, status.FromContextError(ctx.Err()).Err()
		case <-timer.C:
		}
	}
}

// Preserve the directory's existing error contract without exposing response bodies.
func deviceHTTPError(httpStatus int) error {
	code := codes.Internal
	switch {
	case httpStatus == http.StatusForbidden || httpStatus == http.StatusNotFound:
		code = codes.FailedPrecondition
	case httpStatus == http.StatusBadRequest || httpStatus == http.StatusUnauthorized:
		code = codes.Unauthenticated
	case httpStatus == http.StatusTooManyRequests || httpStatus >= 500:
		code = codes.Unavailable
	}
	return status.Error(code, "device authorization rejected")
}
