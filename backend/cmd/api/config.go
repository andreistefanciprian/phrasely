package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// config contains every environment-derived setting used by the API. Loading
// these values once keeps application construction independent from os.Getenv.
type config struct {
	databaseURL string
	port        string
	baseURL     string
	jwtSecret   string

	magicLinkTTL        time.Duration
	jwtTTL              time.Duration
	oauthAccessTokenTTL time.Duration
	relatedMaxDistance  float64

	resendAPIKey string
	emailFrom    string
	openAIAPIKey string
}

func loadConfig() (config, error) {
	// BASE_URL is the frontend origin — magic links point here so the browser
	// lands on the frontend /auth-verify page, not the API's /auth/verify endpoint.
	cfg := config{
		databaseURL:         os.Getenv("DATABASE_URL"),
		port:                stringEnv("PORT", "8080"),
		baseURL:             stringEnv("BASE_URL", "http://localhost:3000"),
		jwtSecret:           os.Getenv("JWT_SECRET"),
		magicLinkTTL:        durationEnv("MAGIC_LINK_TTL", 15*time.Minute),
		jwtTTL:              durationEnv("JWT_TTL", 30*24*time.Hour),
		oauthAccessTokenTTL: durationEnv("OAUTH_ACCESS_TOKEN_TTL", time.Hour),
		relatedMaxDistance:  floatEnv("RELATED_MAX_DISTANCE", 0.45),
		resendAPIKey:        os.Getenv("RESEND_API_KEY"),
		emailFrom:           os.Getenv("EMAIL_FROM"),
		openAIAPIKey:        os.Getenv("OPENAI_API_KEY"),
	}

	if cfg.databaseURL == "" {
		return config{}, fmt.Errorf("DATABASE_URL is not set")
	}
	if cfg.jwtSecret == "" {
		return config{}, fmt.Errorf("JWT_SECRET is not set")
	}

	return cfg, nil
}

func stringEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

// parseLogLevel maps a LOG_LEVEL string to a slog.Level.
// Defaults to INFO for empty or unrecognised values.
func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// floatEnv reads a float64 from the named env var, falling back to def if unset or invalid.
func floatEnv(key string, def float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		slog.Warn("invalid float env var, using default", "key", key, "value", val, "default", def)
		return def
	}
	return f
}

// durationEnv reads a duration from the named env var (e.g. "15m", "720h"),
// falling back to def if unset or invalid.
func durationEnv(key string, def time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("invalid duration env var, using default", "key", key, "value", val, "default", def)
		return def
	}
	return d
}
