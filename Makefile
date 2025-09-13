migrate-up:
	docker exec -it chetoru_golang_container /app/migrate

migrate-down:
	docker exec -it chetoru_golang_container /app/migrate -down

build-migrate:
	go build -o migrate ./migrations/run_migrations.go

down:
	docker compose down

run: down
	docker compose up --build -d