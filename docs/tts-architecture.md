# Listen on Shuffle — Implementation Plan

Status: planned, not implemented.

## Goal

Add a **Listen** button only to the authenticated Shuffle page. It reads the
current phrase aloud using one configured ElevenLabs voice.

The first listen generates the MP3 on demand. Later listens to the same phrase
text by the same user use a private Cloudflare R2 cache and do not call
ElevenLabs again. Editing the phrase text naturally creates a new cache entry;
there is no cache-invalidation workflow.

Explicitly out of scope:

- a Listen button on the Phrases page or any other page;
- a voice picker;
- a download button;
- background or deployment-time pre-generation;
- audio analytics or an audio metadata table;
- cross-user cache sharing.

## Request flow

```text
Shuffle page
  -> GET /fd/phrases/{phraseID}/audio
     -> authenticated frontend streaming proxy
        -> GET /api/v1/phrases/{phraseID}/audio with JWT
           -> load phrase with GetPhrase(userID, phraseID)
           -> spoken text = phrase.Phrase
           -> calculate deterministic cache hash
           -> GET phrase-audio/{userID}/{hash}.mp3 from private R2
              -> hit: read cached MP3 bytes and return them
              -> miss: apply generation safeguards
                       generate MP3 with ElevenLabs
                       buffer the small clip in memory
                       PUT it into R2
                       return it to the browser
```

The API obtains the spoken text exclusively from PostgreSQL after checking
ownership. The frontend never sends rendered phrase text to the speech route.
`Phrase.Phrase` already excludes `Headword.Meaning`, so no parenthesis removal
or other text parsing is needed. Legitimate parentheses in the phrase remain
part of the speech.

## Cache identity and ownership

Use a SHA-256 digest of the JSON encoding of a typed, versioned identity. A Go
struct has deterministic field order and avoids delimiter ambiguity if phrase
text contains newlines:

```go
type AudioCacheIdentity struct {
	Version      int           `json:"version"`
	Text         string        `json:"text"`
	VoiceID      string        `json:"voice_id"`
	ModelID      string        `json:"model_id"`
	OutputFormat string        `json:"output_format"`
	Settings     VoiceSettings `json:"settings"`
}

b, err := json.Marshal(identity)
// Treat an unexpected marshal error as an internal error.
hash := sha256.Sum256(b)
```

The audio module owns these fixed implementation invariants:

```go
const (
	outputFormat       = "mp3_44100_128"
	cacheSchemaVersion = 1
)
```

Do not expose the output format as configuration in this version. Keeping the
format, `.mp3` object suffix, ElevenLabs request, and `audio/mpeg` response type
together prevents invalid combinations.

Store the result at:

```text
phrase-audio/{userID}/{hash}.mp3
```

The key is user-scoped deliberately. This gives each user a simple ownership
and deletion story and prevents one user's private phrase audio from being used
as another user's cache object. Identical text saved by two users may therefore
incur two ElevenLabs generations.

Within one user's collection, identical text and identical speech settings
share one object. A change to `phrase.Phrase`, the selected voice, model, output
format, voice settings, or cache schema version produces a different hash and a
fresh generation on the next listen. Changes to meanings, headwords, notes, or
source URLs do not regenerate audio because they do not affect speech.

R2 is the only persistent cache index: check the deterministic object key
directly. Do not add a `phrase_audio` PostgreSQL table initially. Old objects
left behind after phrase edits are acceptable at the current scale. A bucket
lifecycle or account-deletion cleanup can be introduced when retention becomes
a real concern.

## Backend module design

Create one deep `audio` module whose small external interface is the HTTP route:

```text
GET /api/v1/phrases/{id}/audio
```

The handler hides phrase authorization, cache-key construction, R2 lookup,
ElevenLabs generation, caching, response headers, and failure mapping. Its
dependencies are accepted by its constructor so tests cross the same seam as
production callers.

Internally, use two narrow seams with real production and fake test adapters:

- an object-cache interface that buffers the small clips consistently:
  `Get(ctx, key) ([]byte, error)` and
  `Put(ctx, key string, audio []byte) error`;
- a synthesizer interface that converts phrase text into MP3 bytes.

