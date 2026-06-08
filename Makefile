# Luna Bot — Makefile
# Targets for testing, building, and development

.PHONY: test test-cover test-integration build vet clean lint docker-build docker-up docker-down

# === Testing ===

# test — запускает все юнит-тесты (без интеграционных)
test:
	go test ./internal/... -count=1 -short

# test-cover — тесты с отчётом покрытия
test-cover:
	go test ./internal/... -count=1 -short -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo "---"
	@echo "HTML-отчёт: go tool cover -html=coverage.out"

# test-integration — интеграционные тесты (требуют PostgreSQL)
test-integration:
	go test ./internal/storage/ -run Integration -count=1 -v

# test-all — все тесты, включая интеграционные
test-all:
	go test ./internal/... -count=1 -v

# === Build ===

# build — сборка бинарника (кроссплатформенно)
build:
	go build -o luna_bot ./cmd/luna_bot

# vet — проверка кода
vet:
	go vet ./...

# clean — очистка артефактов сборки (кроссплатформенно)
clean:
	rm -f luna_bot luna_bot.exe coverage.out coverage.html
	@echo "Очищено."

# === Linting ===

# lint — запуск golangci-lint
lint:
	golangci-lint run ./...

# === Docker ===

# docker-build — сборка Docker-образа
docker-build:
	docker compose build

# docker-up — запуск всех сервисов
docker-up:
	docker compose up -d

# docker-down — остановка всех сервисов
docker-down:
	docker compose down
