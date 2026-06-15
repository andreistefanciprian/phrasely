package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	apiURL := os.Getenv("API_HOST")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	slog.Info("mcp server listening", "port", port, "api", apiURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
