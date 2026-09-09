package db

import (
	"testing"
)

func TestValidateHeadwords(t *testing.T) {
	good := Headword{Text: "stood up to scrutiny", Canonical: "stand up to scrutiny", Meaning: "remained convincing"}
	for _, tc := range []struct {
		name  string
		words []Headword
		valid bool
	}{
		{"multiple", []Headword{good, {Text: "disparaging", Canonical: "disparage", Meaning: "expressing contempt", SourceURL: "https://www.merriam-webster.com/dictionary/disparage"}}, true},
		{"missing", nil, false}, {"empty", []Headword{}, false},
		{"blank text", []Headword{{Text: " ", Canonical: "a", Meaning: "b"}}, false},
		{"missing canonical", []Headword{{Text: "ran", Meaning: "moved quickly"}}, false},
		{"missing meaning", []Headword{{Text: "ran", Canonical: "run"}}, false},
		{"unsafe URL", []Headword{{Text: "ran", Canonical: "run", Meaning: "moved quickly", SourceURL: "javascript:alert(1)"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateHeadwords(tc.words); (err == nil) != tc.valid {
				t.Fatalf("error=%v valid=%v", err, tc.valid)
			}
		})
	}
}
