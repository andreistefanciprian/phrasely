package main

import (
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
