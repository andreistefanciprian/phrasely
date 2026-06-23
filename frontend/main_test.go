package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},        // unset → INFO
		{"VERBOSE", slog.LevelInfo}, // unrecognised → INFO
	}
	for _, tt := range tests {
		if got := parseLogLevel(tt.input); got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPrivacyPage(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	w := httptest.NewRecorder()

	app.privacyPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Privacy Policy",
		"Effective June 23, 2026",
		"OpenAI",
		"Data retention and deletion",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain %q", want)
		}
	}
}

func TestPrivacyPageUsesAuthenticatedNavWhenSignedIn(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "jwt"})
	w := httptest.NewRecorder()

	app.privacyPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="navbar"`,
		`href="/bubble"`,
		`id="account-menu"`,
		`href="/settings"`,
		`href="/story"`,
		`href="/privacy"`,
		`aria-label="Sign out"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain %q", want)
		}
	}
	if strings.Contains(body, "Sign in →") {
		t.Error("response contains logged-out sign-in prompt")
	}
}

func TestStoryPageUsesAuthenticatedNavWhenSignedIn(t *testing.T) {
	app := &application{homePhraseSamples: []homePhrase{{
		Phrase:  "a stitch in time",
		Keyword: "stitch",
		Source:  "test",
	}}}
	req := httptest.NewRequest(http.MethodGet, "/story", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "jwt"})
	w := httptest.NewRecorder()

	app.storyPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="navbar"`,
		`href="/bubble"`,
		"Open Phrasely →",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain %q", want)
		}
	}
	if strings.Contains(body, "Sign in →") || strings.Contains(body, "Try Phrasely, it's free →") {
		t.Error("response contains logged-out story prompt")
	}
}
