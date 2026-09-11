package audio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/gorilla/mux"
)

const (
	audioTestUserID   = "550e8400-e29b-41d4-a716-446655440099"
	audioTestPhraseID = "550e8400-e29b-41d4-a716-446655440001"
)

type fakePhraseStore struct {
	get func(context.Context, string, string) (*db.Phrase, error)
}

func (s fakePhraseStore) GetPhrase(ctx context.Context, userID, id string) (*db.Phrase, error) {
	return s.get(ctx, userID, id)
}

type fakeCache struct {
	get func(context.Context, string) ([]byte, error)
	put func(context.Context, string, []byte) error
}

func (c fakeCache) Get(ctx context.Context, key string) ([]byte, error) { return c.get(ctx, key) }
func (c fakeCache) Put(ctx context.Context, key string, clip []byte) error {
	if c.put == nil {
		return nil
	}
	return c.put(ctx, key, clip)
}

type fakeSynth struct {
	synthesize func(context.Context, string) ([]byte, error)
}

func (s fakeSynth) Synthesize(ctx context.Context, text string) ([]byte, error) {
	return s.synthesize(ctx, text)
}

type countingLimiter struct {
	allowed bool
	calls   atomic.Int32
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.deadline = deadline
	return nil
}

func (l *countingLimiter) Allow(string) bool {
	l.calls.Add(1)
	return l.allowed
}

func TestHandlerCacheHitUsesOwnedPhraseAndSkipsGeneration(t *testing.T) {
	var gotUser, gotID, gotKey string
	limiter := &countingLimiter{allowed: true}
	h := testHandler(
		fakePhraseStore{get: func(_ context.Context, userID, id string) (*db.Phrase, error) {
			gotUser, gotID = userID, id
			return &db.Phrase{Phrase: "Exact phrase (parentheses intact)."}, nil
		}},
		fakeCache{get: func(_ context.Context, key string) ([]byte, error) {
			gotKey = key
			return []byte("cached mp3"), nil
		}},
		fakeSynth{synthesize: func(context.Context, string) ([]byte, error) {
			t.Fatal("synthesizer called on cache hit")
			return nil, nil
		}}, limiter,
	)

	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "cached mp3" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if gotUser != audioTestUserID || gotID != audioTestPhraseID {
		t.Fatalf("GetPhrase args = %q, %q", gotUser, gotID)
	}
	if !strings.HasPrefix(gotKey, "phrase-audio/"+audioTestUserID+"/") || !strings.HasSuffix(gotKey, ".mp3") {
		t.Fatalf("cache key = %q", gotKey)
	}
	if limiter.calls.Load() != 0 {
		t.Fatal("limiter consumed on cache hit")
	}
	if recorder.Header().Get("Content-Type") != "audio/mpeg" || recorder.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("headers = %v", recorder.Header())
	}
}

func TestHandlerExtendsWriteDeadlineForColdGeneration(t *testing.T) {
	h := testHandler(storeReturning(db.Phrase{Phrase: "hello"}),
		fakeCache{get: func(context.Context, string) ([]byte, error) { return []byte("cached"), nil }},
		fakeSynth{}, &countingLimiter{allowed: true})
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	serveAudioWithWriter(h, w, context.Background(), audioTestPhraseID)
	minimum := time.Now().Add(h.genTimeout)
	if w.deadline.Before(minimum) {
		t.Fatalf("write deadline = %v, want at least %v", w.deadline, minimum)
	}
}

func TestHandlerMissingPhraseReturns404BeforeCacheAccess(t *testing.T) {
	h := testHandler(
		fakePhraseStore{get: func(context.Context, string, string) (*db.Phrase, error) { return nil, db.ErrNotFound }},
		fakeCache{get: func(context.Context, string) ([]byte, error) {
			t.Fatal("cache accessed before ownership was established")
			return nil, nil
		}}, fakeSynth{}, &countingLimiter{allowed: true},
	)

	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d, headers = %v", recorder.Code, recorder.Header())
	}
}

func TestHandlerCacheKeyIncludesOnlySpeechInputsAndUser(t *testing.T) {
	h := testHandler(nil, nil, nil, nil)
	base, err := h.cacheKey("user-a", "Speak this")
	if err != nil {
		t.Fatal(err)
	}
	if same, _ := h.cacheKey("user-a", "Speak this"); same != base {
		t.Fatalf("same identity produced different keys: %q != %q", same, base)
	}
	for name, mutate := range map[string]func(*Handler){
		"user":     func(changed *Handler) {},
		"text":     func(changed *Handler) {},
		"voice":    func(changed *Handler) { changed.voiceID = "voice-2" },
		"model":    func(changed *Handler) { changed.modelID = "model-2" },
		"settings": func(changed *Handler) { changed.settings.Stability = 0.9 },
	} {
		changed := testHandler(nil, nil, nil, nil)
		userID, text := "user-a", "Speak this"
		if name == "user" {
			userID = "user-b"
		}
		if name == "text" {
			text = "Speak something else"
		}
		mutate(changed)
		key, _ := changed.cacheKey(userID, text)
		if key == base {
			t.Errorf("changing %s did not change cache key", name)
		}
	}
}

