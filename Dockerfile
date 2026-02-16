# Build stage
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o chetoru . && \
    CGO_ENABLED=0 go build -o migrate ./migrations/

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/chetoru .
COPY --from=builder /app/migrate .

ENV TZ=Europe/Moscow

CMD ["./chetoru"]
