# Phrasely MCP Server

Exposes `list_phrases` and `add_phrase` over MCP (Streamable HTTP), backed by the
private `backend` API. See [docs/mcp-server.md](../docs/mcp-server.md) for the
full plan.

## Phase 1: static token auth

For now, `mcp` authenticates to `backend` using a single static long-lived JWT
read from `MCP_AUTH_TOKEN` — the same JWT format issued by the magic-link login
flow. Every tool call is scoped to whichever user that JWT belongs to.

## Local testing

1. **Get a JWT** via the existing magic-link flow:

   ```bash
   ./scripts/api.sh auth-request you@example.com
   # check your email (or `docker compose logs api | grep -i 'magic link'`) for the link/token
   ./scripts/api.sh auth-verify <token-from-link>
   ```

   This prints a JWT — export it for the next step.

2. **Set `MCP_AUTH_TOKEN`** in `.env`:

   ```
   MCP_AUTH_TOKEN=<jwt-from-step-1>
   ```

3. **Start the stack**, including `mcp`:

   ```bash
   docker compose up --build mcp
   ```

   `mcp` listens on `http://localhost:8081`, with the MCP endpoint at
   `http://localhost:8081/mcp`.

## Connecting a client

### MCP Inspector

```bash
npx @modelcontextprotocol/inspector
```

Open the printed URL, choose transport "Streamable HTTP", and set the server
URL to `http://localhost:8081/mcp`. You should see `list_phrases` and
`add_phrase` under Tools.

Or, non-interactively from the CLI:

```bash
npx @modelcontextprotocol/inspector --cli http://localhost:8081/mcp --transport http --method tools/list
npx @modelcontextprotocol/inspector --cli http://localhost:8081/mcp --transport http \
  --method tools/call --tool-name list_phrases --tool-arg headword=unfettered
```

### Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "phrasely": {
      "url": "http://localhost:8081/mcp"
    }
  }
}
```

Restart Claude Desktop and the `list_phrases`/`add_phrase` tools should appear
for the `phrasely` server.
