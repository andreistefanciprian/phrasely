package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/sync/singleflight"
)

const (
	cacheSchemaVersion      = 1
	cacheKeyPrefix          = "phrase-audio"
	generationLimit         = 40
	generationWindow        = time.Hour
	sharedGenerationTimeout = 2 * time.Minute
)

var (
	errUnavailable = errors.New("phrase audio unavailable")
	errRateLimited = errors.New("phrase audio generation limit reached")
)

// AudioCacheIdentity contains every input that can change the generated clip.
// Its JSON encoding is hashed to produce an unambiguous, stable cache key.
type AudioCacheIdentity struct {
	Version      int           `json:"version"`
	Text         string        `json:"text"`
	VoiceID      string        `json:"voice_id"`
	ModelID      string        `json:"model_id"`
	OutputFormat string        `json:"output_format"`
	Settings     VoiceSettings `json:"settings"`
}

type phraseStore interface {
	GetPhrase(ctx context.Context, userID, id string) (*db.Phrase, error)
}

type objectCache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, audio []byte) error
}

type synthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

type generationLimiter interface {
	Allow(userID string) bool
}

// Handler owns the complete authenticated phrase-audio request flow.
type Handler struct {
	appCtx     context.Context
	store      phraseStore
	cache      objectCache
	synth      synthesizer
	voiceID    string
	modelID    string
	settings   VoiceSettings
	limiter    generationLimiter
	group      singleflight.Group
	genTimeout time.Duration
}

// NewHandler builds the production audio endpoint. A nil cache or synthesizer
// deliberately leaves the route registered but unavailable.
func NewHandler(appCtx context.Context, store phraseStore, cache objectCache, synth synthesizer, voiceID, modelID string) *Handler {
	return &Handler{
		appCtx:     appCtx,
		store:      store,
		cache:      cache,
		synth:      synth,
		voiceID:    voiceID,
		modelID:    modelID,
		settings:   defaultVoiceSettings(),
		limiter:    newRollingLimiter(generationLimit, generationWindow),
		genTimeout: sharedGenerationTimeout,
	}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/phrases/{id}/audio", h.get).Methods(http.MethodGet)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.cache == nil || h.synth == nil {
		respondError(w, http.StatusServiceUnavailable, "phrase audio unavailable")
		return
	}
	// Cold audio generation has a larger bounded budget than the server's JSON
	// routes. Extend only this response's deadline instead of weakening the
	// server-wide slow-client protection.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.genTimeout + 5*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.Warn("extend phrase audio write deadline", "error", err)
	}

	userID := middleware.UserIDFromContext(r.Context())
	id := mux.Vars(r)["id"]
	if _, err := uuid.Parse(id); err != nil {
		respondError(w, http.StatusBadRequest, "invalid phrase id")
		return
	}
	phrase, err := h.store.GetPhrase(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("load phrase for audio", "error", err)
		respondError(w, http.StatusServiceUnavailable, "phrase audio unavailable")
		return
	}

	key, err := h.cacheKey(userID, phrase.Phrase)
	if err != nil {
		slog.Error("build phrase audio cache key", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to prepare phrase audio")
		return
	}

	clip, err := h.cache.Get(r.Context(), key)
	if err == nil {
		writeAudio(w, clip)
		return
	}
	if !errors.Is(err, ErrCacheMiss) {
		slog.Error("read phrase audio cache", "error", err)
		respondError(w, http.StatusServiceUnavailable, "phrase audio unavailable")
		return
	}

	result := h.group.DoChan(key, func() (any, error) {
		return h.generate(key, userID, phrase.Phrase)
	})
	select {
	case <-r.Context().Done():
		return
	case outcome := <-result:
		if outcome.Err != nil {
			h.respondGenerationError(w, outcome.Err)
			return
		}
		writeAudio(w, outcome.Val.([]byte))
	}
}

func (h *Handler) generate(key, userID, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(h.appCtx, h.genTimeout)
	defer cancel()

	clip, err := h.cache.Get(ctx, key)
	if err == nil {
		return clip, nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return nil, fmt.Errorf("%w: cache lookup: %v", errUnavailable, err)
	}
	if !h.limiter.Allow(userID) {
		return nil, errRateLimited
	}

	clip, err = h.synth.Synthesize(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("synthesize phrase: %w", err)
	}
	if err := h.cache.Put(ctx, key, clip); err != nil {
		slog.Error("cache generated phrase audio", "error", err)
	}
	return clip, nil
}

func (h *Handler) cacheKey(userID, text string) (string, error) {
	identity := AudioCacheIdentity{
		Version: cacheSchemaVersion, Text: text, VoiceID: h.voiceID, ModelID: h.modelID,
		OutputFormat: outputFormat, Settings: h.settings,
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%s/%s/%s.mp3", cacheKeyPrefix, userID, hex.EncodeToString(digest[:])), nil
}

func (h *Handler) respondGenerationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRateLimited):
		respondError(w, http.StatusTooManyRequests, "phrase audio generation limit reached")
	case errors.Is(err, ErrSynthesis):
		slog.Error("synthesize phrase audio", "error", err)
		respondError(w, http.StatusBadGateway, "failed to generate phrase audio")
	default:
		slog.Error("prepare phrase audio", "error", err)
		respondError(w, http.StatusServiceUnavailable, "phrase audio unavailable")
	}
}

func writeAudio(w http.ResponseWriter, clip []byte) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(clip)
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

type rollingLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	events map[string][]time.Time
}

func newRollingLimiter(limit int, window time.Duration) *rollingLimiter {
	return &rollingLimiter{limit: limit, window: window, now: time.Now, events: make(map[string][]time.Time)}
}

func (l *rollingLimiter) Allow(userID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	events := l.events[userID]
	firstLive := 0
	for firstLive < len(events) && !events[firstLive].After(cutoff) {
		firstLive++
	}
	events = events[firstLive:]
	if len(events) >= l.limit {
		l.events[userID] = events
		return false
	}
	l.events[userID] = append(events, now)
	return true
}
