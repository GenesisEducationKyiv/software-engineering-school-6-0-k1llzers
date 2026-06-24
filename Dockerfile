FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY pkg ./pkg
COPY app-config.yaml ./app-config.yaml
COPY notification-config.yaml ./notification-config.yaml
COPY quota-config.yaml ./quota-config.yaml

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/notification-service ./cmd/notification-service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/quota-service ./cmd/quota-service

FROM alpine:3.22 AS app

WORKDIR /app

RUN adduser -D -H appuser

COPY --from=builder /out/app ./app
COPY migrations ./migrations
COPY app-config.yaml ./app-config.yaml

USER appuser

EXPOSE 8080

CMD ["./app"]

FROM alpine:3.22 AS notification-service

WORKDIR /app

RUN adduser -D -H appuser

COPY --from=builder /out/notification-service ./notification-service
COPY migrations ./migrations
COPY notification-config.yaml ./notification-config.yaml

USER appuser

CMD ["./notification-service"]

FROM alpine:3.22 AS quota-service

WORKDIR /app

RUN adduser -D -H appuser

COPY --from=builder /out/quota-service ./quota-service
COPY migrations ./migrations
COPY quota-config.yaml ./quota-config.yaml

USER appuser

EXPOSE 9091

CMD ["./quota-service"]
