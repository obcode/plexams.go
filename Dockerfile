FROM golang:alpine as builder
WORKDIR /app
COPY . .
RUN go get -d -v ./... && \
    go build -o plexams.go main.go

FROM alpine:edge
COPY --from=builder /app/plexams.go /app/plexams.go
RUN apk add wait4x
EXPOSE 8080
CMD ["/app/plexams.go", "server"]