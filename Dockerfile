FROM golang:1.13-alpine3.10 AS build

WORKDIR /go/src/app

ARG CGO_ENABLED=0

RUN apk --no-cache add ca-certificates git openssl && \
    openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/CN=example.com"

COPY . .

RUN go get -d -v ./... && \
    go build -ldflags "-s -w" -o ./echo-server ./cmd/echo-server

FROM scratch

WORKDIR /bin

COPY --from=build /go/src/app/echo-server .
COPY --from=build /go/src/app/cert.pem .
COPY --from=build /go/src/app/key.pem .

ENV PORT 8080
ENV SSLPORT 8443

EXPOSE "$PORT" "$SSLPORT"

ENV ADD_HEADERS='{"X-Real-Server": "echo-server"}'

ENTRYPOINT ["/bin/echo-server"]
