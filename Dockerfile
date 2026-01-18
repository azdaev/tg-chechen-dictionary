FROM golang:1.25-alpine3.22

WORKDIR /app

# Install dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Copy and build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o chetoru .

CMD ["./chetoru"]