Production adapters wrap Cloudflare R2's S3-compatible interface and the
ElevenLabs HTTP endpoint. Keep vendor-specific request construction, response
validation, credentials, and timeouts inside those adapters. Apply a reasonable
maximum response size to both ElevenLabs and R2 reads before buffering, so a
malformed or unexpectedly large response cannot cause unbounded memory use.
There is no need for streaming readers within the audio module; sentence-sized
MP3s are deliberately handled as `[]byte` throughout.

Add the new route to the existing phrase-authenticated router. The handler must
call `GetPhrase(ctx, userID, id)` before reading R2, so a request for a missing or
other user's phrase remains indistinguishable as `404`.

### Cache-hit behavior

On a hit, return the R2 bytes with:

```http
Content-Type: audio/mpeg
Cache-Control: private, no-cache
```

`no-cache` allows browser storage but requires revalidation at the stable
phrase-ID URL, preventing stale audio after the phrase is edited. Do not add
`ETag` or `If-None-Match` handling initially; every page reload can revalidate
through Phrasely and perform the inexpensive R2 lookup. The private R2 bucket
and object key are never exposed to the browser.

### Cache-miss behavior

On a miss:

1. Enter `singleflight` using `{userID}/{hash}` as the key.
2. Check R2 again inside the shared call.
3. Check and consume the user's generation allowance only if the object is
   still absent.
4. Call ElevenLabs.
5. Buffer the complete MP3 in memory; phrase clips are small.
6. Upload the MP3 to R2.
7. Return the buffered bytes.

Only the goroutine that actually calls ElevenLabs consumes the generation
allowance. Concurrent waiters and cache hits do not. `singleflight` prevents
duplicate charges inside one API process. A rare duplicate generation across
multiple Railway replicas is accepted initially; distributed locking is not
warranted while the API runs as one replica and usage is small.

Start with a simple per-user limit of 40 actual generations per rolling hour.
Keep this limiter in memory for the first version. It protects the paid
dependency without adding schema or infrastructure. Document that the limit is
per API process and revisit it only if the API is scaled horizontally.

If ElevenLabs succeeds but the R2 upload fails, return and play the generated
audio, log the cache failure, and allow a future listen to retry caching. Never
cache ElevenLabs errors, empty responses, invalid content types, rate-limit
responses, or R2 errors.

Use `singleflight.DoChan` (or equivalent behavior) so each HTTP request can stop
waiting when its own context is cancelled. Once an ElevenLabs generation has
started, it should continue with a separate bounded context, finish, and cache
the result for any other waiters and future requests. Do not derive that shared
generation context from the first HTTP request. Derive it from the application's
lifetime context, add a generous timeout, and still allow application shutdown
to cancel it. This avoids discarding a generation that has probably already
incurred cost.

If the initial or inside-`singleflight` R2 lookup returns anything other than a
definite object-not-found result, return `503`; do not turn a storage outage into
a wave of paid ElevenLabs generations.

## Frontend proxy

The existing `/fd/*` proxy buffers responses and forces
`Content-Type: application/json`. Add a narrow binary streaming path for:

```text
GET /fd/phrases/{id}/audio
```

It should:

- attach the JWT from the existing authenticated cookie context;
- stream the upstream response body instead of reading it as JSON;
- preserve `Content-Type`, `Content-Length`, and `Cache-Control`;
- preserve the upstream status code;
- use a client timeout suitable for a cold ElevenLabs generation;
- leave every existing JSON proxy request unchanged.

No public R2 URL, R2 CORS policy, browser credential, or signed URL is needed.

## Shuffle interaction

Change only `frontend/templates/shuffle.html` for the visible feature.

The visual and interaction source of truth is the supplied
`speech-button-handoff.md`, specifically its Turn 4 references (cards 4a, 4b,
and 4c). Use its placement, speaker glyph, dimensions, typography, colors,
motion, focus treatment, and responsive hit area. Do **not** use its Web Speech
implementation instructions: Phrasely uses the authenticated ElevenLabs/R2
flow in this plan, and the stored `Phrase.Phrase` remains the only speech input.

Place a real `<button type="button">` in the existing Shuffle hint row, to the
right of **Click to shuffle** or **Tap to shuffle**, separated by the specified
vertical divider:

```text
CLICK TO SHUFFLE  |  [speaker glyph] LISTEN
```

