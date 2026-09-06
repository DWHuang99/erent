package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"erent/internal/config"
	applogger "erent/internal/logger"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/modules/oauth/openai"
	"erent/internal/upstreamserver"
)

func main() { os.Exit(run()) }
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger, logFile, err := applogger.NewLogger(os.Getenv("LOG_FILE"))
	if err != nil {
		slog.Error("initialize logger", "error", err)
		return 1
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close log file: %v\n", err)
		}
	}()
	slog.SetDefault(logger)
	cfg, err := config.LoadUpstreamServerConfig()
	if err != nil {
		slog.Error("load upstream configuration", "error", err)
		return 1
	}
	oaiConfig, err := config.LoadOIDCConfig("oai")
	if err != nil {
		slog.Error("load OAI configuration", "error", err)
		return 1
	}
	providers := make(map[string]*oidc.OIDCAuth)
	if oaiConfig.Enabled() {
		discoveryCtx, cancelDiscovery := context.WithTimeout(ctx, cfg.DiscoveryTimeout)
		auth, err := oidc.NewOIDCAuth(discoveryCtx, oaiConfig, openai.OaiAuthURLParams(), openai.OaiScopes())
		cancelDiscovery()
		if err != nil {
			slog.Error("initialize OAI discovery", "error", err)
			return 1
		}
		providers["oai"] = auth
	}
	if err := upstreamserver.Serve(ctx, cfg, providers); err != nil {
		slog.Error("run upstream gRPC service", "error", err)
		return 1
	}
	return 0
}
