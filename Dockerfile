# Build args
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build args für den Builder
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_TIME

# Build mit Version
RUN go build -ldflags "\
    -X main.version=${VERSION} \
    -X main.commit=${GIT_COMMIT} \
    -X main.date=${BUILD_TIME} \
    -X main.builtBy=docker" \
    -o plexams.go .

# Final stage
FROM alpine:latest

# tzdata is required: main.go forces time.Local = Europe/Berlin (slot/time calculations).
RUN apk --no-cache add ca-certificates tzdata \
    && adduser -D -H -u 10001 plexams

WORKDIR /app/

COPY --from=builder /app/plexams.go .

# Run as an unprivileged user (the app needs no write access to the filesystem;
# state lives in MongoDB, config is mounted read-only).
USER plexams

# GraphQL/REST server port (server.port). Only exposed to the reverse proxy in
# compose — do NOT publish it to the host in production.
EXPOSE 8080

ENTRYPOINT ["/app/plexams.go"]
