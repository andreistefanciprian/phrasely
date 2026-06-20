package phrases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/embeddings"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Handler holds a reference to the store interface, not the concrete Postgres type.
// This allows tests to inject a mock store without a real database.
type Handler struct {
	store          db.Store
	embedder       *embeddings.Service // nil when OPENAI_API_KEY is not set
	relatedMaxDist float64
}

func NewHandler(store db.Store, embedder *embeddings.Service, relatedMaxDist float64) *Handler {
	return &Handler{store: store, embedder: embedder, relatedMaxDist: relatedMaxDist}
}

// RegisterRoutes attaches phrase endpoints to the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/phrases/search", h.search).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases/summary", h.listSummary).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases/random", h.randomPhrases).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases", h.list).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases", h.create).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/phrases/{id}/related", h.related).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases/{id}", h.get).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases/{id}", h.update).Methods(http.MethodPatch)
	r.HandleFunc("/api/v1/phrases/{id}", h.delete).Methods(http.MethodDelete)
	r.HandleFunc("/internal/phrases/embed-backfill", h.backfillEmbeddings).Methods(http.MethodPost)
}

// list handles GET /api/v1/phrases.
// Accepts an optional ?headword= query param for filtering by headword.
// Always returns a JSON array — empty array when there are no results.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	headword := r.URL.Query().Get("headword")

	phrases, err := h.store.ListPhrases(r.Context(), userID, headword)
	if err != nil {
		slog.Error("list phrases", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to list phrases")
		return
	}

	slog.Debug("list phrases", "count", len(phrases), "filtered", headword != "")
	respond(w, http.StatusOK, phrases)
}

// listSummary handles GET /api/v1/phrases/summary.
// Returns a lightweight projection (phrase, headwords) for the MCP list_phrases tool.
func (h *Handler) listSummary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	headword := r.URL.Query().Get("headword")
	phrases, err := h.store.ListPhrasesSummary(r.Context(), userID, headword)
	if err != nil {
		slog.Error("list phrases summary", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to list phrases")
		return
	}
	slog.Debug("list phrases summary", "count", len(phrases))
	respond(w, http.StatusOK, phrases)
}

// randomPhrases handles GET /api/v1/phrases/random.
// Accepts an optional ?count= query param (default 1, max 10).
func (h *Handler) randomPhrases(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	count := 1
	if raw := r.URL.Query().Get("count"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			count = n
		}
	}
	if count > 10 {
		count = 10
	}

	phrases, err := h.store.GetRandomPhrases(r.Context(), userID, count)
	if err != nil {
		slog.Error("random phrases", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get random phrases")
		return
	}

	slog.Debug("random phrases", "count", len(phrases))
	respond(w, http.StatusOK, phrases)
}

// get handles GET /api/v1/phrases/{id}.
// Returns 404 if the phrase does not exist or belongs to another user.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	phrase, err := h.store.GetPhrase(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("get phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get phrase")
		return
	}

	slog.Debug("get phrase", "id", id)
	respond(w, http.StatusOK, phrase)
}

// maxBodyBytes is the maximum request body size we accept (1 KB).
// A phrase, headword, and note easily fit within this; anything larger is rejected.
// Note: blank-string headwords (e.g. [""]) are currently accepted; revisit if it
// causes issues with curation or display.
const maxBodyBytes = 1024

// create handles POST /api/v1/phrases.
// It decodes the request body, validates required fields, and inserts the phrase.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req db.CreatePhraseRequest
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// phrase and at least one headword are required; note is optional
	if req.Phrase == "" || len(req.Headwords) == 0 {
		respondErr(w, http.StatusBadRequest, "phrase and at least one headword are required")
		return
	}

	phrase, err := h.store.CreatePhrase(r.Context(), userID, req)
	if err != nil {
		slog.Error("create phrase", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to create phrase")
		return
	}

	if h.embedder != nil {
		go h.embedPhrase(phrase)
	}

	slog.Debug("create phrase", "id", phrase.ID, "headword_count", len(phrase.Headwords))
	respond(w, http.StatusCreated, phrase)
}

// update handles PATCH /api/v1/phrases/{id}.
// Only non-null fields in the request body are updated; omitted fields and
// explicit JSON null values both leave the current value unchanged.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req db.UpdatePhraseRequest
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Phrase == nil && req.Headwords == nil && req.Note == nil && req.SourceURLs == nil {
		respondErr(w, http.StatusBadRequest, "at least one field must be provided")
		return
	}
	if req.Phrase != nil && *req.Phrase == "" {
		respondErr(w, http.StatusBadRequest, "phrase cannot be empty")
		return
	}
	if req.Headwords != nil && len(req.Headwords) == 0 {
		respondErr(w, http.StatusBadRequest, "headwords cannot be empty")
		return
	}

	phrase, err := h.store.UpdatePhrase(r.Context(), userID, id, req)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("update phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to update phrase")
		return
	}

	if h.embedder != nil {
		go h.embedPhrase(phrase)
	}

	slog.Debug("update phrase", "id", id)
	respond(w, http.StatusOK, phrase)
}