The row remains centred above the phrase; nothing new appears beneath the
phrase. Because the row lives inside `#main-content`, update that container's
click handling to treat the Listen button as an interactive control and never
turn its clicks into shuffles.

Layout measurements from the handoff:

- row: flex, centered on both axes, 12px gap;
- divider: 1px x 11px using `var(--border)`;
- button: 8px internal gap, 6px 10px padding, transparent background, no border,
  and 999px radius;
- label: 11px uppercase with `0.16em` letter spacing, fixed 52px width,
  left-aligned, and no wrapping.

Use the handoff's inline 22 x 20 SVG speaker glyph (cone plus two arcs), not an
emoji or CSS-positioned spans. Keep the label in its fixed-width 52px slot so
switching among **LISTEN**, **LOADING**, and **STOP** does not shift the row.
The control remains visually quiet at rest: transparent background, muted
color, no circle, and no accent color.

Maintain the current phrase ID in the existing `show` function. The audio state
is:

```text
Rest (LISTEN) -> Loading (LOADING) -> Speaking (STOP)
      ^                                      |
      |--------------------------------------|
        stop, natural end, phrase change, or error
```

Behavior:

- the first click assigns `/fd/phrases/{currentID}/audio` to one reusable Audio
  element and begins playback;
- clicking **STOP** ends playback, resets it to the beginning, clears the audio
  source, and returns the control to **LISTEN**; there is no pause/resume state;
- reaching the natural end returns the control to **LISTEN**;
- changing the displayed phrase stops playback, clears the old audio source,
  returns to **LISTEN**, and prevents a late response from playing;
- loading prevents duplicate generation clicks and exposes an accessible busy
  state;
- Space or Enter on a focused Listen button activates the button rather than
  invoking Shuffle's global Space shortcut;
- all playback failures restore **Listen** and show the same short message:
  **Couldn't play this phrase. Try again.** The `<audio>` element does not need
  to inspect or distinguish `404`, `429`, `502`, or `503` responses;
- changing phrases or retrying clears the previous error.

Use `preload="none"`; merely viewing or shuffling a phrase must not request
audio. Do not add download-related markup or controls.

### Visual states

| State | Appearance | Motion | Label |
|---|---|---|---|
| Rest | Muted: `#9299BB` dark / `#6170B8` light; transparent background | None | `LISTEN` |
| Hover/focus | Text: `#F4F5FF` dark / `#0F1B6E` light; pill: `#20264A` dark / `#F6F6FF` light; visible 2px brand outline with 2px offset | None | `LISTEN` |
| Loading | Dimmed muted: `#555E96` dark / `#9DA3CC` light | Two arcs pulse at 1.4s with a 0.5s offset | `LOADING` |
| Speaking | Accent: `#56DDD6` dark / `#1FD3CC` light; this is the only accent-colored state | Two arcs pulse at 0.9s with a 0.3s offset | `STOP` |
| Error | Return to Rest and show the generic inline error | None | `LISTEN` |

Animate only the two arc paths; the speaker cone never animates. Under
`prefers-reduced-motion: reduce`, remove the pulse while retaining the state
color and label. Pulse between opacity `1` and `.2`, returning to `1`. Use
existing theme tokens/colors matching the values above and introduce no new
colors or fonts. Use a 34px desktop hit area and at least 44px on touch
viewports.

The handoff's **Unavailable** state is not used in the first version. The
frontend intentionally has no duplicated configuration or capability request,
so it cannot know at render time that the backend is unavailable. A `503` or
other playback failure follows the generic Error behavior and remains
retryable.

Accessibility labels:

- Rest/Loading: `aria-label="Hear the phrase"`;
- Speaking: `aria-label="Stop phrase audio"`;
- set `aria-busy="true"` while loading and remove it afterward;
- no `aria-live` region is needed for the changing button label, but the generic
  error text should be announced accessibly when it appears.

## Configuration

Add these optional backend environment variables and document them in
`.env.example`, `docker-compose.yml`, and `AGENTS.md`:

| Variable | Purpose |
|---|---|
| `ELEVENLABS_API_KEY` | Server-side ElevenLabs credential |
| `ELEVENLABS_VOICE_ID` | The single product voice |
| `ELEVENLABS_MODEL_ID` | Model used in the cache identity |
| `R2_ENDPOINT` | Account-specific S3-compatible endpoint |
| `R2_BUCKET` | Private audio bucket |
| `R2_ACCESS_KEY_ID` | R2 access credential |
| `R2_SECRET_ACCESS_KEY` | R2 secret credential |

