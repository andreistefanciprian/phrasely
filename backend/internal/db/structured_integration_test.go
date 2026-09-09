package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Use only a disposable local database: this test applies migrations and inserts synthetic rows.
func TestStructuredStorageIntegration(t *testing.T) {
	dsn := os.Getenv("PHRASELY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PHRASELY_TEST_DATABASE_URL to an empty disposable local database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("test database must be localhost")
	}
	ctx := context.Background()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(sqlDB, ".", 6); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const user = "550e8400-e29b-41d4-a716-446655440001"
	const other = "550e8400-e29b-41d4-a716-446655440002"
	const id = "550e8400-e29b-41d4-a716-446655440003"
	_, err = pool.Exec(ctx, `INSERT INTO users(id,email) VALUES($1,'synthetic@example.invalid'),($2,'other@example.invalid')`, user, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO phrases(id,user_id,phrase,headwords,source_urls,note,created_at,updated_at) VALUES($1,$2,'She stood up to scrutiny (remained convincing).',ARRAY['stood up to scrutiny'],ARRAY['https://www.merriam-webster.com/dictionary/scrutiny'],'Original note','2025-01-01','2025-01-02')`, id, user)
	if err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	var legacy string
	var modified time.Time
	if err = pool.QueryRow(ctx, `SELECT legacy_phrase,updated_at FROM phrases WHERE id=$1`, id).Scan(&legacy, &modified); err != nil {
		t.Fatal(err)
	}
	if legacy != "She stood up to scrutiny (remained convincing)." || modified.UTC().Format("2006-01-02") != "2025-01-02" {
		t.Fatalf("legacy=%q updated=%v", legacy, modified)
	}
	if s, err := NewPostgresStore(ctx, dsn); err == nil {
		s.Close()
		t.Fatal("unconverted rows must block startup")
	}
	// Simulate a reviewed conversion so storage consumers can be exercised.
	_, err = pool.Exec(ctx, `UPDATE phrases SET headwords='[{"text":"stood up to scrutiny","canonical":"stand up to scrutiny","meaning":"remained convincing","source_url":"https://www.merriam-webster.com/dictionary/scrutiny"}]'::jsonb,phrase='She stood up to scrutiny.' WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	words := []Headword{{Text: "disparaging", Canonical: "disparage", Meaning: "expressing contempt", SourceURL: "https://example.com/disparage"}, {Text: "claims", Canonical: "claim", Meaning: "statements"}}
	p, err := store.CreatePhrase(ctx, user, CreatePhraseRequest{Phrase: "Disparaging claims (even then).", Headwords: words, Note: "A note"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	if string(raw) == "" || len(p.Headwords) != 2 || p.Headwords[0] != words[0] {
		t.Fatalf("roundtrip: %s", raw)
	}
	for _, q := range []string{"disparage", "DISPARAGING"} {
		found, err := store.ListPhrases(ctx, user, q)
		if err != nil || len(found) != 1 {
			t.Fatalf("filter %s: %v %v", q, found, err)
		}
	}
	if found, err := store.ListPhrases(ctx, other, "disparage"); err != nil || len(found) != 0 {
		t.Fatalf("ownership: %v %v", found, err)
	}
	note := "changed"
	p, err = store.UpdatePhrase(ctx, user, p.ID, UpdatePhraseRequest{Note: &note})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Headwords) != 2 || p.Headwords[0] != words[0] {
		t.Fatal("omitted array must remain unchanged")
	}
	replacement := []Headword{{Text: "claims", Canonical: "claim", Meaning: "assertions"}}
	p, err = store.UpdatePhrase(ctx, user, p.ID, UpdatePhraseRequest{Headwords: replacement})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Headwords) != 1 || p.Headwords[0].SourceURL != "" {
		t.Fatal("replacement must remove omitted entries and links")
	}
	summaries, err := store.ListPhrasesSummary(ctx, user, "claim")
	if err != nil || len(summaries) != 1 || summaries[0].Headwords[0].Meaning != "assertions" {
		t.Fatalf("summary: %v %v", summaries, err)
	}
	if _, err = store.GetPhrase(ctx, other, p.ID); err != ErrNotFound {
		t.Fatalf("ownership: %v", err)
	}
	if _, err = store.GetRandomPhrases(ctx, user, 10); err != nil {
		t.Fatal(err)
	}
	beforeEmbedding := p.UpdatedAt
	if err = store.SetPhraseEmbedding(ctx, p.ID, make([]float32, 1536)); err != nil {
		t.Fatal(err)
	}
	p, err = store.GetPhrase(ctx, user, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !p.UpdatedAt.Equal(beforeEmbedding) {
		t.Fatal("embedding maintenance changed last-edit timestamp")
	}
	if _, err = store.SearchPhrasesBySimilarity(ctx, user, make([]float32, 1536), 10); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetRelatedPhrases(ctx, user, p.ID, 0.45, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ListPhrasesWithoutEmbedding(ctx); err != nil {
		t.Fatal(err)
	}
}
