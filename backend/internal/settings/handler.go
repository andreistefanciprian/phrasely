package settings

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/gorilla/mux"
)

type Handler struct {
	store db.Store
}

func NewHandler(store db.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/settings/email", h.get).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/settings/email", h.update).Methods(http.MethodPost)
}

var validFrequencies = map[string]bool{"daily": true, "weekly": true, "monthly": true, "disabled": true}

type digestSettings struct {
	Frequency string `json:"frequency"`
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	prefs, err := h.store.GetDigestPreferences(r.Context(), userID)
	if errors.Is(err, db.ErrNotFound) {
		respond(w, http.StatusOK, digestSettings{Frequency: "disabled"})
		return
	}
	if err != nil {
		slog.Error("get digest preferences", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get preferences")
		return
	}

	respond(w, http.StatusOK, digestSettings{Frequency: prefs.Frequency})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req digestSettings
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validFrequencies[req.Frequency] {
		respondErr(w, http.StatusBadRequest, "frequency must be one of: daily, weekly, monthly, disabled")
		return
	}

	prefs, err := h.store.UpsertDigestPreferences(r.Context(), userID, req.Frequency)
	if err != nil {
		slog.Error("upsert digest preferences", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to save preferences")
		return
	}

	respond(w, http.StatusOK, digestSettings{Frequency: prefs.Frequency})
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
