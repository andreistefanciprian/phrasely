package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// apiClient calls the private backend API over the internal network,
// forwarding a caller-supplied JWT — mirrors frontend/api.go's pattern.
type apiClient struct {
	baseURL string
	http    *http.Client
}

// apiStatusError preserves an unexpected backend status so tool wrappers can
// distinguish authentication failures from ordinary API errors.
type apiStatusError struct {
	StatusCode int
}

func (e *apiStatusError) Error() string {
	return fmt.Sprintf("API returned %d", e.StatusCode)
}

func isAPIAuthError(err error) bool {
	var statusErr *apiStatusError
	return errors.As(err, &statusErr) &&
		(statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden)
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// PhraseSummary mirrors backend/internal/db.PhraseSummary — lightweight projection
// returned by GET /api/v1/phrases/summary to reduce token usage in AI contexts.
type PhraseSummary struct {
	Phrase    string   `json:"phrase"`
	Headwords []string `json:"headwords"`
}

// AddPhraseRequest holds the fields needed to create a new phrase, mirroring
// backend/internal/db.CreatePhraseRequest.
type AddPhraseRequest struct {
	Phrase     string   `json:"phrase"`
	Headwords  []string `json:"headwords"`
	Note       string   `json:"note,omitempty"`
	SourceURLs []string `json:"source_urls,omitempty"`
}

// AddPhrase creates a new phrase for the authenticated user.
func (c *apiClient) AddPhrase(jwt string, in AddPhraseRequest) (PhraseSummary, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return PhraseSummary{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/phrases", bytes.NewReader(body))
	if err != nil {
		return PhraseSummary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return PhraseSummary{}, fmt.Errorf("add phrase: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return PhraseSummary{}, &apiStatusError{StatusCode: resp.StatusCode}
	}

	var phrase PhraseSummary
	if err := json.NewDecoder(resp.Body).Decode(&phrase); err != nil {
		return PhraseSummary{}, fmt.Errorf("decode phrase: %w", err)
	}
	return phrase, nil
}

// ListPhrasesSummary fetches a lightweight projection (phrase, headwords) of all
// phrases for the authenticated user. Used by the list_phrases MCP tool to minimise
// token usage — id, note and source_urls are omitted.
func (c *apiClient) ListPhrasesSummary(jwt, headword string) ([]PhraseSummary, error) {
	reqURL := c.baseURL + "/api/v1/phrases/summary"
	if headword != "" {
		reqURL += "?headword=" + url.QueryEscape(headword)
	}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list phrases summary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiStatusError{StatusCode: resp.StatusCode}
	}

	var summaries []PhraseSummary
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		return nil, fmt.Errorf("decode phrases summary: %w", err)
	}
	return summaries, nil
}

// GetRandomPhrases fetches count randomly selected phrases for the authenticated user.
func (c *apiClient) GetRandomPhrases(jwt string, count int) ([]PhraseSummary, error) {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/v1/phrases/random?count=%d", c.baseURL, count), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get random phrases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiStatusError{StatusCode: resp.StatusCode}
	}

	var summaries []PhraseSummary
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		return nil, fmt.Errorf("decode random phrases: %w", err)
	}
	return summaries, nil
}
