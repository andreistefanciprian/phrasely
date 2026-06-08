package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type ctxKey string

const ctxKeyJWT ctxKey = "jwt"

func jwtFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyJWT).(string)
	return v
}

// requireAuth wraps a handler and redirects unauthenticated requests to /login.
func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		jwt := app.sessions.Get(cookie.Value)
		if jwt == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyJWT, jwt)
		next(w, r.WithContext(ctx))
	}
}

// ── Page handlers ─────────────────────────────────────────────────────────────

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	app.render(w, "404.html", nil)
}

func (app *application) homePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w, r)
		return
	}
	// Already signed in → go to bubble
	if cookie, err := r.Cookie("session"); err == nil {
		if app.sessions.Get(cookie.Value) != "" {
			http.Redirect(w, r, "/bubble", http.StatusSeeOther)
			return
		}
	}
	app.render(w, "home.html", nil)
}

func (app *application) loginPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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

	sessionID := app.sessions.Create(jwt)
	// Secure=true only over HTTPS — false on plain HTTP localhost
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/bubble", http.StatusSeeOther)
}

func (app *application) signOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if cookie, err := r.Cookie("session"); err == nil {
		app.sessions.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "session",
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) storyPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, "story.html", nil)
}

func (app *application) bubblePage(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())
	phrases, err := app.api.ListPhrases(jwt)
	if err != nil {
		slog.Error("list phrases", "error", err)
		phrases = []map[string]any{}
	}
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
	phrasesJSON, _ := json.Marshal(phrases)
	app.renderAuth(w, "phrases.html", map[string]any{
		"Page":        "phrases",
		"PhrasesJSON": template.JS(phrasesJSON),
		"Count":       len(phrases),
	})
}

// apiProxy forwards /fd/* requests to the private API using the session JWT.
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
		slog.Error("api proxy", "path", path, "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// addPage is a placeholder — full implementation in next step.
func (app *application) addPage(w http.ResponseWriter, r *http.Request) {
	app.renderAuth(w, "add.html", map[string]any{"Page": "add"})
}

func (app *application) indexPage(w http.ResponseWriter, r *http.Request) {
	jwt := jwtFromContext(r.Context())
	phrases, err := app.api.ListPhrases(jwt)
	if err != nil {
		slog.Error("list phrases for index", "error", err)
		phrases = []map[string]any{}
	}
	phrasesJSON, _ := json.Marshal(phrases)
	app.renderAuth(w, "index.html", map[string]any{
		"Page":        "",
		"PhrasesJSON": template.JS(phrasesJSON),
	})
}
