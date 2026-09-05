package upstreamserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"erent/internal/config"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/transport"
	"erent/internal/rpc/upstream"

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
	token, err := auth.OauthConfig.Exchange(ctx, request.Code, oauth2.VerifierOption(request.CodeVerifier))
	if err != nil {
		code := exchangeErrorCode(ctx, err)
		// Provider response bodies can contain credentials. Log only classified metadata.
		slog.Warn("OAuth token exchange failed", "provider", request.Provider, "code", code.String())
		return nil, status.Error(code, "token exchange failed")
	}
	response := &upstream.TokenResponse{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType}
	if !token.Expiry.IsZero() {
		response.ExpiresAt = timestamppb.New(token.Expiry)
	}
	return response, nil
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
