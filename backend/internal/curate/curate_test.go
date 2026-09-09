package curate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestCurateStructuredResponse(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"structured", `{"phrase":"She stood up to scrutiny (even then).","headwords":[{"text":"stood up to scrutiny","canonical":"stand up to scrutiny","meaning":"remained convincing","source_url":"https://www.merriam-webster.com/dictionary/scrutiny"}],"note":"Often used for claims."}`, true},
		{"invalid input", `{"valid_input":false}`, true},
		{"old strings", `{"phrase":"She ran.","headwords":["ran"]}`, false},
		{"missing meaning", `{"phrase":"She ran.","headwords":[{"text":"ran","canonical":"run"}]}`, false},
		{"blank sentence", `{"phrase":" ","headwords":[{"text":"ran","canonical":"run","meaning":"moved"}]}`, false},
		{"malformed", "not JSON", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": tc.content}}}})
			}))
			defer server.Close()
			config := openai.DefaultConfig("local-test")
			config.BaseURL = server.URL
			curator := Curator{client: openai.NewClientWithConfig(config), systemPrompt: "test"}
			got, err := curator.Curate(context.Background(), "test")
			if (err == nil) != tc.valid {
				t.Fatalf("got %+v, error %v", got, err)
			}
			if tc.name == "structured" && (got.Phrase != "She stood up to scrutiny (even then)." || got.Headwords[0].Meaning != "remained convincing") {
				t.Fatalf("lost structured content: %+v", got)
			}
			if tc.name == "invalid input" && (got.ValidInput || got.InvalidReason == "") {
				t.Fatalf("invalid input: %+v", got)
			}
		})
	}
}
