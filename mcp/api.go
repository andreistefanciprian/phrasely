package main

import (
	"bytes"
	"encoding/json"
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

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Phrase mirrors backend/internal/db.Phrase. mcp has no direct DB access and
// does not share internal/ packages with backend, so the shape is duplicated here.
type Phrase struct {
	ID         string   `json:"id"`
	Phrase     string   `json:"phrase"`
	Headwords  []string `json:"headwords"`
	Note       string   `json:"note"`
	SourceURLs []string `json:"source_urls"`
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
func (c *apiClient) AddPhrase(jwt string, in AddPhraseRequest) (Phrase, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return Phrase{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/phrases", bytes.NewReader(body))
	if err != nil {
		return Phrase{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Phrase{}, fmt.Errorf("add phrase: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return Phrase{}, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var phrase Phrase
	if err := json.NewDecoder(resp.Body).Decode(&phrase); err != nil {
		return Phrase{}, fmt.Errorf("decode phrase: %w", err)
	}
	return phrase, nil
}

// ListPhrases fetches the authenticated user's phrases, optionally filtered by headword.
func (c *apiClient) ListPhrases(jwt, headword string) ([]Phrase, error) {
	reqURL := c.baseURL + "/api/v1/phrases"
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
		return nil, fmt.Errorf("list phrases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var phrases []Phrase
	if err := json.NewDecoder(resp.Body).Decode(&phrases); err != nil {
		return nil, fmt.Errorf("decode phrases: %w", err)
	}
	return phrases, nil
}
