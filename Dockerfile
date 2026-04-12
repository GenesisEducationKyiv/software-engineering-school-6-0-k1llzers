FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY config.yaml ./config.yaml

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./cmd/app

FROM alpine:3.22

WORKDIR /app

RUN adduser -D -H appuser

COPY --from=builder /out/app ./app
COPY migrations ./migrations
COPY config.yaml ./config.yaml

USER appuser

EXPOSE 8080

CMD ["./app"]
