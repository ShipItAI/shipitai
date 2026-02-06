.PHONY: test test-coverage lint tidy fmt dev docker-build docker-run clean

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run ./...

# Tidy dependencies
tidy:
	go mod tidy

# Download dependencies
deps:
	go mod download

# Format code
fmt:
	gofmt -s -w .

# Run local development server (reads from environment variables)
dev:
	go run cmd/local/main.go

# Build Docker image for self-hosted deployment
docker-build:
	docker build -t shipitai:latest .

# Run Docker container locally
docker-run:
	docker run --env-file .env -p 8080:8080 shipitai:latest

# Clean build artifacts
clean:
	rm -f bootstrap coverage.out coverage.html
