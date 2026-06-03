package curate

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

type Handler struct {
	curator *Curator
}

func NewHandler(curator *Curator) *Handler {
	return &Handler{curator: curator}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/phrases/curate", h.curate).Methods(http.MethodPost)
}

// CurateRequest is the body expected by POST /api/v1/phrases/curate.
type CurateRequest struct {
	Input string `json:"input"`
}

// curate handles POST /api/v1/phrases/curate.
// It takes a raw phrase, calls OpenAI to clean and enrich it,
// and returns a structured phrase ready to be reviewed and saved.
func (h *Handler) curate(w http.ResponseWriter, r *http.Request) {
	var req CurateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Input == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	result, err := h.curator.Curate(r.Context(), req.Input)
	if err != nil {
		slog.Error("curate phrase", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to curate phrase"})
		return
	}

	respond(w, http.StatusOK, result)
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
