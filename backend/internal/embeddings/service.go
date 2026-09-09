package embeddings

import (
	"context"
	"fmt"
	"strings"

	"github.com/andreistefanciprian/phrasely/internal/db"
	openai "github.com/sashabaranov/go-openai"
)

// Service generates text embeddings via OpenAI's text-embedding-3-small model (1536 dims).
type Service struct {
	client *openai.Client
}

func New(apiKey string) *Service {
	return &Service{client: openai.NewClient(apiKey)}
}

// PhraseText builds the semantic document for a phrase. The canonical form
// identifies the expression while the meaning, sentence, and note preserve its
// contextual use. Text is omitted as a separate field because it already occurs
// naturally in the sentence.
func PhraseText(p db.Phrase) string {
	parts := make([]string, 0, len(p.Headwords)*2+2)
	for _, w := range p.Headwords {
		parts = append(parts,
			"Expression: "+w.Canonical,
			"Meaning: "+w.Meaning,
		)
	}
	parts = append(parts, "Context: "+p.Phrase)
	if p.Note != "" {
		parts = append(parts, "Usage: "+p.Note)
	}
	return strings.Join(parts, "\n")
}

// Embed returns a 1536-dimensional vector for the given text.
func (s *Service) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := s.client.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{text},
		Model: openai.SmallEmbedding3,
	})
	if err != nil {
		return nil, fmt.Errorf("openai embeddings: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embeddings: empty response")
	}
	return resp.Data[0].Embedding, nil
}
