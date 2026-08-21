package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env when running locally. In production, env vars are injected by the platform.
	_ = godotenv.Load()

	// Configure structured JSON logging first so every subsequent log line
	// is parseable by Railway's log viewer. Level defaults to INFO.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	}))
	slog.SetDefault(logger)

	// Railway sends SIGTERM when a deployment is stopped. Converting process signals
	// into context cancellation gives the HTTP server time to finish in-flight requests.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Keep os.Exit in main so deferred cleanup inside run always completes first.
	if err := run(ctx); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}
