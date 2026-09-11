package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		"Effective September 1, 2026",
		"OpenAI",
		"Data retention and deletion",
		"Waiting list signups",
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
		`href="/terms"`,
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

func TestTermsPage(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	w := httptest.NewRecorder()

	app.termsPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Terms of Service",
		"Effective August 9, 2026",
		"AI-powered features",
		"Your content",
		"Consumer rights",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain %q", want)
		}
	}
}

func TestTermsPageUsesAuthenticatedNavWhenSignedIn(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "jwt"})
	w := httptest.NewRecorder()

	app.termsPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="navbar"`,
		`href="/bubble"`,
		`id="account-menu"`,
		`href="/privacy"`,
		`href="/terms"`,
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

func TestHomeShufflePhraseSet(t *testing.T) {
	data, err := files.ReadFile("home_shuffle_phrases.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples []homeShufflePhrase
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 30 {
		t.Fatalf("shuffle phrase count = %d, want 30", len(samples))
	}

	featured := map[string]bool{
		"a hunch":          true,
		"put my finger on": true,
		"forthcoming":      true,
		"find your groove": true,
		"brush off":        true,
		"hinge on":         true,
		"a whisker away":   true,
		"get cracking":     true,
		"taper off":        true,
		"muddy the waters": true,
	}
	for _, sample := range samples {
		if sample.Keyword == "" || sample.Definition == "" || sample.Phrase == "" {
			t.Errorf("incomplete shuffle sample: %+v", sample)
		}
		wantWeight := 1
		if featured[sample.Keyword] {
			wantWeight = 4
			delete(featured, sample.Keyword)
		}
		if sample.Weight != wantWeight {
			t.Errorf("weight for %q = %d, want %d", sample.Keyword, sample.Weight, wantWeight)
		}
	}
	if len(featured) != 0 {
		t.Errorf("featured phrases missing from shuffle data: %v", featured)
	}
}

func TestBubbleAndShufflePagesShareEmptyState(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/phrases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer backend.Close()

	app := &application{api: newAPIClient(backend.URL)}
	want := `No phrases yet. <a href="/add">Add your first →</a>`

	for _, tt := range []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{name: "bubble", path: "/bubble", handler: app.bubblePage},
		{name: "shuffle", path: "/shuffle", handler: app.shufflePage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("response does not contain shared empty state %q", want)
			}
		})
	}
}

func TestShufflePageKeepsRelatedOutsideStableStage(t *testing.T) {
	template, err := files.ReadFile("templates/shuffle.html")
	if err != nil {
		t.Fatalf("read shuffle template: %v", err)
	}

	source := string(template)
	if !strings.Contains(source, "  </div>\n</div>\n<div id=\"related\"></div>") {
		t.Fatal("related section must be outside the stable shuffle stage")
	}
	if !strings.Contains(source, "body:has(#shuffle-stage) { justify-content: flex-start; }") {
		t.Fatal("shuffle page must not vertically centre the stage together with related results")
	}
	if !strings.Contains(source, "#shuffle-stage { display: grid; justify-items: center;") {
		t.Fatal("shuffle stage must independently centre the main phrase horizontally")
	}
	if strings.Contains(source, "place-items: center") || strings.Contains(source, "min-height: min(55dvh, 32rem)") ||
		strings.Contains(source, "min-height: 50dvh") {
		t.Fatal("shuffle stage must not reserve variable space below the main phrase")
	}
	if !strings.Contains(source, "padding-top: min(15dvh, 7rem)") {
		t.Fatal("shuffle stage must anchor the main phrase with a stable top offset")
	}
}

func TestAuthorizePreservesOAuthResource(t *testing.T) {
	const resource = "https://mcp.example.com"
	const redirectURI = "https://chatgpt.com/callback"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/internal/oauth/clients/"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"redirect_uris": []string{redirectURI}})
		case r.Method == http.MethodPost && r.URL.Path == "/internal/oauth/authorize":
			var body struct {
				Resource string `json:"resource"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode authorize body: %v", err)
			}
			if body.Resource != resource {
				t.Errorf("resource = %q, want %q", body.Resource, resource)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"code": "code-123"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	app := &application{api: newAPIClient(backend.URL)}
	params := url.Values{
		"client_id":             {"client-123"},
		"resource":              {resource},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"state-123"},
		"response_type":         {"code"},
	}

	t.Run("GET keeps resource in consent forms", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/authorize?"+params.Encode(), nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: "jwt"})
		w := httptest.NewRecorder()
		app.authorizePage(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
		}
		if got := strings.Count(w.Body.String(), `name="resource"`); got != 2 {
			t.Errorf("resource hidden field count = %d, want 2", got)
		}
		if got := strings.Count(w.Body.String(), `value="`+resource+`"`); got != 2 {
			t.Errorf("resource hidden value count = %d, want 2", got)
		}
	})

	t.Run("POST forwards resource to backend", func(t *testing.T) {
		params.Set("action", "allow")
		req := httptest.NewRequest(http.MethodPost, "/authorize", strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: "jwt"})
		w := httptest.NewRecorder()
		app.authorizePage(w, req)

		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body = %s", w.Code, w.Body.String())
		}
		location, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}
		if location.Query().Get("code") != "code-123" || location.Query().Get("state") != "state-123" {
			t.Errorf("unexpected redirect query: %s", location.RawQuery)
		}
	})
}

