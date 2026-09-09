package embeddings

import (
	"github.com/andreistefanciprian/phrasely/internal/db"
	"testing"
)

func TestPhraseTextUsesCanonicalMeaningContextAndNote(t *testing.T) {
	p := db.Phrase{Phrase: "She stood up to scrutiny.", Headwords: []db.Headword{{Text: "stood up to scrutiny", Canonical: "stand up to scrutiny", Meaning: "remained convincing", SourceURL: "https://example.com"}, {Text: "she", Canonical: "she", Meaning: "the person mentioned"}}, Note: "Often used for claims."}
	got := PhraseText(p)
	want := "Expression: stand up to scrutiny\n" +
		"Meaning: remained convincing\n" +
		"Expression: she\n" +
		"Meaning: the person mentioned\n" +
		"Context: She stood up to scrutiny.\n" +
		"Usage: Often used for claims."
	if got != want {
		t.Fatalf("PhraseText() = %q, want %q", got, want)
	}
}

func TestPhraseTextOmitsEmptyNote(t *testing.T) {
	p := db.Phrase{Phrase: "She ran.", Headwords: []db.Headword{{Text: "ran", Canonical: "run", Meaning: "moved quickly"}}}
	want := "Expression: run\nMeaning: moved quickly\nContext: She ran."
	if got := PhraseText(p); got != want {
		t.Fatalf("PhraseText() = %q, want %q", got, want)
	}
}
