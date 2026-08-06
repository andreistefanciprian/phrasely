package curate

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// Evals call the real OpenAI API against backend/internal/curate/system_prompt.txt
// to catch behavioral regressions that unit tests can't (e.g. prompt edits that
// silently break a rule). They're gated behind RUN_EVALS so `go test ./...` and
// `task test` never hit the network or cost money by default.
//
// Run with:
//   OPENAI_API_KEY=sk-... RUN_EVALS=1 go test ./internal/curate/ -run TestEval -v

type golden struct {
	name  string
	input string
	check func(t *testing.T, got *CuratedPhrase)
}

var goldens = []golden{
	{
		name:  "?? placeholder is replaced with the meaning",
		input: "He was being cagey ?? about the decision.",
		check: func(t *testing.T, got *CuratedPhrase) {
			if !got.ValidInput {
				t.Fatalf("valid_input = false, want true (invalid_reason: %q)", got.InvalidReason)
			}
			if strings.Contains(got.Phrase, "??") {
				t.Errorf("phrase still contains the literal \"??\" placeholder: %q", got.Phrase)
			}
			if len(got.Headwords) != 1 {
				t.Fatalf("headwords = %v, want exactly 1", got.Headwords)
			}
			if headword := strings.ToLower(got.Headwords[0]); headword != "cagey" {
				t.Errorf("headwords[0] = %q, want \"cagey\"", got.Headwords[0])
			}
			if !strings.Contains(strings.ToLower(got.Phrase), "cagey") {
				t.Errorf("phrase does not contain the headword \"cagey\": %q", got.Phrase)
			}
			if !regexp.MustCompile(`cagey \([^)]+\)`).MatchString(got.Phrase) {
				t.Errorf("phrase does not have a meaning in parentheses right after \"cagey\": %q", got.Phrase)
			}
			if len(got.SourceURLs) != 1 || got.SourceURLs[0] != "https://www.merriam-webster.com/dictionary/cagey" {
				t.Errorf("source_urls = %v, want [\"https://www.merriam-webster.com/dictionary/cagey\"]", got.SourceURLs)
			}
		},
	},
	{
		name:  "phrase and note contain no Markdown formatting",
		input: "he was cower away from the fight",
		check: func(t *testing.T, got *CuratedPhrase) {
			if !got.ValidInput {
				t.Fatalf("valid_input = false, want true (invalid_reason: %q)", got.InvalidReason)
			}
			markdown := regexp.MustCompile(`\*\*|__|` + "`" + `|^#{1,6}\s|\[[^\]]*\]\(`)
			if markdown.MatchString(got.Phrase) {
				t.Errorf("phrase contains Markdown formatting: %q", got.Phrase)
			}
			if markdown.MatchString(got.Note) {
				t.Errorf("note contains Markdown formatting: %q", got.Note)
			}
		},
	},
}

func TestEval(t *testing.T) {
	if os.Getenv("RUN_EVALS") == "" {
		t.Skip("skipping evals: set RUN_EVALS=1 (and OPENAI_API_KEY) to run")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Fatal("OPENAI_API_KEY is required to run evals")
	}

	promptBytes, err := os.ReadFile("system_prompt.txt")
	if err != nil {
		t.Fatalf("read system prompt: %v", err)
	}
	curator := &Curator{
		client:       openai.NewClient(apiKey),
		systemPrompt: strings.TrimSpace(string(promptBytes)),
	}

	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) {
			got, err := curator.Curate(context.Background(), g.input)
			if err != nil {
				t.Fatalf("Curate(%q): %v", g.input, err)
			}
			t.Logf("output: %+v", got)
			g.check(t, got)
		})
	}
}
