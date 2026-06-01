package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Phrase represents a single phrase record as stored in the database.
type Phrase struct {
	ID        string    `json:"id"`
	Phrase    string    `json:"phrase"`
	Keyword   string    `json:"keyword"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreatePhraseRequest holds the fields needed to insert a new phrase.
type CreatePhraseRequest struct {
	Phrase  string `json:"phrase"`
	Keyword string `json:"keyword"`
	Note    string `json:"note"`
}

// Store is the interface all database implementations must satisfy.
// Methods for each domain (phrases, collections, etc.) will be added here as we build them.
type Store interface {
	Close()
	CreatePhrase(ctx context.Context, req CreatePhraseRequest) (*Phrase, error)
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

// CreatePhrase inserts a new phrase and returns the full record including DB-generated fields.
func (s *PostgresStore) CreatePhrase(ctx context.Context, req CreatePhraseRequest) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO phrases (phrase, keyword, note)
		 VALUES ($1, $2, $3)
		 RETURNING id, phrase, keyword, note, created_at, updated_at`,
		req.Phrase, req.Keyword, req.Note,
	).Scan(&p.ID, &p.Phrase, &p.Keyword, &p.Note, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create phrase: %w", err)
	}
	return &p, nil
}
