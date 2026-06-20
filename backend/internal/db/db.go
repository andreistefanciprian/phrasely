package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
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

// PhraseSummary is a lightweight projection of a phrase record containing only
// the fields needed for listing — used by the MCP list_phrases tool to avoid
// sending id, note and source_urls into the AI model's context window.
type PhraseSummary struct {
	Phrase    string   `json:"phrase"`
	Headwords []string `json:"headwords"`
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

// OAuthClient is a registered third-party app (e.g. ChatGPT) that has been
// granted access via Dynamic Client Registration (RFC 7591).
// No client_secret: this is a public client — authentication is via PKCE only.
type OAuthClient struct {
	ID           string    `json:"id"`
	RedirectURIs []string  `json:"redirect_uris"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthAuthorizationCode is a short-lived (~60s), single-use code issued after
// the user approves consent on the /authorize screen. The client exchanges it
// for an access token at POST /token, proving it holds the code_verifier that
// matches the code_challenge stored here (PKCE S256 method).
type OAuthAuthorizationCode struct {
	ID            string     `json:"id"`
	Code          string     `json:"code"`           // opaque value sent in redirect URL
	ClientID      string     `json:"client_id"`
	UserID        string     `json:"user_id"`
	RedirectURI   string     `json:"redirect_uri"`   // pinned; re-validated at /token
	CodeChallenge string     `json:"code_challenge"` // SHA256(code_verifier), base64url
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at"`        // nil = unused; set atomically on consume
	CreatedAt     time.Time  `json:"created_at"`
}

// OAuthRefreshToken is a long-lived token that lets the client obtain new access
// tokens without re-prompting the user. Rotated on every use: old token is
// revoked, new one issued. Stored in DB so it can be revoked (unlike JWTs).
type OAuthRefreshToken struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`      // opaque value sent to client
	ClientID  string     `json:"client_id"`
	UserID    string     `json:"user_id"`
	RevokedAt *time.Time `json:"revoked_at"` // nil = active; set on rotation or revocation
	CreatedAt time.Time  `json:"created_at"`
}

// CreateAuthCodeRequest holds the fields needed to issue an authorization code.
type CreateAuthCodeRequest struct {
	ClientID      string
	UserID        string
	RedirectURI   string
	CodeChallenge string
	ExpiresAt     time.Time
}

// CreateRefreshTokenRequest holds the fields needed to issue a refresh token.
type CreateRefreshTokenRequest struct {
	ClientID string
	UserID   string
}

// Store is the interface all database implementations must satisfy.
// Methods for each domain (phrases, collections, etc.) will be added here as we build them.
type Store interface {
	Close()

	// Phrase methods — all scoped to the owning user.
	CreatePhrase(ctx context.Context, userID string, req CreatePhraseRequest) (*Phrase, error)
	ListPhrases(ctx context.Context, userID string, headword string) ([]Phrase, error)
	ListPhrasesSummary(ctx context.Context, userID string, headword string) ([]PhraseSummary, error)
	GetRandomPhrases(ctx context.Context, userID string, count int) ([]PhraseSummary, error)
	GetPhrase(ctx context.Context, userID string, id string) (*Phrase, error)
	DeletePhrase(ctx context.Context, userID string, id string) error
	UpdatePhrase(ctx context.Context, userID string, id string, req UpdatePhraseRequest) (*Phrase, error)

	// Embedding methods — vector search powered by pgvector.
	SetPhraseEmbedding(ctx context.Context, id string, embedding []float32) error
	ListPhrasesWithoutEmbedding(ctx context.Context) ([]Phrase, error)
	SearchPhrasesBySimilarity(ctx context.Context, userID string, embedding []float32, limit int) ([]Phrase, error)

	// Magic-link auth methods
	UpsertUser(ctx context.Context, email string) (*User, error)
	CreateMagicLinkToken(ctx context.Context, userID string, expiresAt time.Time) (*MagicLinkToken, error)
	GetMagicLinkToken(ctx context.Context, token string) (*MagicLinkToken, error)
	MarkTokenUsed(ctx context.Context, tokenID string) error

	// OAuth 2.1 methods
	//
	// Client registration (PR 23: /internal/oauth/register)
	CreateOAuthClient(ctx context.Context, redirectURIs []string) (*OAuthClient, error)
	GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error)
	//
	// Authorization codes (PR 25: /internal/oauth/authorize)
	CreateAuthorizationCode(ctx context.Context, req CreateAuthCodeRequest) (*OAuthAuthorizationCode, error)
	// ConsumeAuthorizationCode atomically marks the code used and returns it.
	// Returns ErrNotFound if the code doesn't exist, is already used, has expired,
	// or belongs to a different client — deliberately indistinct to avoid leaking state.
	ConsumeAuthorizationCode(ctx context.Context, code, clientID string) (*OAuthAuthorizationCode, error)
	//
	// Refresh tokens (PR 29: token rotation)
	CreateRefreshToken(ctx context.Context, req CreateRefreshTokenRequest) (*OAuthRefreshToken, error)
	// ConsumeRefreshToken atomically revokes the token and returns it.
	// clientID must match the token's owner — prevents one client consuming another's token.
	// Returns ErrNotFound if the token doesn't exist, is already revoked, or clientID mismatches.
	ConsumeRefreshToken(ctx context.Context, token, clientID string) (*OAuthRefreshToken, error)
	// RevokeRefreshTokens revokes all active refresh tokens for the given user+client pair.
	// Called when the user explicitly denies a consent request to ensure the client
	// loses access even if it already holds tokens from a prior authorization.
	RevokeRefreshTokens(ctx context.Context, userID, clientID string) error
}

type PostgresStore struct {
	Pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	// Register the pgvector type codec on every new connection so pgx can
	// encode/decode vector(...) columns and parameters.
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
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

// ListPhrases returns phrases owned by userID, newest first.
// If headword is non-empty, results are filtered by case-insensitive partial match.
func (s *PostgresStore) ListPhrases(ctx context.Context, userID string, headword string) ([]Phrase, error) {
	query := `SELECT id, phrase, headwords, note, source_urls, created_at, updated_at
	          FROM phrases WHERE user_id = $1`
	args := []any{userID}

	if headword != "" {
		query += ` AND headwords_text(headwords) ILIKE $2`
		args = append(args, "%"+headword+"%")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list phrases: %w", err)
	}
	defer rows.Close()

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

// ListPhrasesSummary returns a lightweight projection (phrase, headwords) for all
// phrases owned by userID, newest first. Used by the MCP list_phrases tool.
// If headword is non-empty, results are filtered by case-insensitive partial match.
func (s *PostgresStore) ListPhrasesSummary(ctx context.Context, userID string, headword string) ([]PhraseSummary, error) {
	query := `SELECT phrase, headwords FROM phrases WHERE user_id = $1`
	args := []any{userID}

	if headword != "" {
		query += ` AND headwords_text(headwords) ILIKE $2`
		args = append(args, "%"+headword+"%")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list phrases summary: %w", err)
	}
	defer rows.Close()

	summaries := []PhraseSummary{}
	for rows.Next() {
		var p PhraseSummary
		if err := rows.Scan(&p.Phrase, &p.Headwords); err != nil {
			return nil, fmt.Errorf("scan phrase summary: %w", err)
		}
		summaries = append(summaries, p)
	}
	return summaries, rows.Err()
}

// GetRandomPhrases returns count randomly selected phrases (phrase, headwords) for userID.
func (s *PostgresStore) GetRandomPhrases(ctx context.Context, userID string, count int) ([]PhraseSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT phrase, headwords FROM phrases WHERE user_id = $1 ORDER BY RANDOM() LIMIT $2`,
		userID, count,
	)
	if err != nil {
		return nil, fmt.Errorf("get random phrases: %w", err)
	}
	defer rows.Close()

	summaries := []PhraseSummary{}
	for rows.Next() {
		var p PhraseSummary
		if err := rows.Scan(&p.Phrase, &p.Headwords); err != nil {
			return nil, fmt.Errorf("scan phrase summary: %w", err)
		}
		summaries = append(summaries, p)
	}
	return summaries, rows.Err()
}

