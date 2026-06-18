# pgvector Roadmap

Semantic intelligence layer built on top of PostgreSQL's `vector` extension.
Every feature in this roadmap shares the same foundation (Phase 1).
Phases 2–6 are independent of each other and can be reordered by priority.

---

## Dependency map

```
Phase 1 — Foundation        (prerequisite for everything)
    ├── Phase 2 — Semantic Search   (Features 1, 4 partial)
    ├── Phase 3 — Related Phrases   (Features 2, 3)
    ├── Phase 4 — Clusters          (Features 4, 7)
    ├── Phase 5 — MCP               (Feature 8)
    └── Phase 6 — AI Coach          (Features 5, 6, 9)

Phase 7 — Phrase Graph      (Feature 10, future — depends on Phase 4)
```

---

## Phase 1 — Foundation

**User-facing change:** none. Pure infrastructure.
**Unlocks:** everything below.

### What gets built

| File | Change |
|---|---|
| `backend/migrations/00003_add_phrase_embeddings.sql` | `CREATE EXTENSION vector`, `ADD COLUMN embedding vector(1536)`, HNSW index |
| `backend/internal/embeddings/service.go` | New package — wraps `go-openai`, calls `text-embedding-3-small` |
| `backend/internal/db/db.go` | 2 new Store methods (see below) |
| `backend/internal/phrases/handler.go` | Wire embedder into create/update (async goroutine) |
| `backend/cmd/api/main.go` | Initialize embedder alongside curate (optional, same `OPENAI_API_KEY`) |
| `backend/internal/phrases/handler.go` | `POST /internal/phrases/embed-backfill` for existing phrases |

New dependency: `github.com/pgvector/pgvector-go` (pgx v5 compatible vector type).

### New DB methods

| Method | Purpose |
|---|---|
| `SetPhraseEmbedding(ctx, id, vector)` | Persist embedding after save |
| `ListPhrasesWithoutEmbedding(ctx)` | Used by backfill endpoint |

### Text embedded per phrase

All three fields combined give the richest semantic signal:

```
fortitude, resilience
Facing the crisis demanded extraordinary fortitude.
Used when praising steadfast resolve under pressure.
```

### Embedding cost: platform pays

All embeddings are generated using the platform's `OPENAI_API_KEY`, regardless of how the phrase was created (web UI or MCP tool from ChatGPT). At `text-embedding-3-small` rates (~$0.02/1M tokens), a typical phrase costs ~$0.000002 to embed — negligible at current scale. If cost becomes a concern, the lazy alternative is to skip embedding on save and only embed when the user first triggers a vector feature; the backfill endpoint already supports this path.

### Embedding failure strategy: save-and-catch

Save the phrase to the DB immediately, then generate the embedding in a background goroutine. If OpenAI is down, times out, or rate-limits us — log the error and move on. The phrase lands with `embedding = NULL` and the backfill endpoint picks it up later. The phrase is never blocked on OpenAI availability; the only cost is a short window where a freshly saved phrase won't appear in semantic search results.

---

## Phase 2 — Semantic Search

**Unlocks:** Feature 1 (semantic keyword search), Feature 4 (browse by meaning via free-text).

### What gets built

- `GET /api/v1/phrases/search?q=` — embeds the query, returns top-N phrases by cosine similarity
- Frontend search bar wired to this endpoint

### Open questions

- [x] **Augment**: exact headword match first, semantic fallback when no exact results. Typing a known headword always finds it; semantic kicks in when nothing matches.

---

## Phase 3 — Related Phrases & You May Also Like

**Unlocks:** Feature 2 (related phrases), Feature 3 (you may also like).

### What gets built

- `GET /api/v1/phrases/{id}/related?limit=5`
- Phrase detail page: "Related phrases" section below the phrase
- Collection home: "You may also like" — phrases similar to the user's most recent 3–5 saves

### Open questions

- [ ] **Where does "You may also like" live?** Options:
  - Bottom of the main phrase list
  - Dedicated section on the collection home page
  - Sidebar or drawer

---

## Phase 4 — Clusters & Browse by Meaning

**Unlocks:** Feature 7 (phrase clusters), Feature 4 (browse by meaning).

Most technically interesting phase.

### What gets built

- k-means on the user's phrase embeddings — automatic grouping
- OpenAI labels each cluster based on its contents (e.g. "Uncertainty", "Courage")
- New page: Browse by Meaning — cluster labels with their phrases

### Open questions

- [ ] **Number of clusters**: fixed (e.g. 6) or dynamic (`sqrt(n/2)`, scales with collection size)?
- [ ] **When to recompute clusters**: on every phrase save (always fresh), or on-demand (user triggers "regroup")?
- [ ] **Cluster labels via OpenAI**: send top 5 headwords per cluster to GPT-4o-mini → gets a label like "Perseverance". Alternative: no labels, just grouped cards.
- [ ] **Browse by Meaning UX**: user types a topic to search (dynamic), OR pre-computed cluster groups (static), OR both?

---

## Phase 5 — MCP Superpowers

**Unlocks:** Feature 8.

### What gets built

- New `search_phrases` MCP tool: natural language query → semantically matching phrases from the user's collection
- Existing `list_phrases` and `add_phrase` tools unchanged

Small phase once Phase 2 exists — the MCP tool calls the same search endpoint.

---

## Phase 6 — AI Coach & Smart Shuffle

**Unlocks:** Features 5, 6, 9.

### What gets built

- `POST /api/v1/phrases/match` — user submits text, backend embeds it, returns top-N phrases from their collection
- Frontend: "Elevate my expression" input — paste a sentence, get suggested phrases from your collection
- Smart Shuffle: "random phrase about [topic]" — semantic search with randomised selection from top-20 results

---

## Phase 7 — Phrase Graph

**Unlocks:** Feature 10. Future — defer until Phases 1–4 prove value.

Visual bubble map. Main work is frontend (D3.js or canvas). Depends on cluster data from Phase 4.

---

## Decisions log

| # | Decision | Options | Status |
|---|---|---|---|
| 1 | Text to embed per phrase | headwords + phrase + note | ✅ Decided |
| 2 | Semantic search UX | replace vs augment headword filter | ✅ Augment |
| 3 | "You may also like" placement | list bottom / home section / sidebar | ⬜ Open |
| 4 | Number of clusters | fixed vs dynamic | ⬜ Open |
| 5 | Cluster recompute trigger | on-save vs on-demand | ⬜ Open |
| 6 | Cluster labels | OpenAI-named vs unlabelled | ⬜ Open |
| 7 | Browse by Meaning UX | type-to-search vs pre-computed clusters vs both | ⬜ Open |
| 8 | Phase execution order | 1→2→3→4→5→6 or reprioritised | ⬜ Open |
