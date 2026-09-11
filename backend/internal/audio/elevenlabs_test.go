package audio

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestElevenLabsSynthesizeSendsExpectedRequest(t *testing.T) {
	var got textToSpeechRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/text-to-speech/voice-123" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if value := r.URL.Query().Get("output_format"); value != outputFormat {
			t.Errorf("output_format = %q", value)
		}
		if value := r.Header.Get("xi-api-key"); value != "secret" {
			t.Errorf("xi-api-key = %q", value)
		}
		if value := r.Header.Get("Accept"); value != "audio/mpeg" {
			t.Errorf("Accept = %q", value)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3 bytes"))
	}))
	defer server.Close()

	synth := testElevenLabs(server)
	audio, err := synth.Synthesize(context.Background(), "She stood up to scrutiny (even then).")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != "mp3 bytes" {
		t.Fatalf("audio = %q", audio)
	}
	if got.Text != "She stood up to scrutiny (even then)." || got.ModelID != "model-1" {
		t.Fatalf("request = %+v", got)
	}
	if got.VoiceSettings != defaultVoiceSettings() {
		t.Fatalf("voice settings = %+v", got.VoiceSettings)
	}
}

func TestElevenLabsSynthesizeRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		maxBytes    int64
	}{
		{name: "provider error", status: http.StatusTooManyRequests, contentType: "application/json", body: `{"detail":"secret"}`},
		{name: "wrong content type", status: http.StatusOK, contentType: "application/json", body: "not audio"},
		{name: "empty response", status: http.StatusOK, contentType: "audio/mpeg"},
		{name: "oversized response", status: http.StatusOK, contentType: "audio/mpeg", body: "12345", maxBytes: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			synth := testElevenLabs(server)
			if tc.maxBytes != 0 {
				synth.maxAudioBytes = tc.maxBytes
			}
			_, err := synth.Synthesize(context.Background(), "hello")
			if !errors.Is(err, ErrSynthesis) {
				t.Fatalf("error = %v, want ErrSynthesis", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked provider body: %v", err)
			}
		})
	}
}

func TestElevenLabsSynthesizeHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()

	synth := testElevenLabs(server)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := synth.Synthesize(ctx, "hello")
		done <- err
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		close(release)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Synthesize did not return after cancellation")
	}
}

func TestNewElevenLabsValidatesConfiguration(t *testing.T) {
	valid := ElevenLabsConfig{APIKey: "key", VoiceID: "voice", ModelID: "model"}
	for _, mutate := range []func(*ElevenLabsConfig){
		func(c *ElevenLabsConfig) { c.APIKey = "" },
		func(c *ElevenLabsConfig) { c.VoiceID = "" },
		func(c *ElevenLabsConfig) { c.ModelID = "" },
	} {
		cfg := valid
		mutate(&cfg)
		if _, err := NewElevenLabs(cfg); err == nil {
			t.Fatalf("NewElevenLabs(%+v) succeeded", cfg)
		}
	}
}

func TestValidateVoiceSettingsRejectsNonFiniteAndOutOfRangeValues(t *testing.T) {
	for _, value := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		settings := defaultVoiceSettings()
		settings.Stability = value
		if err := validateVoiceSettings(settings); err == nil {
			t.Fatalf("validateVoiceSettings(%v) succeeded", value)
		}
	}
}

func testElevenLabs(server *httptest.Server) *ElevenLabs {
	return &ElevenLabs{
		client:         server.Client(),
		baseURL:        server.URL,
		apiKey:         "secret",
		voiceID:        "voice-123",
		modelID:        "model-1",
		settings:       defaultVoiceSettings(),
		requestTimeout: time.Second,
		maxAudioBytes:  maxAudioBytes,
	}
}
