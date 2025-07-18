.PHONY: build test clean release release-dry-run

# Default target
all: build

# Build the application
build: go-imports 
	mkdir -p bin
	@if [ ! -f version.txt ]; then echo "version.txt not found"; exit 1; fi
	$(eval VERSION := $(shell cat version.txt))
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/marvai ./cmd/marvai

# Run tests
test:
	mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./...
	@echo "\n=== COVERAGE SUMMARY ==="
	@go tool cover -func=coverage/coverage.out | tail -1

go-imports:
	goimports -w .

# Clean build artifacts
clean:
	rm -rf bin coverage

# Install dependencies
deps:
	go mod tidy

upgrade-deps:
	go get -u ./...
	go mod tidy

lint:
	golangci-lint run

sec:
	gosec ./...
	govulncheck ./...

tag:
	@if [ ! -f version.txt ]; then echo "version.txt not found"; exit 1; fi
	$(eval VERSION := $(shell cat version.txt))
	@echo "Creating git tag v$(VERSION)"
	git tag -a "v$(VERSION)" -m "Release version $(VERSION)"

# Run tests with coverage
test-coverage:
	mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./... || true
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report generated at coverage/coverage.html"
	@echo "\n=== COVERAGE SUMMARY ==="
	@go tool cover -func=coverage/coverage.out | tail -1

# Release using goreleaser
release:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: go install github.com/goreleaser/goreleaser@latest" && exit 1)
	goreleaser release --clean

# Dry run release (build without releasing)
release-dry-run:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: go install github.com/goreleaser/goreleaser@latest" && exit 1)
	goreleaser release --snapshot --clean