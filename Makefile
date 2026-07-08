.PHONY: build test lint docker-up docker-down clean

# Build the server binary
build:
	go build -o bin/data-agent ./cmd/server

# Run tests
test:
	go test ./... -v -count=1

# Run golangci-lint
lint:
	golangci-lint run ./... --timeout=5m

# Start development environment
docker-up:
	docker compose up -d --wait

# Stop development environment
docker-down:
	docker compose down --remove-orphans

# Start UI test environment
docker-up-test:
	docker compose -f docker-compose.ui-test.yml up -d --wait

# Stop UI test environment
docker-down-test:
	docker compose -f docker-compose.ui-test.yml down --remove-orphans

# Clean build artifacts
clean:
	rm -rf bin/ dist/
	go clean ./...
