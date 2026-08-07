package main

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type homePhrase struct {
	Phrase  string `json:"phrase"`
	Keyword string `json:"keyword"`
	Source  string `json:"source"`
}

const authCookieName = "auth_token"
const authCookieTTL = 30 * 24 * time.Hour

type ctxKey string

const ctxKeyJWT ctxKey = "jwt"

func jwtFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyJWT).(string)
	return v
}

func hasAuthCookie(r *http.Request) bool {
	cookie, err := r.Cookie(authCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

// isSafeLocalRedirect returns true when next is a safe same-origin path.
// It must start with "/" but not "//" (which would be protocol-relative and
// could redirect to an attacker-controlled host).
func isSafeLocalRedirect(next string) bool {
	return strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//")
}

// requireAuth wraps a handler and redirects unauthenticated requests to /login.
func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil {
			slog.Debug("requireAuth: no auth cookie, redirecting to login", "path", r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		jwt := strings.TrimSpace(cookie.Value)
		if jwt == "" {
			slog.Debug("requireAuth: empty auth cookie, redirecting to login", "path", r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyJWT, jwt)
		next(w, r.WithContext(ctx))
	}
}

// ── Page handlers ─────────────────────────────────────────────────────────────

func (app *application) notFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	app.render(w, "404.html", nil)
}

func (app *application) homePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w, r)
		return
	}
	// Already signed in → go to bubble
	if cookie, err := r.Cookie(authCookieName); err == nil {
		if strings.TrimSpace(cookie.Value) != "" {
			http.Redirect(w, r, "/bubble", http.StatusSeeOther)
			return
		}
	}
	sample := app.homePhraseSamples[rand.IntN(len(app.homePhraseSamples))]
	app.render(w, "home.html", map[string]any{
		"Quote":     sample.Phrase,
		"QuoteMeta": sample.Keyword + " · captured from " + sample.Source,
	})
}

func (app *application) loginPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Preserve the post-login destination across the magic-link round-trip.
		// The authorize handler sets ?next=/authorize?... when it redirects here.
		// We store it in a short-lived cookie so it survives the email detour.
		next := r.URL.Query().Get("next")
		secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		if isSafeLocalRedirect(next) {
			http.SetCookie(w, &http.Cookie{
				Name:     "oauth_next",
				Value:    next,
				Path:     "/",
				HttpOnly: true,
				Secure:   secure,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   900, // 15 minutes — enough time to click the email link
			})
		} else {
			// Clear any stale oauth_next from a previous OAuth flow so a regular
			// login doesn't unexpectedly redirect the user to /authorize.
			http.SetCookie(w, &http.Cookie{
				Name:     "oauth_next",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				Secure:   secure,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   -1,
				Expires:  time.Unix(0, 0),
			})
		}
		app.render(w, "login.html", map[string]any{"Sent": false})

	case http.MethodPost:
		// Tighten the global limit: login only needs an email address
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if err := r.ParseForm(); err != nil {
			app.render(w, "login.html", map[string]any{"Error": "Invalid request."})
			return
		}
		email := strings.TrimSpace(r.PostForm.Get("email"))
		if email == "" {
			app.render(w, "login.html", map[string]any{"Error": "Email is required."})
			return
		}
		if err := app.api.RequestMagicLink(email); err != nil {
			slog.Error("request magic link", "error", err)
			app.render(w, "login.html", map[string]any{"Error": "Something went wrong. Try again."})
			return
		}
		app.render(w, "login.html", map[string]any{"Sent": true, "Email": email})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *application) authVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	jwt, err := app.api.VerifyToken(token)
	if err != nil {
		slog.Warn("verify token failed", "error", err)
		app.render(w, "login.html", map[string]any{"Error": "This link is invalid or has expired."})
		return
	}

	// Secure=true only over HTTPS — false on plain HTTP localhost
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    jwt,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(authCookieTTL.Seconds()),
	})

	// If the user arrived here via an OAuth flow, send them back to the consent
	// screen rather than the home page. The oauth_next cookie is set by loginPage
	// when /authorize redirects an unauthenticated user to /login?next=...
	redirect := "/bubble"
	if c, err := r.Cookie("oauth_next"); err == nil && isSafeLocalRedirect(c.Value) {
		redirect = c.Value
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_next",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
	}

	slog.Info("user authenticated via magic link")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (app *application) signOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	slog.Info("user signed out")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) storyPage(w http.ResponseWriter, r *http.Request) {
	sample := app.homePhraseSamples[rand.IntN(len(app.homePhraseSamples))]
	authenticated := hasAuthCookie(r)
	data := map[string]any{
		"Authenticated": authenticated,
		"Quote":         sample.Phrase,
		"QuoteMeta":     sample.Keyword + " · captured from " + sample.Source,
	}
	if authenticated {
		app.renderAuth(w, "story.html", data)
		return
	}
	app.render(w, "story.html", data)
}

