FROM golang:1.24.1

WORKDIR /app

COPY go.mod go.sum ./   
RUN go mod download

COPY . .

RUN go build -o /chetoru
RUN go build -o /app/migrate ./migrations/run_migrations.go

CMD ["/chetoru"]
