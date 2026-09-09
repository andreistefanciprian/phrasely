package main

import (
	"net/url"
	"slices"
	"strings"
)

type Headword struct {
	Text      string `json:"text" jsonschema:"Expression in its actual grammatical form in the sentence, e.g. stood up to scrutiny. Keep fixed particles and prepositions."`
	Canonical string `json:"canonical" jsonschema:"Normalized grouping form, e.g. stand up to scrutiny or disparage for disparaging. Use linguistic judgment, never naive suffix stripping. Equality identifies grammatical variants, not identical senses."`
	Meaning   string `json:"meaning" jsonschema:"Short plain-English gloss specific to this sentence. Required; rendered inline by the UI."`
	SourceURL string `json:"source_url,omitempty" jsonschema:"Optional absolute HTTP(S) Merriam-Webster dictionary URL for this expression's actual dictionary entry. Omit if uncertain."`
}

func canonicalWords(words []Headword) []string {
	result := make([]string, 0, len(words))
	for _, w := range words {
		result = append(result, strings.ToLower(strings.TrimSpace(w.Canonical)))
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func validSourceURL(value string) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}
