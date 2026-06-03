# Run pending migrations inside the running prod container (goose ships in the image).
migrate-up:
	docker exec chetoru_golang_container sh -c 'goose -dir /app/migrations sqlite3 "$$DB_PATH" up'

migrate-down:
	docker exec chetoru_golang_container sh -c 'goose -dir /app/migrations sqlite3 "$$DB_PATH" down'

migrate-status:
	docker exec chetoru_golang_container sh -c 'goose -dir /app/migrations sqlite3 "$$DB_PATH" status'

build-migrate:
	go build -o migrate ./migrations/run_migrations.go

down:
	docker compose down

run: down
	docker compose up --build -d