.PHONY: build run test migrate-up migrate-down migrate-new fmt lint docker-build

APP := compliancekit-api
BIN := ./bin/$(APP)
PKG := ./...
MIGRATIONS_DIR := migrations
DB_URL ?= $(DATABASE_URL)

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race -count=1 $(PKG)

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

migrate-new:
	@if [ -z "$(name)" ]; then echo "usage: make migrate-new name=<snake_case>"; exit 1; fi
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

fmt:
	gofmt -s -w .
	go vet $(PKG)

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PKG); \
	else \
		echo "golangci-lint not installed; falling back to go vet"; \
		go vet $(PKG); \
	fi

docker-build:
	docker build -t $(APP):latest .
