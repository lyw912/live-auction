.PHONY: test run dev-watch migrate-up migrate-status

DATABASE_URL ?= postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable

test:
	go test ./...

run:
	go run ./cmd/server

dev-watch:
	air

migrate-up:
	goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-status:
	goose -dir migrations postgres "$(DATABASE_URL)" status
