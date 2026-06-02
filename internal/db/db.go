package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested record does not exist in the database.
var ErrNotFound = errors.New("not found")

// Phrase represents a single phrase record as stored in the database.
type Phrase struct {
	ID         string    `json:"id"`
	Phrase     string    `json:"phrase"`
	Keywords   []string  `json:"keywords"`
	Note       string    `json:"note"`
	SourceURLs []string  `json:"source_urls"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreatePhraseRequest holds the fields needed to insert a new phrase.
type CreatePhraseRequest struct {
	Phrase     string   `json:"phrase"`
	Keywords   []string `json:"keywords"`
	Note       string   `json:"note"`
	SourceURLs []string `json:"source_urls"`
}

// UpdatePhraseRequest holds the fields that may be updated on an existing phrase.
// Pointer fields allow partial updates: nil means "leave this field unchanged".
type UpdatePhraseRequest struct {
	Phrase     *string  `json:"phrase"`
	Keywords   []string `json:"keywords"` // nil = leave unchanged; when provided, must contain at least one keyword
	Note       *string  `json:"note"`
	SourceURLs []string `json:"source_urls"` // nil = leave unchanged; [] = clear all URLs
}

// Store is the interface all database implementations must satisfy.
// Methods for each domain (phrases, collections, etc.) will be added here as we build them.
type Store interface {
	Close()
	CreatePhrase(ctx context.Context, req CreatePhraseRequest) (*Phrase, error)
	// ListPhrases returns all phrases, optionally filtered by keyword (case-insensitive partial match).
	ListPhrases(ctx context.Context, keyword string) ([]Phrase, error)
	// GetPhrase returns a single phrase by ID. Returns ErrNotFound if no match.
	GetPhrase(ctx context.Context, id string) (*Phrase, error)
	// DeletePhrase removes a phrase by ID. Returns ErrNotFound if no match.
	DeletePhrase(ctx context.Context, id string) error
	// UpdatePhrase applies a partial update to an existing phrase. Returns ErrNotFound if no match.
	UpdatePhrase(ctx context.Context, id string, req UpdatePhraseRequest) (*Phrase, error)
}

type PostgresStore struct {
	Pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &PostgresStore{Pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.Pool.Close()
}

// ListPhrases returns all phrases ordered by creation date, newest first.
// If keyword is non-empty, results are filtered to phrases where any element matches (case-insensitive).
func (s *PostgresStore) ListPhrases(ctx context.Context, keyword string) ([]Phrase, error) {
	query := `SELECT id, phrase, keywords, note, source_urls, created_at, updated_at FROM phrases`
	args := []any{}

	if keyword != "" {
		// array_to_string flattens keywords into a single string so the trigram GIN index is used.
		// This makes ILIKE '%term%' fast even as the table grows.
		query += ` WHERE keywords_text(keywords) ILIKE $1`
		args = append(args, "%"+keyword+"%")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list phrases: %w", err)
	}
	defer rows.Close()

	// Initialise as empty slice so the JSON response is [] not null when there are no results
	phrases := []Phrase{}
	for rows.Next() {
		var p Phrase
		if err := rows.Scan(&p.ID, &p.Phrase, &p.Keywords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan phrase: %w", err)
		}
		phrases = append(phrases, p)
	}
	return phrases, rows.Err()
}

// GetPhrase fetches a single phrase by its UUID. Returns ErrNotFound if no row matches.
func (s *PostgresStore) GetPhrase(ctx context.Context, id string) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`SELECT id, phrase, keywords, note, source_urls, created_at, updated_at FROM phrases WHERE id = $1`, id,
	).Scan(&p.ID, &p.Phrase, &p.Keywords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get phrase: %w", err)
	}
	return &p, nil
}

// DeletePhrase removes a phrase by ID. Returns ErrNotFound if no row was deleted.
func (s *PostgresStore) DeletePhrase(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM phrases WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete phrase: %w", err)
	}
	// RowsAffected() == 0 means no row matched that ID — treat as not found
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePhrase applies a partial update using COALESCE: nil fields keep their current DB value.
// For keywords/source_urls: nil = leave unchanged; [] = clear.
// Returns ErrNotFound if no row matches the given ID.
func (s *PostgresStore) UpdatePhrase(ctx context.Context, id string, req UpdatePhraseRequest) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`UPDATE phrases
		 SET phrase      = COALESCE($1, phrase),
		     keywords    = CASE WHEN $2::text[] IS NOT NULL THEN $2 ELSE keywords END,
		     note        = COALESCE($3, note),
		     source_urls = CASE WHEN $4::text[] IS NOT NULL THEN $4 ELSE source_urls END
		 WHERE id = $5
		 RETURNING id, phrase, keywords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Keywords, req.Note, req.SourceURLs, id,
	).Scan(&p.ID, &p.Phrase, &p.Keywords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update phrase: %w", err)
	}
	return &p, nil
}

// CreatePhrase inserts a new phrase and returns the full record including DB-generated fields.
func (s *PostgresStore) CreatePhrase(ctx context.Context, req CreatePhraseRequest) (*Phrase, error) {
	if req.Keywords == nil {
		req.Keywords = []string{}
	}
	if req.SourceURLs == nil {
		req.SourceURLs = []string{}
	}
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO phrases (phrase, keywords, note, source_urls)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, phrase, keywords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Keywords, req.Note, req.SourceURLs,
	).Scan(&p.ID, &p.Phrase, &p.Keywords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create phrase: %w", err)
	}
	return &p, nil
}
