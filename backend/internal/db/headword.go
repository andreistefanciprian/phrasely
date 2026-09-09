package db

import (
	"fmt"
	"net/url"
	"strings"
)

// Headword records one expression in this sentence and its context-specific gloss.
// Canonical groups grammatical variants; it does not identify a sense.
type Headword struct {
	Text      string `json:"text"`
	Canonical string `json:"canonical"`
	Meaning   string `json:"meaning"`
	SourceURL string `json:"source_url,omitempty"`
}

func ValidateHeadwords(words []Headword) error {
	if len(words) == 0 {
		return fmt.Errorf("at least one headword is required")
	}
	for i, w := range words {
		if strings.TrimSpace(w.Text) == "" || strings.TrimSpace(w.Canonical) == "" || strings.TrimSpace(w.Meaning) == "" {
			return fmt.Errorf("headword %d requires text, canonical, and meaning", i+1)
		}
		if w.SourceURL != "" {
			u, err := url.Parse(w.SourceURL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				return fmt.Errorf("headword %d source_url must be an absolute HTTP(S) URL", i+1)
			}
		}
	}
	return nil
}
