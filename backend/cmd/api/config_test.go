package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/phrasely")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if cfg.port != "8080" {
		t.Errorf("port = %q, want %q", cfg.port, "8080")
	}
	if cfg.baseURL != "http://localhost:3000" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://localhost:3000")
	}
	if cfg.magicLinkTTL != 15*time.Minute {
		t.Errorf("magicLinkTTL = %v, want %v", cfg.magicLinkTTL, 15*time.Minute)
	}
	if cfg.jwtTTL != 30*24*time.Hour {
		t.Errorf("jwtTTL = %v, want %v", cfg.jwtTTL, 30*24*time.Hour)
	}
	if cfg.oauthAccessTokenTTL != time.Hour {
		t.Errorf("oauthAccessTokenTTL = %v, want %v", cfg.oauthAccessTokenTTL, time.Hour)
	}
	if cfg.relatedMaxDistance != 0.45 {
		t.Errorf("relatedMaxDistance = %v, want %v", cfg.relatedMaxDistance, 0.45)
	}
}

func TestLoadConfigRequiresDatabaseURLAndJWTSecret(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		jwtSecret   string
		wantError   string
	}{
		{
			name:      "missing database URL",
			jwtSecret: "test-secret",
			wantError: "DATABASE_URL is not set",
		},
		{
			name:        "missing JWT secret",
			databaseURL: "postgres://localhost/phrasely",
			wantError:   "JWT_SECRET is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("JWT_SECRET", tt.jwtSecret)

			_, err := loadConfig()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("loadConfig() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"DATABASE_URL",
		"PORT",
		"BASE_URL",
		"JWT_SECRET",
		"MAGIC_LINK_TTL",
		"JWT_TTL",
		"OAUTH_ACCESS_TOKEN_TTL",
		"RELATED_MAX_DISTANCE",
		"RESEND_API_KEY",
		"EMAIL_FROM",
		"OPENAI_API_KEY",
	} {
		t.Setenv(key, "")
	}
}