// delete handles DELETE /api/v1/phrases/{id}.
// Returns 204 on success, 404 if the phrase does not exist or belongs to another user.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.store.DeletePhrase(r.Context(), userID, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("delete phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to delete phrase")
		return
	}

	slog.Debug("delete phrase", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// search handles GET /api/v1/phrases/search?q=<query>.
// Embeds the query and returns the top-10 phrases by cosine similarity.
// Returns 400 if q is missing, 503 if the embedder is not configured.
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	if h.embedder == nil {
		respondErr(w, http.StatusServiceUnavailable, "semantic search is not available")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		respondErr(w, http.StatusBadRequest, "q is required")
		return
	}
	if len(q) > 500 {
		respondErr(w, http.StatusBadRequest, "q must be 500 characters or fewer")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	vec, err := h.embedder.Embed(ctx, q)
	if err != nil {
		slog.Error("embed search query", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to embed query")
		return
	}

	phrases, err := h.store.SearchPhrasesBySimilarity(ctx, userID, vec, 10)
	if err != nil {
		slog.Error("search phrases", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to search phrases")
		return
	}

	slog.Debug("search phrases", "q", q, "results", len(phrases))
	respond(w, http.StatusOK, phrases)
}

// related handles GET /api/v1/phrases/{id}/related?limit=5.
// Returns phrases semantically close to the given phrase, capped by RELATED_MAX_DISTANCE.
// Returns an empty array when no phrases fall within the distance threshold.
func (h *Handler) related(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	limit := 5
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}

	phrases, err := h.store.GetRelatedPhrases(r.Context(), userID, id, h.relatedMaxDist, limit)
	if err != nil {
		slog.Error("get related phrases", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get related phrases")
		return
	}

	slog.Debug("related phrases", "id", id, "results", len(phrases))
	respond(w, http.StatusOK, phrases)
}

// parseID extracts and validates the {id} path variable as a UUID.
func parseID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := mux.Vars(r)["id"]
	if _, err := uuid.Parse(raw); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid id: must be a UUID")
		return "", false
	}
	return raw, true
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}

// embedPhrase generates and stores an embedding for p in the background.
// If OpenAI is unavailable the error is logged and the phrase is left with a
// NULL embedding — the backfill endpoint will pick it up later.
func (h *Handler) embedPhrase(p *db.Phrase) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := h.embedder.Embed(ctx, embeddings.PhraseText(*p))
	if err != nil {
		slog.Error("embed phrase", "id", p.ID, "error", err)
		return
	}
	if err := h.store.SetPhraseEmbedding(ctx, p.ID, vec); err != nil {
		slog.Error("set phrase embedding", "id", p.ID, "error", err)
		return
	}
	slog.Debug("embed phrase", "id", p.ID)
}

// backfillEmbeddings handles POST /internal/phrases/embed-backfill.
// It embeds every phrase that currently has a NULL embedding — used after the
// initial migration and as a recovery path when OpenAI was temporarily unavailable.
func (h *Handler) backfillEmbeddings(w http.ResponseWriter, r *http.Request) {
	if h.embedder == nil {
		respondErr(w, http.StatusServiceUnavailable, "embedder not configured: set OPENAI_API_KEY")
		return
	}

	phrases, err := h.store.ListPhrasesWithoutEmbedding(r.Context())
	if err != nil {
		slog.Error("backfill: list phrases without embedding", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to list phrases")
		return
	}

	slog.Info("backfill started", "total", len(phrases))
	respond(w, http.StatusAccepted, map[string]int{"queued": len(phrases)})

	go func() {
		var embedded, failed int
		total := len(phrases)
		for i, p := range phrases {
			slog.Info("backfill: embedding phrase", "progress", fmt.Sprintf("%d/%d", i+1, total), "id", p.ID, "headwords", p.Headwords)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			vec, err := h.embedder.Embed(ctx, embeddings.PhraseText(p))
			if err != nil {
				cancel()
				slog.Error("backfill: embed failed", "progress", fmt.Sprintf("%d/%d", i+1, total), "id", p.ID, "error", err)
				failed++
				continue
			}
			if err := h.store.SetPhraseEmbedding(ctx, p.ID, vec); err != nil {
				cancel()
				slog.Error("backfill: store failed", "progress", fmt.Sprintf("%d/%d", i+1, total), "id", p.ID, "error", err)
				failed++
				continue
			}
			cancel()
			embedded++
			slog.Info("backfill: phrase done", "progress", fmt.Sprintf("%d/%d", i+1, total), "id", p.ID, "embedded_so_far", embedded)
		}
		slog.Info("backfill complete", "total", total, "embedded", embedded, "failed", failed)
	}()
}
