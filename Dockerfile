FROM golang:alpine as builder
WORKDIR /app
COPY . .
RUN go get -d -v ./... && \
    go build -o plexams.go main.go

FROM alpine
COPY --from=builder /app/plexams.go /app/plexams.go

RUN apk --no-cache add tzdata wait4x
ENV TZ=Europe/Berlin
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

EXPOSE 8080
ENTRYPOINT ["/app/plexams.go"]