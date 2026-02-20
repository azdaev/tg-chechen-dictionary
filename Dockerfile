# Build stage
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app

# Install goose (sqlite only, smaller binary)
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go install -ldflags="-s -w" -tags='no_clickhouse no_mssql no_mysql no_postgres no_vertica no_ydb no_libsql no_turso' github.com/pressly/goose/v3/cmd/goose@latest

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o chetoru .

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/chetoru .
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/migrations/*.sql migrations/

ENV TZ=Europe/Moscow

CMD ["./chetoru"]
