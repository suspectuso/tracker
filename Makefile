.PHONY: build run test clean

# Build the application
build:
	go build -o ton-tracker ./cmd/bot

# Run the application
run:
	go run ./cmd/bot

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f ton-tracker
	rm -f *.db

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Build for Linux (for deployment)
build-linux:
	GOOS=linux GOARCH=amd64 go build -o ton-tracker-linux ./cmd/bot
