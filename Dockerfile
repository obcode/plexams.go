FROM golang:alpine as builder
WORKDIR /app
COPY . .
RUN  \
    go get -d -v ./... && \
    go build -o plexams.go main.go

FROM alpine
COPY --from=builder /app/plexams.go /app/plexams.go
EXPOSE 8080
ENTRYPOINT [ "/app/plexams.go" ]
CMD [ "server" ]