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

var templates = template.Must(template.ParseFS(files, "templates/*.html"))

func main() {
	mux := http.NewServeMux()

	// Static assets (CSS, favicon, images)
	staticFiles, err := fs.Sub(files, "static")
	if err != nil {
		slog.Error("static files not found", "error", err)
		os.Exit(1)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	// Health check — used by Railway
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Pages
	mux.HandleFunc("/", homePage)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	slog.Info("frontdoor listening", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func homePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := templates.ExecuteTemplate(w, "base.html", nil); err != nil {
		slog.Error("render home", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
