# Build args
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

FROM golang:1.24-alpine AS builder

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
    -X 'github.com/obcode/plexams.go/cmd.Version=${VERSION}' \
    -X 'github.com/obcode/plexams.go/cmd.BuildTime=${BUILD_TIME}' \
    -X 'github.com/obcode/plexams.go/cmd.GitCommit=${GIT_COMMIT}'" \
    -o plexams.go .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY --from=builder /app/plexams.go .

ENTRYPOINT ["/app/plexams.go"]
