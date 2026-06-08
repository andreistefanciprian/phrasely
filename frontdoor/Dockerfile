FROM golang:1.26.1 AS builder

WORKDIR /app

# Download Tailwind standalone CLI (glibc build, works on Debian-based builder)
RUN curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64 \
    && chmod +x tailwindcss-linux-x64 \
    && mv tailwindcss-linux-x64 tailwindcss

COPY . .

# Generate optimised CSS from templates
RUN ./tailwindcss -i input.css -o static/app.css --minify

# Build Go binary — CGO disabled for static binary that runs on Alpine (musl)
RUN go mod download && CGO_ENABLED=0 GOOS=linux go build -o frontdoor .

# ── Final image ──────────────────────────────────────────────────────────────
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/frontdoor .

EXPOSE 3000
CMD ["./frontdoor"]
