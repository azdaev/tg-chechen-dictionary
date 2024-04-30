include .env

init-migrator:
	go install github.com/pressly/goose/v3/cmd/goose@latest

migrate-up:
	docker exec -it chetoru_golang_container goose -dir ./migrations postgres "host=${PG_HOST} user=${PG_USER} password=${PG_PASSWORD} dbname=${PG_DB_NAME} sslmode=disable" up

migrate-down:
	docker exec -it chetoru_golang_container goose -dir ./migrations postgres "host=${PG_HOST} user=${PG_USER} password=${PG_PASSWORD} dbname=${PG_DB_NAME} sslmode=disable" down

up: down
	docker-compose up --build

down:
	docker-compose down
