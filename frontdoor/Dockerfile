FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

# CGO_ENABLED=0  — static binary, no C runtime dependency
# -trimpath      — removes local file paths from the binary (no source leakage)
# -ldflags -s -w — strips debug symbols and DWARF info (smaller, no symbol table)
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o frontdoor .

# ── Final image ───────────────────────────────────────────────────────────────
FROM alpine:3.22

RUN adduser -D -u 1001 appuser
WORKDIR /app

COPY --from=builder /app/frontdoor .

USER appuser
EXPOSE 3000
CMD ["./frontdoor"]
