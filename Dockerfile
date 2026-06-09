# Stage 1: build the binary
FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
# CGO_ENABLED=0  — static binary, no C runtime dependency
# -trimpath      — removes local file paths from the binary
# -ldflags -s -w — strips debug symbols and DWARF info
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /api ./cmd/api

# Stage 2: scratch runtime — just the binary + CA certs for HTTPS (Resend, OpenAI)
FROM scratch

# CA certificates are required for HTTPS calls to external APIs
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /api /api
COPY --from=builder /app/internal/curate/system_prompt.txt /etc/phrasely/system_prompt.txt

EXPOSE 8080
USER 1001
ENTRYPOINT ["/api"]
