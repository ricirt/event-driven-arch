# ---- build stage ----
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Download dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

# ---- runtime stage ----
# distroless gives us a minimal, read-only filesystem with no shell.
FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/server /server
COPY --from=builder /app/migrations /migrations

EXPOSE 8080
ENTRYPOINT ["/server"]
