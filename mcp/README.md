# Phrasely MCP Server

Exposes `list_phrases`, `sample_phrases`, `explore_phrase`, and `add_phrase` over MCP (Streamable HTTP), backed by the
private `backend` API.

## Auth

`/mcp` requires `Authorization: Bearer <access_token>`. The access token is a
short-lived JWT issued by the OAuth 2.1 + PKCE flow:

1. Client fetches `/.well-known/oauth-authorization-server` to discover endpoints.
2. Client registers via `POST /register` (Dynamic Client Registration, RFC 7591).
3. User logs in and consents at `GET /authorize` on the frontend.
4. Client exchanges the auth code for tokens at `POST /token` (PKCE S256).
5. Client calls `/mcp` with the access token as a Bearer header.

`MCP_AUTH_TOKEN` no longer exists — all auth goes through the OAuth flow.

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
