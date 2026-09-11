package main

import "testing"

func TestUpstreamConfigurationLoadsWithoutOAI(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	for _, key := range []string{"OAI_ISSUER", "OAI_CLIENT_ID", "OAI_CLIENT_SECRET", "OAI_REDIRECT_URL", "UPSTREAM_GRPC_TLS_CA_FILE", "UPSTREAM_GRPC_TLS_CERT_FILE", "UPSTREAM_GRPC_TLS_KEY_FILE", "UPSTREAM_GRPC_TLS_SERVER_NAME"} {
		t.Setenv(key, "")
	}
	t.Setenv("UPSTREAM_GRPC_TARGET", "localhost:50051")
	t.Setenv("UPSTREAM_GRPC_TIMEOUT", "10s")
	cfg, err := loadAPIConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.oai.Enabled() || cfg.upstream.Target != "localhost:50051" || cfg.upstream.Timeout <= 0 {
		t.Fatalf("upstream configuration not loaded: %+v", cfg.upstream)
	}
}
