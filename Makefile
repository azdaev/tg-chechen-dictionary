include .env

migrate-up:
	docker exec -it chetoru_golang_container go run migrations/run_migrations.go