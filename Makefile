.PHONY: build test test-cover test-cover-html test-cover-check lint docker-up docker-down docker-up-test docker-down-test clean

# Build the server binary
build:
	go build -o bin/data-agent ./cmd/server

# Run tests
test:
	go test ./... -v -count=1

# Run tests with coverage
test-cover:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out | grep total

# Generate HTML coverage report
test-cover-html:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

# Check coverage threshold
test-cover-check:
	@go test -race -coverprofile=coverage.out ./internal/... || exit 1
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE < 10" | bc -l) -eq 1 ]; then \
		echo "ERROR: Coverage $$COVERAGE% below 10% threshold"; \
		exit 1; \
	fi

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
