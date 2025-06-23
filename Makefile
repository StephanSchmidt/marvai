.PHONY: build test clean

# Default target
all: build

# Build the application
build:
	mkdir -p bin
	go build -o bin/marvai ./cmd/marvai

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin coverage

# Install dependencies
deps:
	go mod tidy

# Run tests with coverage
test-coverage:
	mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./... || true
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report generated at coverage/coverage.html"
	@echo "\n=== COVERAGE SUMMARY ==="
	@go tool cover -func=coverage/coverage.out | tail -1