// GetPhrase fetches a phrase by ID scoped to userID. Returns ErrNotFound if no match.
func (s *PostgresStore) GetPhrase(ctx context.Context, userID string, id string) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`SELECT id, phrase, headwords, note, source_urls, created_at, updated_at
		 FROM phrases WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get phrase: %w", err)
	}
	return &p, nil
}

// DeletePhrase removes a phrase scoped to userID. Returns ErrNotFound if no row was deleted.
func (s *PostgresStore) DeletePhrase(ctx context.Context, userID string, id string) error {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM phrases WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete phrase: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePhrase applies a partial update scoped to userID. Returns ErrNotFound if no match.
func (s *PostgresStore) UpdatePhrase(ctx context.Context, userID string, id string, req UpdatePhraseRequest) (*Phrase, error) {
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`UPDATE phrases
		 SET phrase      = COALESCE($1, phrase),
		     headwords   = CASE WHEN $2::text[] IS NOT NULL THEN $2 ELSE headwords END,
		     note        = COALESCE($3, note),
		     source_urls = CASE WHEN $4::text[] IS NOT NULL THEN $4 ELSE source_urls END
		 WHERE id = $5 AND user_id = $6
		 RETURNING id, phrase, headwords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Headwords, req.Note, req.SourceURLs, id, userID,
	).Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update phrase: %w", err)
	}
	return &p, nil
}

