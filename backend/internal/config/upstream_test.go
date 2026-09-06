package config

import (
	"testing"
	"time"
)

func lookupValues(values map[string]string) lookupEnvironment {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}
func TestUpstreamConfiguration(t *testing.T) {
	client, err := loadUpstreamClient(lookupValues(nil))
	if err != nil || client.Target != "localhost:50051" || client.Timeout != 10*time.Second {
		t.Fatalf("client defaults: %+v %v", client, err)
	}
	server, err := loadUpstreamServer(lookupValues(nil))
	if err != nil || server.Address != ":50051" || server.RequestTimeout != 8*time.Second || server.DiscoveryTimeout != 10*time.Second {
		t.Fatalf("server defaults: %+v %v", server, err)
	}
	client, err = loadUpstreamClient(lookupValues(map[string]string{"UPSTREAM_GRPC_TARGET": "upstream:5443", "UPSTREAM_GRPC_TIMEOUT": "3s", "UPSTREAM_GRPC_TLS_CA_FILE": "ca", "UPSTREAM_GRPC_TLS_CERT_FILE": "cert", "UPSTREAM_GRPC_TLS_KEY_FILE": "key", "UPSTREAM_GRPC_TLS_SERVER_NAME": "upstream"}))
	if err != nil || client.Timeout != 3*time.Second || client.TLS.ServerName != "upstream" {
		t.Fatalf("overrides: %+v %v", client, err)
	}
}
func TestUpstreamConfigurationRejectsInvalidValues(t *testing.T) {
	for key, value := range map[string]string{"UPSTREAM_GRPC_TARGET": ":50051", "UPSTREAM_GRPC_TIMEOUT": "0s", "UPSTREAM_GRPC_TLS_CERT_FILE": "only-cert", "UPSTREAM_GRPC_TLS_SERVER_NAME": "only-name"} {
		t.Run(key, func(t *testing.T) {
			if _, err := loadUpstreamClient(lookupValues(map[string]string{key: value})); err == nil {
				t.Fatal("invalid client configuration accepted")
			}
		})
	}
	for key, value := range map[string]string{"UPSTREAM_GRPC_ADDR": ":70000", "UPSTREAM_OAUTH_TIMEOUT": "-1s", "UPSTREAM_SHUTDOWN_TIMEOUT": "later", "OIDC_DISCOVERY_TIMEOUT": "0s", "UPSTREAM_GRPC_TLS_KEY_FILE": "only-key"} {
		t.Run(key, func(t *testing.T) {
			if _, err := loadUpstreamServer(lookupValues(map[string]string{key: value})); err == nil {
				t.Fatal("invalid server configuration accepted")
			}
		})
	}
}
