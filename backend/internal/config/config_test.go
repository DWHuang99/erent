package config

import (
	"strings"
	"testing"
	"time"
)

const testJWTSecret = "test-secret-with-at-least-32-characters"

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(func(key string) (string, bool) {
		if key == "JWT_SECRET" {
			return testJWTSecret, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if cfg.Environment != defaultEnvironment || cfg.HTTPAddress != defaultHTTPAddress {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	values := map[string]string{
		"APP_ENV":                  "test",
		"HTTP_ADDR":                "127.0.0.1:9090",
		"DATABASE_CONNECT_TIMEOUT": "4s",
		"OIDC_DISCOVERY_TIMEOUT":   "6s",
		"JWT_ACCESS_TTL":           "20m",
		"JWT_REFRESH_TTL":          "48h",
		"JWT_SECRET":               testJWTSecret,
		"JWT_ISSUER":               "test-issuer",
		"JWT_AUDIENCE":             "test-audience",
		"REDIS_ADDR":               "redis:6379",
		"REDIS_PASSWORD":           "redis-password",
		"REDIS_DB":                 "2",
		"COOKIE_SECURE":            "true",
		"BOOTSTRAP_ADMIN_USERNAME": "admin",
		"BOOTSTRAP_ADMIN_PASSWORD": "safe-password",
	}
	cfg, err := load(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("load overrides: %v", err)
	}
	if cfg.Environment != "test" || cfg.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.DatabaseConnectTimeout != 4*time.Second || cfg.OIDCDiscoveryTimeout != 6*time.Second || cfg.JWT.AccessTTL != 20*time.Minute {
		t.Fatalf("unexpected dependency timeouts: %+v", cfg)
	}
	if cfg.JWT.Secret != testJWTSecret || cfg.JWT.Issuer != "test-issuer" || cfg.JWT.Audience != "test-audience" || cfg.JWT.RefreshTTL != 48*time.Hour {
		t.Fatalf("unexpected JWT configuration: %+v", cfg.JWT)
	}
	if cfg.Redis.Address != "redis:6379" || cfg.Redis.Password != "redis-password" || cfg.Redis.DB != 2 || !cfg.CookieSecure {
		t.Fatalf("unexpected refresh configuration: %+v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"empty environment":    {"APP_ENV": ""},
		"invalid address":      {"HTTP_ADDR": "8080"},
		"invalid duration":     {"JWT_ACCESS_TTL": "soon"},
		"invalid OIDC timeout": {"OIDC_DISCOVERY_TIMEOUT": "0s"},
		"zero duration":        {"JWT_REFRESH_TTL": "0s"},
		"short jwt secret":     {"JWT_SECRET": "too-short"},
		"missing jwt secret":   {"JWT_SECRET": ""},
		"partial bootstrap":    {"BOOTSTRAP_ADMIN_USERNAME": "admin"},
		"invalid redis db":     {"REDIS_DB": "-1"},
		"invalid cookie":       {"COOKIE_SECURE": "sometimes"},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := load(func(key string) (string, bool) {
				value, ok := values[key]
				if !ok && key == "JWT_SECRET" {
					return testJWTSecret, true
				}
				return value, ok
			})
			if err == nil || strings.TrimSpace(err.Error()) == "" {
				t.Fatal("expected a descriptive error")
			}
		})
	}
}

func TestLoadOIDCConfigDisabled(t *testing.T) {
	cfg, err := loadOidcConfig("oai", func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("load disabled OIDC config: %v", err)
	}
	if cfg.Provider != "oai" || cfg.Enabled() {
		t.Fatalf("unexpected disabled OIDC config: %+v", cfg)
	}
}

func TestLoadOIDCConfigUsesProviderSpecificUppercaseKeys(t *testing.T) {
	values := map[string]string{
		"OAI_ISSUER":        " https://auth.example.com ",
		"OAI_CLIENT_ID":     " client-id ",
		"OAI_CLIENT_SECRET": " client-secret ",
		"OAI_REDIRECT_URL":  " http://localhost:8080/oai/callback ",
	}
	cfg, err := loadOidcConfig("oai", func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("load OIDC config: %v", err)
	}
	if !cfg.Enabled() || cfg.Provider != "oai" || cfg.Issuer != "https://auth.example.com" || cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" || cfg.RedirectURL != "http://localhost:8080/oai/callback" {
		t.Fatalf("unexpected OIDC config: %+v", cfg)
	}
}

func TestLoadOIDCConfigRejectsPartialAndInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"missing issuer": {
			"OAI_CLIENT_ID":    "client-id",
			"OAI_REDIRECT_URL": "http://localhost:8080/oai/callback",
		},
		"missing client id": {
			"OAI_ISSUER":       "https://auth.example.com",
			"OAI_REDIRECT_URL": "http://localhost:8080/oai/callback",
		},
		"missing redirect URL": {
			"OAI_ISSUER":    "https://auth.example.com",
			"OAI_CLIENT_ID": "client-id",
		},
		"invalid issuer": {
			"OAI_ISSUER":       "auth.example.com",
			"OAI_CLIENT_ID":    "client-id",
			"OAI_REDIRECT_URL": "http://localhost:8080/oai/callback",
		},
		"invalid redirect URL": {
			"OAI_ISSUER":       "https://auth.example.com",
			"OAI_CLIENT_ID":    "client-id",
			"OAI_REDIRECT_URL": "/oai/callback",
		},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadOidcConfig("oai", func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})
			if err == nil || strings.TrimSpace(err.Error()) == "" {
				t.Fatal("expected a descriptive error")
			}
		})
	}
}
