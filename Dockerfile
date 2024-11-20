FROM golang:1.23.0

WORKDIR /app

COPY go.mod go.sum ./   
ENV GOPROXY=direct
RUN go mod download

COPY . .

RUN go build -o /chetoru

CMD ["/chetoru"]
