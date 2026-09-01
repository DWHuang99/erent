// Package config loads and validates process configuration from environment variables.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvironment     = "development"
	defaultHTTPAddress     = ":8080"
	defaultDatabaseURL     = "postgres://ai_gateway:ai_gateway_dev@localhost:5432/ai_gateway?sslmode=disable"
	defaultJWTIssuer       = "ai-gateway"
	defaultJWTAudience     = "ai-gateway-api"
	defaultJWTAccessTTL    = 15 * time.Minute
	defaultJWTRefreshTTL   = 7 * 24 * time.Hour
	defaultRedisAddress    = "localhost:6379"
	defaultDatabaseTimeout = 10 * time.Second
)

// Config contains the runtime settings required by the current server scaffold.
type Config struct {
	Environment            string
	HTTPAddress            string
	DatabaseURL            string
	DatabaseConnectTimeout time.Duration
	JWTSecret              string
	JWTIssuer              string
	JWTAudience            string
	JWTAccessTTL           time.Duration
	JWTRefreshTTL          time.Duration
	RedisAddress           string
	RedisPassword          string
	RedisDB                int
	CookieSecure           bool
	BootstrapUsername      string
	BootstrapPassword      string
	BootstrapRole          string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

type lookupEnvironment func(string) (string, bool)

func load(lookup lookupEnvironment) (Config, error) {
	cfg := Config{
		Environment:       strings.TrimSpace(valueOrDefault(lookup, "APP_ENV", defaultEnvironment)),
		HTTPAddress:       strings.TrimSpace(valueOrDefault(lookup, "HTTP_ADDR", defaultHTTPAddress)),
		DatabaseURL:       strings.TrimSpace(valueOrDefault(lookup, "DATABASE_URL", defaultDatabaseURL)),
		JWTSecret:         valueOrDefault(lookup, "JWT_SECRET", ""),
		JWTIssuer:         strings.TrimSpace(valueOrDefault(lookup, "JWT_ISSUER", defaultJWTIssuer)),
		JWTAudience:       strings.TrimSpace(valueOrDefault(lookup, "JWT_AUDIENCE", defaultJWTAudience)),
		RedisAddress:      strings.TrimSpace(valueOrDefault(lookup, "REDIS_ADDR", defaultRedisAddress)),
		RedisPassword:     valueOrDefault(lookup, "REDIS_PASSWORD", ""),
		BootstrapUsername: strings.TrimSpace(valueOrDefault(lookup, "BOOTSTRAP_ADMIN_USERNAME", "")),
		BootstrapPassword: valueOrDefault(lookup, "BOOTSTRAP_ADMIN_PASSWORD", ""),
		BootstrapRole:     strings.TrimSpace(valueOrDefault(lookup, "BOOTSTRAP_ADMIN_ROLE", "admin")),
	}

	var err error
	if cfg.DatabaseConnectTimeout, err = positiveDuration(lookup, "DATABASE_CONNECT_TIMEOUT", defaultDatabaseTimeout); err != nil {
		return Config{}, err
	}
	if cfg.JWTAccessTTL, err = positiveDuration(lookup, "JWT_ACCESS_TTL", defaultJWTAccessTTL); err != nil {
		return Config{}, err
	}
	if cfg.JWTRefreshTTL, err = positiveDuration(lookup, "JWT_REFRESH_TTL", defaultJWTRefreshTTL); err != nil {
		return Config{}, err
	}
	if cfg.RedisDB, err = nonNegativeInt(lookup, "REDIS_DB", 0); err != nil {
		return Config{}, err
	}
	if cfg.CookieSecure, err = booleanValue(lookup, "COOKIE_SECURE", false); err != nil {
		return Config{}, err
	}

	if cfg.Environment == "" {
		return Config{}, fmt.Errorf("APP_ENV must not be empty")
	}
	if err := validateAddress(cfg.HTTPAddress); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must not be empty")
	}
	if cfg.RedisAddress == "" {
		return Config{}, fmt.Errorf("REDIS_ADDR must not be empty")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.JWTIssuer == "" || cfg.JWTAudience == "" {
		return Config{}, fmt.Errorf("JWT_ISSUER and JWT_AUDIENCE must not be empty")
	}
	if (cfg.BootstrapUsername == "") != (cfg.BootstrapPassword == "") {
		return Config{}, fmt.Errorf("BOOTSTRAP_ADMIN_USERNAME and BOOTSTRAP_ADMIN_PASSWORD must be set together")
	}
	if cfg.BootstrapPassword != "" && len(cfg.BootstrapPassword) < 8 {
		return Config{}, fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must contain at least 8 characters")
	}
	if cfg.BootstrapRole == "" {
		return Config{}, fmt.Errorf("BOOTSTRAP_ADMIN_ROLE must not be empty")
	}

	return cfg, nil
}

func nonNegativeInt(lookup lookupEnvironment, key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return value, nil
}

func booleanValue(lookup lookupEnvironment, key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func valueOrDefault(lookup lookupEnvironment, key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	return value
}

func positiveDuration(lookup lookupEnvironment, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return value, nil
}

func validateAddress(address string) error {
	if address == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		return fmt.Errorf("parse HTTP_ADDR: %w", err)
	}
	return nil
}
