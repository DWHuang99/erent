// Command gateway is an optional edge reverse proxy for the monolithic API.
package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type gatewayConfig struct {
	address         string
	upstreamURL     string
	dialTimeout     time.Duration
	responseTimeout time.Duration
}

func main() {
	configuration, err := loadGatewayConfig(os.LookupEnv)
	if err != nil {
		slog.Error("load gateway configuration", "error", err)
		os.Exit(1)
	}
	proxy, err := newProxy(configuration)
	if err != nil {
		slog.Error("create reverse proxy", "error", err)
		os.Exit(1)
	}

	router := newGatewayRouter(proxy)
	slog.Info("gateway starting", "address", configuration.address)
	if err := router.Run(configuration.address); err != nil {
		slog.Error("run gin gateway", "error", err)
		os.Exit(1)
	}
}

func newGatewayRouter(upstream http.Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	router.NoRoute(func(c *gin.Context) {
		upstream.ServeHTTP(c.Writer, c.Request)
	})
	return router
}

func newProxy(configuration gatewayConfig) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(configuration.upstreamURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("GATEWAY_UPSTREAM_URL must be an absolute HTTP(S) URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: configuration.dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = configuration.responseTimeout
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		slog.Error("gateway upstream request failed", "error", proxyErr)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":50200,"data":null,"message":"upstream unavailable"}`))
	}
	return proxy, nil
}

type lookupEnvironment func(string) (string, bool)

func loadGatewayConfig(lookup lookupEnvironment) (gatewayConfig, error) {
	value := func(key, fallback string) string {
		if result, ok := lookup(key); ok {
			return strings.TrimSpace(result)
		}
		return fallback
	}
	parseDuration := func(key, fallback string) (time.Duration, error) {
		result, err := time.ParseDuration(value(key, fallback))
		if err != nil || result <= 0 {
			return 0, fmt.Errorf("%s must be a positive duration", key)
		}
		return result, nil
	}

	configuration := gatewayConfig{
		address:     value("GATEWAY_ADDR", ":8080"),
		upstreamURL: value("GATEWAY_UPSTREAM_URL", "http://127.0.0.1:8081"),
	}
	var err error
	if configuration.dialTimeout, err = parseDuration("GATEWAY_DIAL_TIMEOUT", "2s"); err != nil {
		return gatewayConfig{}, err
	}
	if configuration.responseTimeout, err = parseDuration("GATEWAY_RESPONSE_HEADER_TIMEOUT", "30s"); err != nil {
		return gatewayConfig{}, err
	}
	if _, _, err := net.SplitHostPort(configuration.address); err != nil {
		return gatewayConfig{}, fmt.Errorf("parse GATEWAY_ADDR: %w", err)
	}
	return configuration, nil
}
