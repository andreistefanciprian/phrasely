// Package audio contains the server-side text-to-speech adapters and orchestration.
package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	elevenLabsBaseURL = "https://api.elevenlabs.io"
	outputFormat      = "mp3_44100_128"
	requestTimeout    = 30 * time.Second
	maxAudioBytes     = 10 << 20 // 10 MiB is generous for a spoken phrase.
)

// ErrSynthesis identifies failures returned by, or while validating, the
// speech provider. Callers can map it to their public upstream-error contract.
var ErrSynthesis = errors.New("speech synthesis failed")

// VoiceSettings are part of both the ElevenLabs request and the eventual audio
// cache identity. Keep these values explicit so a settings change cannot reuse
// audio generated with different characteristics.
type VoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
}

func defaultVoiceSettings() VoiceSettings {
	return VoiceSettings{
		Stability:       0.5,
		SimilarityBoost: 0.75,
		Style:           0,
		UseSpeakerBoost: true,
	}
}

// ElevenLabsConfig contains the fixed inputs used for every synthesis request.
type ElevenLabsConfig struct {
	APIKey  string
	VoiceID string
	ModelID string
}

// ElevenLabs synthesizes short phrase audio through ElevenLabs.
type ElevenLabs struct {
	client         *http.Client
	baseURL        string
	apiKey         string
	voiceID        string
	modelID        string
	settings       VoiceSettings
	requestTimeout time.Duration
	maxAudioBytes  int64
}

// NewElevenLabs constructs the production adapter. The HTTP timeout is also
// enforced per request so cancellation works even if the transport stalls.
func NewElevenLabs(cfg ElevenLabsConfig) (*ElevenLabs, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("elevenlabs api key is required")
	}
	if strings.TrimSpace(cfg.VoiceID) == "" {
		return nil, fmt.Errorf("elevenlabs voice id is required")
	}
	if strings.TrimSpace(cfg.ModelID) == "" {
		return nil, fmt.Errorf("elevenlabs model id is required")
	}
	settings := defaultVoiceSettings()
	if err := validateVoiceSettings(settings); err != nil {
		return nil, err
	}

	return &ElevenLabs{
		client:         &http.Client{Timeout: requestTimeout},
		baseURL:        elevenLabsBaseURL,
		apiKey:         cfg.APIKey,
		voiceID:        cfg.VoiceID,
		modelID:        cfg.ModelID,
		settings:       settings,
		requestTimeout: requestTimeout,
		maxAudioBytes:  maxAudioBytes,
	}, nil
}

func validateVoiceSettings(settings VoiceSettings) error {
	for name, value := range map[string]float64{
		"stability": settings.Stability, "similarity boost": settings.SimilarityBoost, "style": settings.Style,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("elevenlabs %s must be between 0 and 1", name)
		}
	}
	return nil
}

type textToSpeechRequest struct {
	Text          string        `json:"text"`
	ModelID       string        `json:"model_id"`
	VoiceSettings VoiceSettings `json:"voice_settings"`
}

// Synthesize converts text to an MP3. Returned bytes are complete and bounded;
// provider error bodies are intentionally not exposed to callers or logs.
func (e *ElevenLabs) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: text is empty", ErrSynthesis)
	}

	body, err := json.Marshal(textToSpeechRequest{Text: text, ModelID: e.modelID, VoiceSettings: e.settings})
	if err != nil {
		return nil, fmt.Errorf("encode elevenlabs request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, e.requestTimeout)
	defer cancel()

	endpoint, err := url.JoinPath(e.baseURL, "v1", "text-to-speech", e.voiceID)
	if err != nil {
		return nil, fmt.Errorf("build elevenlabs endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build elevenlabs request: %w", err)
	}
	query := req.URL.Query()
	query.Set("output_format", outputFormat)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request: %w", ErrSynthesis, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: provider returned status %d", ErrSynthesis, resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "audio/mpeg" {
		return nil, fmt.Errorf("%w: provider returned invalid content type", ErrSynthesis)
	}
	if resp.ContentLength > e.maxAudioBytes {
		return nil, fmt.Errorf("%w: provider response exceeds %d bytes", ErrSynthesis, e.maxAudioBytes)
	}

	audio, err := io.ReadAll(io.LimitReader(resp.Body, e.maxAudioBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrSynthesis, err)
	}
	if int64(len(audio)) > e.maxAudioBytes {
		return nil, fmt.Errorf("%w: provider response exceeds %d bytes", ErrSynthesis, e.maxAudioBytes)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("%w: provider returned empty audio", ErrSynthesis)
	}
	return audio, nil
}