func TestHandlerMissSynthesizesExactStoredTextAndCaches(t *testing.T) {
	const phraseText = "They put it under scrutiny (again)."
	var synthesizedText, putKey, putClip string
	limiter := &countingLimiter{allowed: true}
	h := testHandler(
		storeReturning(db.Phrase{Phrase: phraseText, Note: "must not be spoken", Headwords: []db.Headword{{Meaning: "not spoken"}}}),
		fakeCache{
			get: func(context.Context, string) ([]byte, error) { return nil, ErrCacheMiss },
			put: func(_ context.Context, key string, clip []byte) error {
				putKey, putClip = key, string(clip)
				return nil
			},
		},
		fakeSynth{synthesize: func(_ context.Context, text string) ([]byte, error) {
			synthesizedText = text
			return []byte("new mp3"), nil
		}}, limiter,
	)

	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "new mp3" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if synthesizedText != phraseText || putClip != "new mp3" || putKey == "" {
		t.Fatalf("synth text = %q, put = %q at %q", synthesizedText, putClip, putKey)
	}
	if limiter.calls.Load() != 1 {
		t.Fatalf("limiter calls = %d", limiter.calls.Load())
	}
}

func TestHandlerConcurrentMissesShareGeneration(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var synthCalls atomic.Int32
	var cached atomic.Bool
	limiter := &countingLimiter{allowed: true}
	h := testHandler(storeReturning(db.Phrase{Phrase: "Shared"}),
		fakeCache{
			get: func(context.Context, string) ([]byte, error) {
				if cached.Load() {
					return []byte("shared mp3"), nil
				}
				return nil, ErrCacheMiss
			},
			put: func(context.Context, string, []byte) error { cached.Store(true); return nil },
		},
		fakeSynth{synthesize: func(context.Context, string) ([]byte, error) {
			if synthCalls.Add(1) == 1 {
				close(started)
			}
			<-release
			return []byte("shared mp3"), nil
		}}, limiter)

	const requests = 6
	results := make(chan *httptest.ResponseRecorder, requests)
	for range requests {
		go func() { results <- serveAudio(h, context.Background(), audioTestPhraseID) }()
	}
	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)
	for range requests {
		recorder := <-results
		if recorder.Code != http.StatusOK || recorder.Body.String() != "shared mp3" {
			t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
		}
	}
	if synthCalls.Load() != 1 || limiter.calls.Load() != 1 {
		t.Fatalf("synth calls = %d, limiter calls = %d", synthCalls.Load(), limiter.calls.Load())
	}
}

func TestHandlerCanceledWaiterDoesNotCancelSharedGeneration(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	putDone := make(chan struct{})
	h := testHandler(storeReturning(db.Phrase{Phrase: "Keep going"}),
		fakeCache{
			get: func(context.Context, string) ([]byte, error) { return nil, ErrCacheMiss },
			put: func(context.Context, string, []byte) error { close(putDone); return nil },
		},
		fakeSynth{synthesize: func(ctx context.Context, _ string) ([]byte, error) {
			close(started)
			select {
			case <-release:
				return []byte("completed"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}, &countingLimiter{allowed: true})

	requestCtx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- serveAudio(h, requestCtx, audioTestPhraseID) }()
	<-started
	cancel()
	<-firstDone
	close(release)
	select {
	case <-putDone:
	case <-time.After(time.Second):
		t.Fatal("shared generation did not finish and cache after waiter cancellation")
	}
}

func TestHandlerApplicationShutdownCancelsSharedGeneration(t *testing.T) {
	appCtx, stopApp := context.WithCancel(context.Background())
	started := make(chan struct{})
	h := NewHandler(appCtx, storeReturning(db.Phrase{Phrase: "Stop on shutdown"}),
		fakeCache{get: func(context.Context, string) ([]byte, error) { return nil, ErrCacheMiss }},
		fakeSynth{synthesize: func(ctx context.Context, _ string) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}}, "voice-1", "model-1")

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- serveAudio(h, context.Background(), audioTestPhraseID) }()
	<-started
	stopApp()
	select {
	case recorder := <-done:
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", recorder.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("generation did not stop when application context was canceled")
	}
}