// CreatePhrase inserts a phrase owned by userID and returns the full record.
func (s *PostgresStore) CreatePhrase(ctx context.Context, userID string, req CreatePhraseRequest) (*Phrase, error) {
	if req.Headwords == nil {
		req.Headwords = []string{}
	}
	if req.SourceURLs == nil {
		req.SourceURLs = []string{}
	}
	var p Phrase
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO phrases (phrase, headwords, note, source_urls, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, phrase, headwords, note, source_urls, created_at, updated_at`,
		req.Phrase, req.Headwords, req.Note, req.SourceURLs, userID,
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

// ── OAuth store methods ───────────────────────────────────────────────────────

// CreateOAuthClient inserts a new public OAuth client with the given redirect URIs.
func (s *PostgresStore) CreateOAuthClient(ctx context.Context, redirectURIs []string) (*OAuthClient, error) {
	var c OAuthClient
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO oauth_clients (redirect_uris)
		 VALUES ($1)
		 RETURNING id, redirect_uris, created_at`,
		redirectURIs,
	).Scan(&c.ID, &c.RedirectURIs, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}
	return &c, nil
}

// GetOAuthClient fetches a client by ID. Returns ErrNotFound if no match.
func (s *PostgresStore) GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error) {
	var c OAuthClient
	err := s.Pool.QueryRow(ctx,
		`SELECT id, redirect_uris, created_at FROM oauth_clients WHERE id = $1`, clientID,
	).Scan(&c.ID, &c.RedirectURIs, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth client: %w", err)
	}
	return &c, nil
}

