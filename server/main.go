package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"rbf-api/applog"
	"rbf-api/handler"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger := applog.New()
	cfg := handler.ConfigFromEnv()
	addr := ":" + port

	server := &http.Server{
		Addr:              addr,
		Handler:           handler.NewRouter(logger, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info(
		applog.CategorySystem,
		"server started",
		slog.String("addr", addr),
		slog.Int("install_limit_per_min", cfg.InstallRateLimitPerMinute),
		slog.Int("install_burst", cfg.InstallRateLimitBurst),
		slog.Int("index_limit_per_min", cfg.IndexRateLimitPerMinute),
		slog.Int("index_burst", cfg.IndexRateLimitBurst),
	)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error(
			applog.CategorySystem,
			"server stopped unexpectedly",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
}
