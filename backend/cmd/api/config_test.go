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

func TestLoadConfigReadsAudioConfiguration(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/phrasely")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ELEVENLABS_API_KEY", "eleven-key")
	t.Setenv("ELEVENLABS_VOICE_ID", "voice-id")
	t.Setenv("ELEVENLABS_MODEL_ID", "model-id")
	t.Setenv("R2_ENDPOINT", "https://account.r2.cloudflarestorage.com")
	t.Setenv("R2_BUCKET", "audio")
	t.Setenv("R2_ACCESS_KEY_ID", "r2-key")
	t.Setenv("R2_SECRET_ACCESS_KEY", "r2-secret")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.elevenLabsAPIKey != "eleven-key" || cfg.elevenLabsVoiceID != "voice-id" || cfg.elevenLabsModelID != "model-id" {
		t.Fatalf("ElevenLabs config not loaded: %+v", cfg)
	}
	if cfg.r2Endpoint == "" || cfg.r2Bucket != "audio" || cfg.r2AccessKeyID != "r2-key" || cfg.r2SecretAccessKey != "r2-secret" {
		t.Fatalf("R2 config not loaded: %+v", cfg)
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
		"ELEVENLABS_API_KEY",
		"ELEVENLABS_VOICE_ID",
		"ELEVENLABS_MODEL_ID",
		"R2_ENDPOINT",
		"R2_BUCKET",
		"R2_ACCESS_KEY_ID",
		"R2_SECRET_ACCESS_KEY",
	} {
		t.Setenv(key, "")
	}
}
