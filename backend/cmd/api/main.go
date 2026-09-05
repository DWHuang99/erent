package main

import (
	"fmt"
	"log/slog"
	"os"
)

const serviceName = "ai-gateway"

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	logger, logFile, err := newApplicationLogger()
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

	configuration, err := loadAPIConfiguration()
	if err != nil {
		slog.Error("load configuration", "error", err)
		return 1
	}
	instances, err := newApplicationInstances(configuration)
	if err != nil {
		slog.Error("initialize application instances", "error", err)
		return 1
	}
	defer func() {
		if err := instances.Close(); err != nil {
			slog.Error("close application instances", "error", err)
		}
	}()

	router := newRouter(configuration, instances, logger)
	slog.Info("http server starting", "address", configuration.runtime.HTTPAddress, "environment", configuration.runtime.Environment, "version", version)
	if err := router.Run(configuration.runtime.HTTPAddress); err != nil {
		slog.Error("run gin server", "error", err)
		return 1
	}
	return 0
}