func (app *application) privacyPage(w http.ResponseWriter, r *http.Request) {
	authenticated := hasAuthCookie(r)
	data := map[string]any{"Authenticated": authenticated}
	if authenticated {
		app.renderAuth(w, "privacy.html", data)
		return
	}
	app.render(w, "privacy.html", data)
}

func (app *application) bubblePage(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())
	phrases, err := app.api.ListPhrases(jwt)
	if err != nil {
		slog.Error("list phrases", "error", err)
		phrases = []map[string]any{}
	}
	slog.Debug("bubble page", "phrase_count", len(phrases))
	phrasesJSON, _ := json.Marshal(phrases)
	app.renderAuth(w, "bubble.html", map[string]any{
		"Page":        "bubble",
		"PhrasesJSON": template.JS(phrasesJSON),
	})
}

func (app *application) phrasesPage(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())
	phrases, err := app.api.ListPhrases(jwt)
	if err != nil {
		slog.Error("list phrases", "error", err)
		phrases = []map[string]any{}
	}
	slog.Debug("phrases page", "phrase_count", len(phrases))
	phrasesJSON, _ := json.Marshal(phrases)
	app.renderAuth(w, "phrases.html", map[string]any{
		"Page":        "phrases",
		"PhrasesJSON": template.JS(phrasesJSON),
		"Count":       len(phrases),
	})
}

