# Goose build stage - cached separately
FROM golang:1.25-alpine3.22 AS goose-builder

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

# Build stage
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o chetoru .

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=goose-builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/chetoru .
COPY --from=builder /app/migrations/*.sql migrations/

ENV TZ=Europe/Moscow

CMD ["./chetoru"]
