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

// PhraseText builds the string to embed for a phrase, combining headwords,
// the example sentence, and the usage note for maximum semantic richness.
func PhraseText(p db.Phrase) string {
	parts := []string{strings.Join(p.Headwords, ", "), p.Phrase}
	if p.Note != "" {
		parts = append(parts, p.Note)
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
