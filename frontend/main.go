package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

//go:embed templates/* static/* home_phrases.json home_shuffle_phrases.json
var files embed.FS

type application struct {
	api                *apiClient
	homePhraseSamples  []homePhrase
	homeShuffleSamples []homeShufflePhrase
}

func main() {
	// Configure structured JSON logging first so every subsequent log line
	// is parseable by Railway's log viewer. Level defaults to INFO.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	})))

	apiURL := os.Getenv("API_HOST")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	var homePhraseSamples []homePhrase
	if data, err := files.ReadFile("home_phrases.json"); err != nil {
		slog.Error("failed to read home_phrases.json", "error", err)
		os.Exit(1)
	} else if err := json.Unmarshal(data, &homePhraseSamples); err != nil {
		slog.Error("failed to parse home_phrases.json", "error", err)
		os.Exit(1)
	}
	var homeShuffleSamples []homeShufflePhrase
	if data, err := files.ReadFile("home_shuffle_phrases.json"); err != nil {
		slog.Error("failed to read home_shuffle_phrases.json", "error", err)
		os.Exit(1)
	} else if err := json.Unmarshal(data, &homeShuffleSamples); err != nil {
		slog.Error("failed to parse home_shuffle_phrases.json", "error", err)
		os.Exit(1)
	}

	app := &application{
		api:                newAPIClient(apiURL),
		homePhraseSamples:  homePhraseSamples,
		homeShuffleSamples: homeShuffleSamples,
	}

	mux := http.NewServeMux()

	// Static assets
	staticFiles, err := fs.Sub(files, "static")
	if err != nil {
		slog.Error("static files not found", "error", err)
		os.Exit(1)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// LLM / SEO files served at root
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		f, _ := files.ReadFile("static/robots.txt")
		w.Write(f)
	})
	mux.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		f, _ := files.ReadFile("static/llms.txt")
		w.Write(f)
	})

	// Public routes
	mux.HandleFunc("/", app.homePage)
	mux.HandleFunc("/login", app.loginPage)
	mux.HandleFunc("/story", app.storyPage)
	mux.HandleFunc("/privacy", app.privacyPage)
	mux.HandleFunc("/terms", app.termsPage)
	mux.HandleFunc("/auth/verify", app.authVerify) // API-side verify (internal)
	mux.HandleFunc("/auth-verify", app.authVerify) // magic link landing (what the API emails)
	mux.HandleFunc("/sign-out", app.signOut)

	// OAuth 2.1 consent screen — auth check is handled inside the handler so
	// we can show proper errors for bad params before redirecting to login.
	mux.HandleFunc("/authorize", app.authorizePage)

	// Protected pages
	mux.HandleFunc("/bubble", app.requireAuth(app.bubblePage))
	mux.HandleFunc("/phrases", app.requireAuth(app.phrasesPage))
	mux.HandleFunc("/add", app.requireAuth(app.addPage))
	mux.HandleFunc("/shuffle", app.requireAuth(app.shufflePage))
	mux.HandleFunc("/settings", app.requireAuth(app.settingsPage))

	// API proxy — cookie-authenticated, forwards to private API
	mux.HandleFunc("/fd/", app.requireAuth(app.apiProxy))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Wrap the entire mux: body size limit + security headers on every response.
	const maxBody int64 = 8 * 1024 // 8 KB
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers set first so they appear on every response,
		// including early-exit error responses (413, etc.).
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// CSP: allow fonts from Google, images from self + data URIs, no inline scripts
		// CSP notes:
		// - 'unsafe-inline' required for script-src and style-src because templates
		//   embed <script> and <style> blocks directly in HTML.
		// - data: allowed for img-src (canvas bubble uses data URIs).
		// Moving scripts/styles to external files would allow removing 'unsafe-inline'.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src https://fonts.gstatic.com; "+
				"img-src 'self' data:; "+
				"frame-ancestors 'none'")

		// Reject oversized requests before reading or proxying the body.
		if r.ContentLength > maxBody {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)

		mux.ServeHTTP(w, r)
	})

	slog.Info("frontdoor listening", "port", port, "api", apiURL)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
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

// render renders a public page (base.html + page template).
func (app *application) render(w http.ResponseWriter, name string, data any) {
	app.renderWith("base.html", w, name, data)
}

// renderAuth renders an authenticated page (base-auth.html + navbar + page template).
func (app *application) renderAuth(w http.ResponseWriter, name string, data any) {
	app.renderWith("base-auth.html", w, name, data)
}

func (app *application) renderWith(base string, w http.ResponseWriter, name string, data any) {
	t, err := template.ParseFS(files,
		"templates/"+base,
		"templates/navbar.html",
		"templates/"+name,
	)
	if err != nil {
		slog.Error("parse template", "template", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, base, data); err != nil {
		slog.Error("render template", "template", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
