# Makefile for FractalBot

.PHONY: build clean test fmt vet run install

# Build the main binary
build:
	@echo "🔨 Building fractalbot..."
	go build -v -o fractalbot ./cmd/fractalbot

# Build for multiple platforms
build-all:
	@echo "🔨 Building for all platforms..."
	@echo "Linux..."
	GOOS=linux GOARCH=amd64 go build -o fractalbot-linux-amd64 ./cmd/fractalbot
	@echo "macOS..."
	GOOS=darwin GOARCH=amd64 go build -o fractalbot-darwin-amd64 ./cmd/fractalbot
	@echo "macOS (ARM64)..."
	GOOS=darwin GOARCH=arm64 go build -o fractalbot-darwin-arm64 ./cmd/fractalbot
	@echo "Windows..."
	GOOS=windows GOARCH=amd64 go build -o fractalbot-windows-amd64.exe ./cmd/fractalbot

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f fractalbot fractalbot-*
	go clean

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v -race ./...

# Format code
fmt:
	@echo "✨ Formatting code..."
	go fmt ./...
	goimports -w . 2>/dev/null || true

# Run go vet
vet:
	@echo "🔍 Running go vet..."
	go vet ./...

# Run all checks
check: fmt vet test
	@echo "✅ All checks passed!"

# Run the application
run:
	@echo "🚀 Starting fractalbot..."
	go run ./cmd/fractalbot

# Run with verbose output
run-verbose:
	@echo "🚀 Starting fractalbot (verbose)..."
	go run ./cmd/fractalbot --verbose

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy

# Download tools
tools:
	@echo "🔧 Installing tools..."
	go install golang.org/x/tools/cmd/goimports@latest

# Development setup
dev:
	@echo "🛠️  Setting up development environment..."
	@make deps
	@make tools

# Run in development mode with hot reload (requires 'air' tool)
dev:
	@which air > /dev/null || (echo "❌ 'air' not installed. Install with: go install github.com/air-verse/air@latest" && exit 1)
	@echo "🔥 Starting development server with hot reload..."
	air

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the main binary"
	@echo "  build-all    - Build for all platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  check        - Run all checks (fmt, vet, test)"
	@echo "  run          - Run the application"
	@echo "  run-verbose  - Run with verbose output"
	@echo "  dev          - Run in development mode with hot reload"
	@echo "  deps         - Install dependencies"
	@echo "  tools        - Install development tools"
