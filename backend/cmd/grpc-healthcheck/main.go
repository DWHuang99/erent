package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"erent/internal/config"
	"erent/internal/rpc/transport"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func check() error {
	cfg, err := config.LoadUpstreamClientConfig()
	if err != nil {
		return err
	}
	// A server-only certificate cannot authenticate the health probe as a client.
	if cert := os.Getenv("UPSTREAM_HEALTHCHECK_TLS_CERT_FILE"); cert != "" {
		cfg.TLS.CertFile = cert
		cfg.TLS.KeyFile = os.Getenv("UPSTREAM_HEALTHCHECK_TLS_KEY_FILE")
	}
	credentials, err := transport.ClientCredentials(cfg.TLS)
	if err != nil {
		return err
	}
	connection, err := grpc.NewClient(cfg.Target, grpc.WithTransportCredentials(credentials), grpc.WithDisableRetry())
	if err != nil {
		return err
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if result.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("upstream is not serving")
	}
	return nil
}
