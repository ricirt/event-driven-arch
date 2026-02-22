.PHONY: all build run test test-cover lint docker-up docker-down clean

BINARY   = server
MAIN     = ./cmd/server

all: build

## build: compile the binary to bin/server
build:
	go build -ldflags="-s -w" -o bin/$(BINARY) $(MAIN)

## run: run the server locally (requires DATABASE_URL in env or .env)
run:
	go run $(MAIN)

## test: run all tests with race detector
test:
	go test -race -count=1 ./...

## test-cover: run tests and open HTML coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## lint: run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	golangci-lint run ./...

## docker-up: build the Docker image and start all services
docker-up:
	docker compose up --build -d

## docker-down: stop and remove all containers and volumes
docker-down:
	docker compose down -v

## clean: remove compiled binary and coverage artifacts
clean:
	rm -rf bin/ coverage.out coverage.html
