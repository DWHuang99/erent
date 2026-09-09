package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"erent/internal/config"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func ClientCredentials(cfg config.GRPCTLSConfig) (credentials.TransportCredentials, error) {
	if cfg == (config.GRPCTLSConfig{}) {
		return insecure.NewCredentials(), nil
	}
	tlsConfig, err := loadTLS(cfg)
	if err != nil {
		return nil, err
	}
	tlsConfig.ServerName = cfg.ServerName
	return credentials.NewTLS(tlsConfig), nil
}

// ServerCredentials requires a client certificate signed by the configured CA.
func ServerCredentials(cfg config.GRPCTLSConfig) (credentials.TransportCredentials, error) {
	if cfg == (config.GRPCTLSConfig{}) {
		return nil, nil
	}
	tlsConfig, err := loadTLS(cfg)
	if err != nil {
		return nil, err
	}
	tlsConfig.ClientCAs = tlsConfig.RootCAs
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	return credentials.NewTLS(tlsConfig), nil
}
func loadTLS(cfg config.GRPCTLSConfig) (*tls.Config, error) {
	ca, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read gRPC CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("gRPC CA file contains no certificates")
	}
	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load gRPC certificate: %w", err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, Certificates: []tls.Certificate{certificate}}, nil
}
