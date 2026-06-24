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

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app/

COPY --from=builder /app/plexams.go .

ENTRYPOINT ["/app/plexams.go"]
