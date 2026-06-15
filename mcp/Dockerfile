FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

# CGO_ENABLED=0  — static binary, no C runtime dependency
# -trimpath      — removes local file paths from the binary (no source leakage)
# -ldflags -s -w — strips debug symbols and DWARF info (smaller, no symbol table)
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o mcp .

# ── Final image ───────────────────────────────────────────────────────────────
# scratch = completely empty — just the binary, no shell, no OS, zero attack surface
FROM scratch

COPY --from=builder /app/mcp /mcp

EXPOSE 8081
# Run as non-root (uid 1001) without needing adduser
USER 1001
ENTRYPOINT ["/mcp"]
