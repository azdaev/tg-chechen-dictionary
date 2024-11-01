FROM golang:1.22.3

WORKDIR /app

RUN go install github.com/pressly/goose/v3/cmd/goose@latest

COPY go.mod go.sum ./
RUN go mod download


COPY . .

RUN go build -o /chetoru

CMD ["/chetoru"]