func TestValidateOAuthParamsRequiresResource(t *testing.T) {
	p := oauthParams{
		ClientID:            "client-123",
		RedirectURI:         "https://chatgpt.com/callback",
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		State:               "state-123",
		ResponseType:        "code",
	}
	if err := validateOAuthParams(p); err == nil {
		t.Fatal("missing resource was accepted")
	}
	p.Resource = "not-a-url"
	if err := validateOAuthParams(p); err == nil {
		t.Fatal("invalid resource was accepted")
	}
	p.Resource = "https://mcp.example.com#fragment"
	if err := validateOAuthParams(p); err == nil {
		t.Fatal("resource with fragment was accepted")
	}
	p.Resource = "https://mcp.example.com"
	if err := validateOAuthParams(p); err != nil {
		t.Fatalf("valid resource was rejected: %v", err)
	}
}

func TestStructuredCollectionPagesRender(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": "sample", "phrase": "She stood up to scrutiny (even then).", "headwords": []map[string]string{{"text": "stood up to scrutiny", "canonical": "stand up to scrutiny", "meaning": "remained convincing"}}, "note": "Context"}})
	}))
	defer backend.Close()
	app := &application{api: newAPIClient(backend.URL)}
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"bubble", app.bubblePage}, {"shuffle", app.shufflePage}, {"phrases", app.phrasesPage}, {"add", app.addPage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest(http.MethodGet, "/"+tc.name, nil))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `src="/static/headwords.js"`) || strings.Contains(w.Body.String(), "ZgotmplZ") {
				t.Fatalf("render failed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAPIProxyStreamsPhraseAudioResponse(t *testing.T) {
	wantAudio := []byte{0xff, 0xfb, 0x90, 0x64, 0x00, 0x7f}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RequestURI() != "/api/v1/phrases/phrase-123/audio?fresh=1" {
			t.Errorf("upstream request = %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-123" {
			t.Errorf("Authorization = %q, want Bearer jwt-123", got)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("Content-Length", "6")
		w.WriteHeader(http.StatusOK)
		w.Write(wantAudio)
	}))
	defer backend.Close()

	app := &application{api: newAPIClient(backend.URL)}
	req := httptest.NewRequest(http.MethodGet, "/fd/phrases/phrase-123/audio?fresh=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyJWT, "jwt-123"))
	w := httptest.NewRecorder()

	app.apiProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.Bytes(); !bytes.Equal(got, wantAudio) {
		t.Errorf("body = %v, want %v", got, wantAudio)
	}
	for header, want := range map[string]string{
		"Content-Type": "audio/mpeg", "Content-Length": "6", "Cache-Control": "private, no-cache",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestAPIProxyPreservesPhraseAudioError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, `{"error":"listen unavailable"}`)
	}))
	defer backend.Close()

	app := &application{api: newAPIClient(backend.URL)}
	req := httptest.NewRequest(http.MethodGet, "/fd/phrases/phrase-123/audio", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyJWT, "jwt-123"))
	w := httptest.NewRecorder()

	app.apiProxy(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Body.String(); got != `{"error":"listen unavailable"}` {
		t.Errorf("body = %q", got)
	}
}

func TestNonAudioAPIProxyRemainsJSON(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, `{"error":"still relayed"}`)
	}))
	defer backend.Close()

	app := &application{api: newAPIClient(backend.URL)}
	req := httptest.NewRequest(http.MethodGet, "/fd/phrases", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyJWT, "jwt-123"))
	w := httptest.NewRecorder()

	app.apiProxy(w, req)

	if w.Code != http.StatusTeapot || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