// CreateAuthorizationCode inserts a new single-use PKCE authorization code.
func (s *PostgresStore) CreateAuthorizationCode(ctx context.Context, req CreateAuthCodeRequest) (*OAuthAuthorizationCode, error) {
	var a OAuthAuthorizationCode
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO oauth_authorization_codes
		     (client_id, user_id, redirect_uri, code_challenge, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, code, client_id, user_id, redirect_uri, code_challenge, expires_at, used_at, created_at`,
		req.ClientID, req.UserID, req.RedirectURI, req.CodeChallenge, req.ExpiresAt,
	).Scan(&a.ID, &a.Code, &a.ClientID, &a.UserID, &a.RedirectURI, &a.CodeChallenge, &a.ExpiresAt, &a.UsedAt, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create authorization code: %w", err)
	}
	return &a, nil
}

// ConsumeAuthorizationCode atomically marks a code used and returns it.
// The single UPDATE guards against:
//   - replay attacks (used_at IS NULL)
//   - expired codes (expires_at > NOW())
//   - client confusion (client_id = $2), preventing one client redeeming another's code
//
// All failure cases return ErrNotFound — deliberately indistinct to avoid leaking
// whether the code existed, was already used, or just expired.
func (s *PostgresStore) ConsumeAuthorizationCode(ctx context.Context, code, clientID string) (*OAuthAuthorizationCode, error) {
	var a OAuthAuthorizationCode
	err := s.Pool.QueryRow(ctx,
		`UPDATE oauth_authorization_codes
		 SET used_at = NOW()
		 WHERE code = $1
		   AND client_id = $2
		   AND used_at IS NULL
		   AND expires_at > NOW()
		 RETURNING id, code, client_id, user_id, redirect_uri, code_challenge, expires_at, used_at, created_at`,
		code, clientID,
	).Scan(&a.ID, &a.Code, &a.ClientID, &a.UserID, &a.RedirectURI, &a.CodeChallenge, &a.ExpiresAt, &a.UsedAt, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume authorization code: %w", err)
	}
	return &a, nil
}

// CreateRefreshToken inserts a new active refresh token for the given client + user.
func (s *PostgresStore) CreateRefreshToken(ctx context.Context, req CreateRefreshTokenRequest) (*OAuthRefreshToken, error) {
	var t OAuthRefreshToken
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO oauth_refresh_tokens (client_id, user_id)
		 VALUES ($1, $2)
		 RETURNING id, token, client_id, user_id, revoked_at, created_at`,
		req.ClientID, req.UserID,
	).Scan(&t.ID, &t.Token, &t.ClientID, &t.UserID, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &t, nil
}

// ConsumeRefreshToken atomically revokes a refresh token and returns it.
// Requires clientID to match — prevents a client from consuming a token it did not receive.
// The WHERE clause (revoked_at IS NULL) ensures each token can only be rotated once.
// If the same token arrives twice (replay), ErrNotFound is returned — the caller
// should treat this as a potential token theft and revoke all tokens for that client.
func (s *PostgresStore) ConsumeRefreshToken(ctx context.Context, token, clientID string) (*OAuthRefreshToken, error) {
	var t OAuthRefreshToken
	err := s.Pool.QueryRow(ctx,
		`UPDATE oauth_refresh_tokens
		 SET revoked_at = NOW()
		 WHERE token = $1
		   AND client_id = $2
		   AND revoked_at IS NULL
		 RETURNING id, token, client_id, user_id, revoked_at, created_at`,
		token, clientID,
	).Scan(&t.ID, &t.Token, &t.ClientID, &t.UserID, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}
	return &t, nil
}

// RevokeRefreshTokens revokes all active refresh tokens for a given user+client pair.
// Used when the user denies consent so the client immediately loses access.
func (s *PostgresStore) RevokeRefreshTokens(ctx context.Context, userID, clientID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE oauth_refresh_tokens
		 SET revoked_at = NOW()
		 WHERE user_id = $1
		   AND client_id = $2
		   AND revoked_at IS NULL`,
		userID, clientID,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}

// ── Magic-link token methods ──────────────────────────────────────────────────

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

// ── Embedding methods ─────────────────────────────────────────────────────────

// SetPhraseEmbedding stores the vector embedding for a phrase.
func (s *PostgresStore) SetPhraseEmbedding(ctx context.Context, id string, embedding []float32) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE phrases SET embedding = $1 WHERE id = $2`,
		pgvector.NewVector(embedding), id,
	)
	if err != nil {
		return fmt.Errorf("set phrase embedding: %w", err)
	}
	return nil
}

// SearchPhrasesBySimilarity returns up to limit phrases for the given user ordered
// by cosine similarity to the provided embedding. Phrases without an embedding are excluded.
func (s *PostgresStore) SearchPhrasesBySimilarity(ctx context.Context, userID string, embedding []float32, limit int) ([]Phrase, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, phrase, headwords, note, source_urls, created_at, updated_at
		 FROM phrases
		 WHERE user_id = $1 AND embedding IS NOT NULL
		 ORDER BY embedding <=> $2
		 LIMIT $3`,
		userID, pgvector.NewVector(embedding), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search phrases by similarity: %w", err)
	}
	defer rows.Close()

	var phrases []Phrase
	for rows.Next() {
		var p Phrase
		if err := rows.Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan phrase: %w", err)
		}
		phrases = append(phrases, p)
	}
	return phrases, rows.Err()
}

// ListPhrasesWithoutEmbedding returns all phrases that have not yet been embedded.
// Used by the backfill endpoint to catch up after the migration or on failure.
func (s *PostgresStore) ListPhrasesWithoutEmbedding(ctx context.Context) ([]Phrase, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, phrase, headwords, note, source_urls, created_at, updated_at
		 FROM phrases WHERE embedding IS NULL`,
	)
	if err != nil {
		return nil, fmt.Errorf("list phrases without embedding: %w", err)
	}
	defer rows.Close()

	var phrases []Phrase
	for rows.Next() {
		var p Phrase
		if err := rows.Scan(&p.ID, &p.Phrase, &p.Headwords, &p.Note, &p.SourceURLs, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan phrase: %w", err)
		}
		phrases = append(phrases, p)
	}
	return phrases, rows.Err()
}

