# Structured headwords

Each phrase stores an ordered JSONB array of `{text, canonical, meaning, source_url?}`. These small objects belong to the phrase and are replaced together; a join table and additional indexes are unnecessary for this collection. PostgreSQL handles encoding through pgx's JSONB codec. Text/canonical filtering uses `jsonb_array_elements` with case-insensitive substring matching.

The `headwords` column is non-null. Migration 00009 finalized this schema by removing the temporary legacy columns, their obsolete indexes, and the old array-search helper after the reviewed conversion completed.

The sentence is plain natural text, with legitimate parentheses preserved. Meaning is a separate contextual gloss. `text` is the actual grammatical form; `canonical` is a linguistically authored grouping form, retaining fixed particles/prepositions. No stemming is performed. Canonical equality groups grammatical variants and does **not** assert identical senses. Multiple expressions are supported.

POST requires a nonblank sentence and a nonempty array. Every headword requires nonblank text, canonical and meaning; source_url is optional, with an absolute HTTP(S) URL when provided. PATCH uses the same validation. A supplied array replaces all headwords and links; omitted or null fields leave existing values unchanged. `[]` is invalid. An empty note clears it. Unknown fields, including the former source_urls array, are rejected. Existing sentence parentheses are not classified or removed by CRUD endpoints.

Shuffle, lists, creation previews and MCP cards render escaped glosses from structured data. Each expression's first matched occurrence receives its gloss; later occurrences are emphasized. At the same start position the longest expression wins. An unmatched or wholly overlapped expression's gloss appears after the sentence so it remains visible. Matching is literal, case-insensitive and word-boundary aware. Bubble and sibling grouping use sorted, lowercased canonical sets; grouping says nothing about sense equality. CSV exports contain JSON in the headwords cell to preserve the full structure.

# Local verification

```sh
task test
(cd frontend && node --test headwords_test.cjs)
```

`backend/internal/db/structured_integration_test.go` additionally runs the actual Goose migrations through the temporary structured schema, verifies that finalization rejects an unconverted synthetic row, applies the reviewed-conversion simulation, and checks the final schema, JSONB round trips, filters, ownership, summaries, whole-array PATCH semantics, and embedding timestamp behavior. Set `PHRASELY_TEST_DATABASE_URL` to an **empty disposable localhost database** to enable it. It inserts synthetic rows; never point it at an existing collection.

## Verification for this change

The repository checks cover JSONB CRUD and summaries, text/canonical filtering, ownership, whole-array replacement, rendered meanings, and timestamp preservation during embedding writes. Curation tests use a local mock server.
