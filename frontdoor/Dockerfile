FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o frontdoor .

# ── Final image ──────────────────────────────────────────────────────────────
FROM alpine:latest

RUN adduser -D -u 1001 appuser
WORKDIR /app

COPY --from=builder /app/frontdoor .

USER appuser
EXPOSE 3000
CMD ["./frontdoor"]
