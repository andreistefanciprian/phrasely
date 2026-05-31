# Stage 1: build the binary
FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p bin && go build -o bin/api ./cmd/api

# Stage 2: minimal runtime image
FROM alpine:3.21
RUN adduser -D -u 1001 appuser
WORKDIR /app
COPY --from=builder /app/bin/api .
USER appuser
EXPOSE 8080
CMD ["./api"]
