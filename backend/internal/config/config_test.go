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
		"JWT_ACCESS_TTL":           "20m",
		"JWT_REFRESH_TTL":          "48h",
		"JWT_SECRET":               testJWTSecret,
		"REDIS_ADDR":               "redis:6379",
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
	if cfg.DatabaseConnectTimeout != 4*time.Second || cfg.JWTAccessTTL != 20*time.Minute {
		t.Fatalf("unexpected dependency timeouts: %+v", cfg)
	}
	if cfg.JWTRefreshTTL != 48*time.Hour || cfg.RedisAddress != "redis:6379" || cfg.RedisDB != 2 || !cfg.CookieSecure {
		t.Fatalf("unexpected refresh configuration: %+v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"empty environment":  {"APP_ENV": ""},
		"invalid address":    {"HTTP_ADDR": "8080"},
		"invalid duration":   {"JWT_ACCESS_TTL": "soon"},
		"zero duration":      {"JWT_REFRESH_TTL": "0s"},
		"short jwt secret":   {"JWT_SECRET": "too-short"},
		"missing jwt secret": {"JWT_SECRET": ""},
		"partial bootstrap":  {"BOOTSTRAP_ADMIN_USERNAME": "admin"},
		"invalid redis db":   {"REDIS_DB": "-1"},
		"invalid cookie":     {"COOKIE_SECURE": "sometimes"},
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
