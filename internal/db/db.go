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

// User represents a registered user identified by email.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// MagicLinkToken is a single-use token tied to a user, valid for 15 minutes.
type MagicLinkToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Phrase represents a single phrase record as stored in the database.
type Phrase struct {
	ID         string    `json:"id"`
	Phrase     string    `json:"phrase"`
	Headwords  []string  `json:"headwords"`
	Note       string    `json:"note"`
	SourceURLs []string  `json:"source_urls"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreatePhraseRequest holds the fields needed to insert a new phrase.
type CreatePhraseRequest struct {
	Phrase     string   `json:"phrase"`
	Headwords  []string `json:"headwords"`
	Note       string   `json:"note"`
	SourceURLs []string `json:"source_urls"`
}

// UpdatePhraseRequest holds the fields that may be updated on an existing phrase.
// Pointer fields allow partial updates: nil means "leave this field unchanged".
type UpdatePhraseRequest struct {
	Phrase     *string  `json:"phrase"`
	Headwords  []string `json:"headwords"` // nil = leave unchanged; when provided, must contain at least one headword
	Note       *string  `json:"note"`
	SourceURLs []string `json:"source_urls"` // nil = leave unchanged; [] = clear all URLs
}

// Store is the interface all database implementations must satisfy.
// Methods for each domain (phrases, collections, etc.) will be added here as we build them.
type Store interface {
	Close()

	// Phrase methods
	CreatePhrase(ctx context.Context, req CreatePhraseRequest) (*Phrase, error)
	ListPhrases(ctx context.Context, headword string) ([]Phrase, error)
	GetPhrase(ctx context.Context, id string) (*Phrase, error)
	DeletePhrase(ctx context.Context, id string) error
	UpdatePhrase(ctx context.Context, id string, req UpdatePhraseRequest) (*Phrase, error)

	// Auth methods
	UpsertUser(ctx context.Context, email string) (*User, error)
	CreateMagicLinkToken(ctx context.Context, userID string, expiresAt time.Time) (*MagicLinkToken, error)
	GetMagicLinkToken(ctx context.Context, token string) (*MagicLinkToken, error)
	MarkTokenUsed(ctx context.Context, tokenID string) error
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
// If headword is non-empty, results are filtered to phrases where any element matches (case-insensitive).
func (s *PostgresStore) ListPhrases(ctx context.Context, headword string) ([]Phrase, error) {
	query := `SELECT id, phrase, headwords, note, source_urls, created_at, updated_at FROM phrases`
	args := []any{}

	if headword != "" {
		// array_to_string flattens headwords into a single string so the trigram GIN index is used.
		// This makes ILIKE '%term%' fast even as the table grows.
		query += ` WHERE headwords_text(headwords) ILIKE $1`
		args = append(args, "%"+headword+"%")
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
		if err := rows.Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
		`SELECT id, phrase, headwords, note, source_urls, created_at, updated_at FROM phrases WHERE id = $1`, id,
	).Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
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
// For headwords/source_urls: nil = leave unchanged; [] = clear.
// Returns ErrNotFound if no row matches the given ID.
func (s *PostgresStore) UpdatePhrase(ctx context.Context, id string, req UpdatePhraseRequest) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`UPDATE phrases
		 SET phrase      = COALESCE($1, phrase),
		     headwords   = CASE WHEN $2::text[] IS NOT NULL THEN $2 ELSE headwords END,
		     note        = COALESCE($3, note),
		     source_urls = CASE WHEN $4::text[] IS NOT NULL THEN $4 ELSE source_urls END
		 WHERE id = $5
		 RETURNING id, phrase, headwords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Headwords, req.Note, req.SourceURLs, id,
	).Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
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
	if req.Headwords == nil {
		req.Headwords = []string{}
	}
	if req.SourceURLs == nil {
		req.SourceURLs = []string{}
	}
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO phrases (phrase, headwords, note, source_urls)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, phrase, headwords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Headwords, req.Note, req.SourceURLs,
	).Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create phrase: %w", err)
	}
	return &p, nil
}

// UpsertUser finds a user by email or creates one if they don't exist yet.
// Uses DO NOTHING on conflict to avoid writing a new row version for existing users.
func (s *PostgresStore) UpsertUser(ctx context.Context, email string) (*User, error) {
	var u User

	// Try insert first; DO NOTHING avoids unnecessary write amplification on conflict.
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO users (email)
		 VALUES ($1)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id, email, created_at`,
		email,
	).Scan(&u.ID, &u.Email, &u.CreatedAt)

	if err == nil {
		return &u, nil // new user created
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	// User already exists — fetch them without touching the row.
	err = s.Pool.QueryRow(ctx,
		`SELECT id, email, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("fetch existing user: %w", err)
	}
	return &u, nil
}

// CreateMagicLinkToken inserts a new single-use token for the given user.
func (s *PostgresStore) CreateMagicLinkToken(ctx context.Context, userID string, expiresAt time.Time) (*MagicLinkToken, error) {
	var t MagicLinkToken
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO magic_link_tokens (user_id, expires_at)
		 VALUES ($1, $2)
		 RETURNING id, user_id, token, expires_at, used_at, created_at`,
		userID, expiresAt,
	).Scan(&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create magic link token: %w", err)
	}
	return &t, nil
}

// GetMagicLinkToken fetches a token record by its token value.
// Returns ErrNotFound if no match.
func (s *PostgresStore) GetMagicLinkToken(ctx context.Context, token string) (*MagicLinkToken, error) {
	var t MagicLinkToken
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_id, token, expires_at, used_at, created_at
		 FROM magic_link_tokens WHERE token = $1`, token,
	).Scan(&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get magic link token: %w", err)
	}
	return &t, nil
}

// MarkTokenUsed atomically consumes the token in a single UPDATE.
// The WHERE clause guards against race conditions: only succeeds if the token
// is still unused and not yet expired. Returns ErrNotFound if another request
// already consumed it, or if it expired between SELECT and UPDATE.
func (s *PostgresStore) MarkTokenUsed(ctx context.Context, tokenID string) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE magic_link_tokens
		 SET used_at = NOW()
		 WHERE id = $1 AND used_at IS NULL AND expires_at > NOW()`,
		tokenID,
	)
	if err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
