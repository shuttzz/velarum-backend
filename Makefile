# Atalhos (tudo roda via Docker — não precisa de Go local).
.PHONY: up down test itest build tidy logs

up:    ; docker compose up --build
down:  ; docker compose down
logs:  ; docker compose logs -f backend
test:  ; docker compose run --rm backend go test ./...
itest: ; docker compose run --rm -e TEST_DATABASE_URL=postgres://velarum:velarum@db:5432/velarum_test?sslmode=disable backend go test -p 1 ./...
build: ; docker compose run --rm backend go build ./...
tidy:  ; docker compose run --rm backend go mod tidy
