package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// GRPCTLSConfig enables mutual TLS when all three certificate paths are set.
// Empty paths select plaintext for a trusted, private development network.
type GRPCTLSConfig struct{ CAFile, CertFile, KeyFile, ServerName string }
type UpstreamClientConfig struct {
	Target  string
	Timeout time.Duration
	TLS     GRPCTLSConfig
}
type UpstreamServerConfig struct {
	Address          string
	DiscoveryTimeout time.Duration
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
	TLS              GRPCTLSConfig
}

func LoadUpstreamClientConfig() (UpstreamClientConfig, error) {
	return loadUpstreamClient(os.LookupEnv)
}
func loadUpstreamClient(lookup lookupEnvironment) (UpstreamClientConfig, error) {
	cfg := UpstreamClientConfig{Target: strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_TARGET", "localhost:50051"))}
	if err := validateGRPCAddress("UPSTREAM_GRPC_TARGET", cfg.Target, true); err != nil {
		return cfg, err
	}
	var err error
	if cfg.Timeout, err = positiveDuration(lookup, "UPSTREAM_GRPC_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	cfg.TLS, err = loadGRPCTLS(lookup)
	return cfg, err
}
func LoadUpstreamServerConfig() (UpstreamServerConfig, error) {
	return loadUpstreamServer(os.LookupEnv)
}
func loadUpstreamServer(lookup lookupEnvironment) (UpstreamServerConfig, error) {
	cfg := UpstreamServerConfig{Address: strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_ADDR", ":50051"))}
	if err := validateGRPCAddress("UPSTREAM_GRPC_ADDR", cfg.Address, false); err != nil {
		return cfg, err
	}
	var err error
	if cfg.DiscoveryTimeout, err = positiveDuration(lookup, "OIDC_DISCOVERY_TIMEOUT", defaultOIDCTimeout); err != nil {
		return cfg, err
	}
	if cfg.RequestTimeout, err = positiveDuration(lookup, "UPSTREAM_OAUTH_TIMEOUT", 8*time.Second); err != nil {
		return cfg, err
	}
	if cfg.ShutdownTimeout, err = positiveDuration(lookup, "UPSTREAM_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	cfg.TLS, err = loadGRPCTLS(lookup)
	return cfg, err
}
func loadGRPCTLS(lookup lookupEnvironment) (GRPCTLSConfig, error) {
	cfg := GRPCTLSConfig{
		CAFile:     strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_TLS_CA_FILE", "")),
		CertFile:   strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_TLS_CERT_FILE", "")),
		KeyFile:    strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_TLS_KEY_FILE", "")),
		ServerName: strings.TrimSpace(valueOrDefault(lookup, "UPSTREAM_GRPC_TLS_SERVER_NAME", "")),
	}
	if cfg == (GRPCTLSConfig{}) {
		return cfg, nil
	}
	if cfg.CAFile == "" || cfg.CertFile == "" || cfg.KeyFile == "" {
		return cfg, fmt.Errorf("UPSTREAM_GRPC_TLS_CA_FILE, UPSTREAM_GRPC_TLS_CERT_FILE and UPSTREAM_GRPC_TLS_KEY_FILE must be set together")
	}
	return cfg, nil
}
func validateGRPCAddress(key, address string, requireHost bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s must be a host:port address: %w", key, err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || (requireHost && host == "") {
		return fmt.Errorf("%s must contain a valid host and port (1-65535)", key)
	}
	return nil
}
