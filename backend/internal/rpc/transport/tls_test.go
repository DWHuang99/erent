package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"erent/internal/config"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func testIdentity(t *testing.T) config.GRPCTLSConfig {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "upstream.test"}, DNSNames: []string{"upstream.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	certFile, keyFile := filepath.Join(directory, "identity.pem"), filepath.Join(directory, "identity.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return config.GRPCTLSConfig{CAFile: certFile, CertFile: certFile, KeyFile: keyFile, ServerName: "upstream.test"}
}
func TestMutualTLSHandshake(t *testing.T) {
	cfg := testIdentity(t)
	serverCredentials, err := ServerCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(serverCredentials))
	healthpb.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	valid, err := ClientCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	noClientCertificate, err := loadTLS(cfg)
	if err != nil {
		t.Fatal(err)
	}
	noClientCertificate.ServerName = "upstream.test"
	noClientCertificate.Certificates = nil
	wrongNameCfg := cfg
	wrongNameCfg.ServerName = "different.test"
	wrongName, err := ClientCredentials(wrongNameCfg)
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := ClientCredentials(testIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		credentials credentials.TransportCredentials
		success     bool
	}{
		{"trusted client", valid, true},
		{"missing client identity", credentials.NewTLS(noClientCertificate), false},
		{"wrong server identity", wrongName, false},
		{"untrusted authority", untrusted, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(test.credentials))
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err = healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{})
			if (err == nil) != test.success {
				t.Fatalf("handshake success=%v, error=%v", test.success, err)
			}
		})
	}
}
func TestTLSRejectsInvalidFiles(t *testing.T) {
	if _, err := ClientCredentials(config.GRPCTLSConfig{CAFile: "missing"}); err == nil {
		t.Fatal("missing CA accepted")
	}
	file := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(file, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ServerCredentials(config.GRPCTLSConfig{CAFile: file}); err == nil {
		t.Fatal("invalid CA accepted")
	}
	cfg := testIdentity(t)
	cfg.KeyFile = file
	if _, err := ClientCredentials(cfg); err == nil {
		t.Fatal("invalid identity accepted")
	}
}
