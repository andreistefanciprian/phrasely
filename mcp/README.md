# Phrasely MCP Server

Exposes `list_phrases`, `sample_phrases`, `explore_phrase`, `render_phrase_choices`, and `add_phrase` over MCP (Streamable HTTP), backed by the private `backend` API. `render_phrase_choices` owns an optional MCP Apps component with one-click Save actions.

## Auth

MCP initialization, `tools/list`, and the stateless `explore_phrase` and
`render_phrase_choices` tools are public. Reading the phrase-choice UI resource
is also public. Tools that read or write a user's collection require
`Authorization: Bearer <access_token>`; missing, invalid, or expired tokens
return an `mcp/www_authenticate` challenge. The access token is a short-lived
JWT issued by the OAuth 2.1 + PKCE flow:

1. Client fetches `/.well-known/oauth-authorization-server` to discover endpoints.
2. Client registers via `POST /register` (Dynamic Client Registration, RFC 7591).
3. User logs in and consents at `GET /authorize` on the frontend.
4. Client exchanges the auth code for tokens at `POST /token` (PKCE S256).
5. Client calls tools on `/mcp` with the access token as a Bearer header.

`MCP_AUTH_TOKEN` no longer exists — all auth goes through the OAuth flow.

## Phrase-choice UI

After `explore_phrase`, the assistant prepares three complete save-ready
entries: the refined original context (or a personal context when the user
provided only the target expression), a distinct situation from the user's
life, and a learning connection grounded in another personal situation. It
personalizes only from reliable context and does not invent personal facts.
The assistant then calls `render_phrase_choices`. That read-only tool returns
all three as structured, save-ready choices and is associated with
`ui://phrasely/phrase-choices-v2.html`.

The UI displays the connection as the third saveable card. Its title names
the selected category—for example, **A likely confusable word**, **A meaningful
opposite or contrast**, or **A nuanced near-synonym**—rather than using the
generic label “One useful connection.” The assistant falls back to register,
usage patterns, word families, common mistakes, or a memory association only
when those are more useful, and does not force a weak connection.

Supporting clients render the resource as an inline MCP Apps component. Every
card's Save button calls the existing OAuth-protected `add_phrase` tool through the
standard `tools/call` bridge; the component never calls the backend directly
and never handles Bearer tokens. The button is the user's save instruction.
Rendering choices alone does not persist anything.

The component is plain HTML, CSS, and JavaScript embedded into the Go binary.
It has no external runtime assets or build step. Clients without MCP Apps UI
support continue with numbered conversational choices and direct `add_phrase`
calls after the user selects one.

The component performs the MCP Apps `ui/initialize` / `ui/notifications/initialized`
handshake before using the standard `tools/call` bridge. It also supports
ChatGPT's `window.openai.callTool` compatibility API.

## Railway debugging

OAuth and MCP milestones are emitted as structured logs without tokens, auth
codes, PKCE material, cookies, or request bodies. At the default `INFO` level,
Railway shows successful registration, authorization, token exchange, MCP
initialization, and tool discovery. Set `LOG_LEVEL=DEBUG` temporarily to also
see discovery-document requests, successful proxy responses, and authenticated
tool calls. Rejections include a safe `reason`, `client_id` where available,
and the affected tool or internal path.

## Local testing

Start the full stack (frontend, api, mcp, db are all needed for the consent screen):

```bash
docker compose up --build
```

`mcp` listens on `http://localhost:8081`.

### Quick test with a magic-link JWT

For ad-hoc testing you can skip the OAuth flow and use a magic-link JWT directly
as the Bearer token — the backend accepts both:

```bash
./scripts/api.sh auth-request you@example.com
# grab the token from stdout (dev mode logs it)
./scripts/api.sh auth-verify <token>
# prints a JWT — use it below

curl -s http://localhost:8081/mcp \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | jq
```

### MCP Inspector (full OAuth flow)

```bash
npx @modelcontextprotocol/inspector
```

Open the printed URL, choose transport **Streamable HTTP**, set the server URL to
`http://localhost:8081/mcp`, and follow the OAuth prompts. Inspector handles
client registration, the consent redirect, and token exchange automatically.

Or non-interactively with a pre-obtained JWT:

```bash
npx @modelcontextprotocol/inspector --cli http://localhost:8081/mcp \
  --transport http \
  --header "Authorization: Bearer <jwt>" \
  --method tools/list
```

MCP Inspector can verify the resource and structured render result. The final
visual and click-to-save flow must also be checked in a client that implements
MCP Apps UI and OAuth tool calls, such as ChatGPT.

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "phrasely": {
      "url": "http://localhost:8081/mcp"
    }
  }
}
```

Restart Claude Desktop. It will trigger the OAuth flow on first use and cache
the tokens for subsequent calls.