// apiProxy forwards /fd/* requests to the private API using the auth JWT.
// This keeps the API private while letting browser JS call frontdoor endpoints.
func (app *application) apiProxy(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())

	// Strip /fd prefix and map to /api/v1
	path := "/api/v1" + r.URL.Path[len("/fd"):]
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	status, body, err := app.api.Proxy(r.Method, path, jwt, r.Body, r.Header.Get("Content-Type"))
	if err != nil {
		slog.Error("api proxy", "method", r.Method, "path", path, "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	slog.Debug("api proxy", "method", r.Method, "path", path, "status", status)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// addPage is a placeholder — full implementation in next step.
func (app *application) addPage(w http.ResponseWriter, r *http.Request) {
	app.renderAuth(w, "add.html", map[string]any{"Page": "add"})
}

func (app *application) settingsPage(w http.ResponseWriter, r *http.Request) {
	app.renderAuth(w, "settings.html", map[string]any{"Page": "settings"})
}

// ── OAuth 2.1 consent screen ──────────────────────────────────────────────────

// oauthParams holds the query/form values that travel with the OAuth flow.
// They arrive as query params on GET /authorize and are echoed as hidden form
// fields so POST /authorize can re-read them without server-side session state.
type oauthParams struct {
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	ResponseType        string
}

// base64urlRE matches a base64url-encoded SHA256 hash: exactly 43 chars,
// no padding. Mirrors the same check in the backend oauth handler.
var base64urlRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// validateOAuthParams checks that all required OAuth 2.1 params are present and
// that the values we actually enforce (response_type, code_challenge_method) are
// correct. We do NOT validate client_id or redirect_uri here — that's the
// backend's job at /internal/oauth/authorize where the DB check happens.
func validateOAuthParams(p oauthParams) error {
	if p.ClientID == "" || p.RedirectURI == "" || p.CodeChallenge == "" || p.State == "" {
		return errors.New("client_id, redirect_uri, code_challenge, and state are required")
	}
	if p.ResponseType != "code" {
		return errors.New("response_type must be 'code'")
	}
	// OAuth 2.1 mandates S256 — reject "plain" and anything else.
	if p.CodeChallengeMethod != "S256" {
		return errors.New("code_challenge_method must be S256")
	}
	// Validate format early so a bad challenge returns a clear 400 here rather
	// than a generic 500 when the backend rejects it after the user clicks Allow.
	if !base64urlRE.MatchString(p.CodeChallenge) {
		return errors.New("code_challenge must be base64url-encoded SHA256 (43 chars, no padding)")
	}
	return nil
}

// authorizePage handles GET and POST /authorize — the OAuth 2.1 consent screen.
//
// GET: If the user isn't logged in, redirect to /login first (they must authenticate
// before they can grant access). If logged in, render the consent form.
//
// POST: The user submitted the consent form.
//   - "Allow" → call backend to issue an auth code, redirect to redirect_uri?code=...&state=...
//   - "Deny"  → redirect to redirect_uri?error=access_denied&state=...
//
// The state param is always echoed back in the redirect. This is the client's
// anti-CSRF nonce — ChatGPT generates it before the redirect and verifies it
// on the way back to ensure this response is for its own request.
func (app *application) authorizePage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.authorizeGet(w, r)
	case http.MethodPost:
		app.authorizePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *application) authorizeGet(w http.ResponseWriter, r *http.Request) {
	// Require login before showing the consent screen.
	// We handle this inline (not via requireAuth) so we can show a proper 400
	// for bad OAuth params rather than silently redirecting.
	cookie, err := r.Cookie(authCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		// Pass the full /authorize URL as ?next= so the login+magic-link flow
		// brings the user back to the consent screen instead of the home page.
		loginURL := "/login?next=" + url.QueryEscape("/authorize?"+r.URL.RawQuery)
		http.Redirect(w, r, loginURL, http.StatusSeeOther)
		return
	}

	q := r.URL.Query()
	params := oauthParams{
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"),
		State:               q.Get("state"),
		ResponseType:        q.Get("response_type"),
	}

	// Validate well-formedness of params before touching the backend.
	// Per RFC 6749 §4.1.2.1 we must NOT redirect on invalid redirect_uri —
	// show a plain error page instead.
	if err := validateOAuthParams(params); err != nil {
		http.Error(w, "Invalid authorization request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate client_id + redirect_uri with the backend before rendering the
	// consent form. This ensures that both the Allow and Deny paths can safely
	// redirect to redirect_uri — it's already been verified as registered.
	if err := app.api.ValidateOAuthClient(params.ClientID, params.RedirectURI); err != nil {
		slog.Warn("oauth: client validation failed", "error", err)
		http.Error(w, "Invalid authorization request: "+err.Error(), http.StatusBadRequest)
		return
	}

	slog.Debug("oauth: showing consent screen", "client_id", params.ClientID)
	app.render(w, "authorize.html", params)
}

func (app *application) authorizePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// Re-read all OAuth params from hidden form fields.
	// We re-validate on every POST — no server-side session state needed.
	params := oauthParams{
		ClientID:            r.PostForm.Get("client_id"),
		RedirectURI:         r.PostForm.Get("redirect_uri"),
		CodeChallenge:       r.PostForm.Get("code_challenge"),
		CodeChallengeMethod: r.PostForm.Get("code_challenge_method"),
		State:               r.PostForm.Get("state"),
		ResponseType:        r.PostForm.Get("response_type"),
	}
	if err := validateOAuthParams(params); err != nil {
		http.Error(w, "Invalid authorization request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Require login before processing Allow or Deny — consent decisions must be
	// authenticated. Without this check an unauthenticated POST with action=deny
	// could cancel another user's in-flight OAuth flow.
	cookie, err := r.Cookie(authCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	jwt := strings.TrimSpace(cookie.Value)

	// Build the base redirect URI; state is always appended regardless of Allow/Deny.
	redirectBase, err := url.Parse(params.RedirectURI)
	if err != nil {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}
	redirectQuery := redirectBase.Query()
	redirectQuery.Set("state", params.State)

	// Deny: revoke any existing refresh tokens then redirect with error=access_denied
	// (RFC 6749 §4.1.2.1). Revocation prevents the client from refreshing access
	// after a prior authorization; any already-issued access token remains valid
	// until it expires (1 hour).
	if r.PostForm.Get("action") == "deny" {
		if err := app.api.RevokeOAuthTokens(jwt, params.ClientID); err != nil {
			slog.Warn("oauth: user denied consent; token revoke failed", "client_id", params.ClientID, "error", err)
		} else {
			slog.Info("oauth: user denied consent; refresh tokens revoked", "client_id", params.ClientID)
		}
		redirectQuery.Set("error", "access_denied")
		redirectBase.RawQuery = redirectQuery.Encode()
		http.Redirect(w, r, redirectBase.String(), http.StatusSeeOther)
		return
	}

	// Call backend to issue the authorization code. The backend validates
	// client_id and redirect_uri against the registered values and stores
	// the code_challenge for PKCE verification at /token time.
	code, err := app.api.IssueAuthCode(jwt, params.ClientID, params.RedirectURI, params.CodeChallenge)
	if err != nil {
		slog.Error("oauth: issue auth code failed", "error", err)
		http.Error(w, "Failed to issue authorization code. Please try again.", http.StatusInternalServerError)
		return
	}

	// Redirect back to the client with code + state.
	// The client (ChatGPT) will immediately POST code + code_verifier to /token.
	slog.Info("oauth: user granted consent, auth code issued", "client_id", params.ClientID)
	redirectQuery.Set("code", code)
	redirectBase.RawQuery = redirectQuery.Encode()
	http.Redirect(w, r, redirectBase.String(), http.StatusSeeOther)
}

func (app *application) shufflePage(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())
	phrases, err := app.api.ListPhrases(jwt)
	if err != nil {
		slog.Error("list phrases for shuffle", "error", err)
		phrases = []map[string]any{}
	}
	slog.Debug("shuffle page", "phrase_count", len(phrases))
	phrasesJSON, _ := json.Marshal(phrases)
	app.renderAuth(w, "shuffle.html", map[string]any{
		"Page":        "shuffle",
		"PhrasesJSON": template.JS(phrasesJSON),
	})
}
