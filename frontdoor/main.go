package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
)

//go:embed templates/* static/*
var files embed.FS

type application struct {
	sessions *SessionStore
	api      *apiClient
}

func main() {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	app := &application{
		sessions: newSessionStore(),
		api:      newAPIClient(apiURL),
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

	// Public routes
	mux.HandleFunc("/", app.homePage)
	mux.HandleFunc("/login", app.loginPage)
	mux.HandleFunc("/story", app.storyPage)
	mux.HandleFunc("/auth/verify", app.authVerify) // API-side verify (internal)
	mux.HandleFunc("/auth-verify", app.authVerify) // magic link landing (what the API emails)
	mux.HandleFunc("/sign-out", app.signOut)

	// Protected pages
	mux.HandleFunc("/bubble", app.requireAuth(app.bubblePage))
	mux.HandleFunc("/phrases", app.requireAuth(app.phrasesPage))
	mux.HandleFunc("/add", app.requireAuth(app.addPage))
	mux.HandleFunc("/index", app.requireAuth(app.indexPage))

	// API proxy — session-authenticated, forwards to private API
	mux.HandleFunc("/fd/", app.requireAuth(app.apiProxy))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	slog.Info("frontdoor listening", "port", port, "api", apiURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
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