Always register the audio route and always render the Listen button on Shuffle.
When the ElevenLabs or R2 configuration is incomplete, the handler returns
`503 Service Unavailable` and logs one clear startup warning; the frontend shows
the same generic playback failure used for all errors. Do not duplicate backend
configuration in the frontend and do not add a capabilities endpoint solely to
hide this button. This keeps the backend as the single source of truth. Do not
fall back to browser speech synthesis because that would violate the promised
voice and produce inconsistent behavior.

## Error contract

The audio route should return:

| Status | Meaning |
|---|---|
| `200` | Cached or newly generated MP3 |
| `404` | Phrase missing or owned by another user |
| `429` | User's generation allowance is exhausted |
| `502` | ElevenLabs returned an unusable response |
| `503` | Listen is disabled or temporarily unavailable |

Return JSON errors before audio bytes have been written. The frontend audio
proxy must preserve their status and content type rather than relabeling them as
MP3.

## Implementation sequence

Ship the feature as five sequential, independently reviewed PRs. Create each
branch from the updated `main` after the previous PR merges.

1. [x] **ElevenLabs adapter** — fixed MP3 format, voice/model configuration,
   response validation, size limit, timeouts, cancellation behavior, and
   `httptest` coverage. No route or UI.
2. [ ] **R2 audio-cache adapter** — buffered `Get`/`Put`, private bucket
   configuration, object-not-found distinction, size limit, and adapter tests.
   No database table.
3. [ ] **Authenticated audio endpoint** — typed cache identity, user-scoped key,
   phrase ownership, double-checked R2 flow, miss-only limiter,
   cancellation-safe `singleflight`, best-effort caching, status contract, and
   handler tests.
4. [ ] **Frontend binary proxy** — stream the audio HTTP response and preserve its
   status and relevant headers without changing existing JSON proxy behavior.
5. [ ] **Shuffle Listen control** — implement the supplied Turn 4 visual design,
   Listen/Loading/Stop behavior, keyboard handling, reduced motion, generic
   error, and UI tests. Update final user-facing documentation here.

PRs 1–4 expose no user-facing control. PR 5 renders Listen only after its full
backend path has merged. Do not split PR 3 in a way that temporarily exposes an
endpoint without its concurrency and spending safeguards.

## Verification

Automated tests should prove:

- the exact stored phrase text is sent to ElevenLabs;
- meanings, notes, and headword metadata never enter spoken text or the hash;
- text or speech-setting changes produce a new hash;
- identical text for the same user reuses one object;
- identical text for different users produces different object keys;
- a cache hit never calls ElevenLabs or consumes generation allowance;
- concurrent misses in one process call ElevenLabs once and consume one
  generation allowance;
- cancelling one waiting request does not cancel shared generation or caching;
- an R2 error other than object-not-found does not call ElevenLabs;
- an R2 upload failure still returns generated audio;
- oversized ElevenLabs or R2 responses are rejected before unbounded buffering;
- failed or invalid ElevenLabs responses are not cached;
- another user's phrase returns `404` before cache access;
- the frontend proxy preserves binary bytes, headers, and errors;
- Listen clicks and keyboard activation do not shuffle;
- changing phrases stops stale audio and clears the generic error.

Manual acceptance:

1. Open Shuffle and confirm no audio request occurs until Listen is clicked.
2. Click Listen and confirm a short cold-generation delay followed by playback.
3. Click **STOP** and confirm playback ends and resets; click **LISTEN** again
   and confirm it replays without another ElevenLabs generation.
4. Reload and listen again; confirm the R2 hit plays without generation cost.
5. Edit only the meaning or note; confirm the same cached audio remains valid.
6. Edit the phrase text; confirm the next listen creates a new object and speaks
   the new text.
7. Sign in as another user with identical text; confirm a separate user-scoped
   object is generated.
8. Simulate ElevenLabs and R2 failures and confirm the UI recovers cleanly.

Run the repository's normal `task test` suite before requesting review. Do not
push until the user has reviewed the implementation, per the repository's Git
workflow.
