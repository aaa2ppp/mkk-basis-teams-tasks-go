FROM golang:1.26.5-alpine3.24 AS builder
WORKDIR /app
RUN apk --no-cache add make
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.27.1

FROM alpine:3.24 AS migrate
WORKDIR /app
COPY --from=builder /go/bin/goose /bin
COPY scripts/migrate.sh .
COPY migrations migrations
ENTRYPOINT [ "sh", "migrate.sh", "up" ]

FROM builder AS server_builder
WORKDIR /app
COPY go.mod go.sum ./
RUN  go mod download
COPY pkg pkg
COPY cmd cmd
COPY internal internal
COPY Makefile .
RUN  CGO_ENABLED=0 make test bin/server

FROM scratch AS server
COPY --from=server_builder /app/bin/server .
EXPOSE 8080
USER 65535:65535
ENTRYPOINT [ "/server" ]