func TestHandlerErrorContract(t *testing.T) {
	tests := []struct {
		name       string
		cacheErr   error
		synthErr   error
		allowed    bool
		wantStatus int
		wantSynth  int32
	}{
		{name: "storage failure", cacheErr: errors.New("r2 unavailable"), allowed: true, wantStatus: http.StatusServiceUnavailable},
		{name: "rate limited", cacheErr: ErrCacheMiss, allowed: false, wantStatus: http.StatusTooManyRequests},
		{name: "synthesis failure", cacheErr: ErrCacheMiss, synthErr: ErrSynthesis, allowed: true, wantStatus: http.StatusBadGateway, wantSynth: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var synthCalls atomic.Int32
			var putCalls atomic.Int32
			h := testHandler(storeReturning(db.Phrase{Phrase: "hello"}),
				fakeCache{
					get: func(context.Context, string) ([]byte, error) { return nil, tt.cacheErr },
					put: func(context.Context, string, []byte) error { putCalls.Add(1); return nil },
				},
				fakeSynth{synthesize: func(context.Context, string) ([]byte, error) { synthCalls.Add(1); return nil, tt.synthErr }},
				&countingLimiter{allowed: tt.allowed})
			recorder := serveAudio(h, context.Background(), audioTestPhraseID)
			if recorder.Code != tt.wantStatus || recorder.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response = %d, headers = %v", recorder.Code, recorder.Header())
			}
			if synthCalls.Load() != tt.wantSynth {
				t.Fatalf("synth calls = %d, want %d", synthCalls.Load(), tt.wantSynth)
			}
			if putCalls.Load() != 0 {
				t.Fatalf("cache Put calls = %d after failed request", putCalls.Load())
			}
		})
	}
}

func TestHandlerRejectsMalformedPhraseIDBeforeStoreAccess(t *testing.T) {
	h := testHandler(fakePhraseStore{get: func(context.Context, string, string) (*db.Phrase, error) {
		t.Fatal("store called with malformed phrase ID")
		return nil, nil
	}}, fakeCache{}, fakeSynth{}, &countingLimiter{allowed: true})

	recorder := serveAudio(h, context.Background(), "not-a-uuid")
	if recorder.Code != http.StatusBadRequest || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d, headers = %v", recorder.Code, recorder.Header())
	}
}

func TestHandlerSecondCacheFailureDoesNotGenerate(t *testing.T) {
	var gets atomic.Int32
	var synthCalls atomic.Int32
	h := testHandler(storeReturning(db.Phrase{Phrase: "hello"}),
		fakeCache{get: func(context.Context, string) ([]byte, error) {
			if gets.Add(1) == 1 {
				return nil, ErrCacheMiss
			}
			return nil, errors.New("r2 failed during double check")
		}},
		fakeSynth{synthesize: func(context.Context, string) ([]byte, error) {
			synthCalls.Add(1)
			return []byte("unexpected"), nil
		}}, &countingLimiter{allowed: true})

	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusServiceUnavailable || synthCalls.Load() != 0 {
		t.Fatalf("response = %d, synth calls = %d", recorder.Code, synthCalls.Load())
	}
}

func TestHandlerDisabledReturns503WithoutLoadingPhrase(t *testing.T) {
	h := NewHandler(context.Background(), fakePhraseStore{get: func(context.Context, string, string) (*db.Phrase, error) {
		t.Fatal("store called while audio is disabled")
		return nil, nil
	}}, nil, nil, "", "")
	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandlerCacheUploadFailureStillReturnsAudio(t *testing.T) {
	h := testHandler(storeReturning(db.Phrase{Phrase: "hello"}),
		fakeCache{
			get: func(context.Context, string) ([]byte, error) { return nil, ErrCacheMiss },
			put: func(context.Context, string, []byte) error { return errors.New("upload failed") },
		},
		fakeSynth{synthesize: func(context.Context, string) ([]byte, error) { return []byte("play me"), nil }},
		&countingLimiter{allowed: true})
	recorder := serveAudio(h, context.Background(), audioTestPhraseID)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "play me" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestRollingLimiterUsesRollingPerUserWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	limiter := newRollingLimiter(2, time.Hour)
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("a") || !limiter.Allow("a") || limiter.Allow("a") {
		t.Fatal("user a limit was not enforced")
	}
	if !limiter.Allow("b") {
		t.Fatal("user b was not independently limited")
	}
	now = now.Add(time.Hour + time.Nanosecond)
	if !limiter.Allow("a") {
		t.Fatal("expired generations were not removed")
	}
}

func testHandler(store phraseStore, cache objectCache, synth synthesizer, limiter generationLimiter) *Handler {
	h := NewHandler(context.Background(), store, cache, synth, "voice-1", "model-1")
	if limiter != nil {
		h.limiter = limiter
	}
	return h
}

func storeReturning(phrase db.Phrase) phraseStore {
	return fakePhraseStore{get: func(context.Context, string, string) (*db.Phrase, error) { return &phrase, nil }}
}

func serveAudio(h *Handler, ctx context.Context, id string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	serveAudioWithWriter(h, recorder, ctx, id)
	return recorder
}

func serveAudioWithWriter(h *Handler, w http.ResponseWriter, ctx context.Context, id string) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/phrases/"+id+"/audio", nil).WithContext(ctx)
	request = request.WithContext(context.WithValue(request.Context(), middleware.UserIDKey, audioTestUserID))
	request = mux.SetURLVars(request, map[string]string{"id": id})
	h.get(w, request)
}
