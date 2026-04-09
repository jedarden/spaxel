# Makefile for Spaxel CSI Simulator

# Variables
BINARY_NAME=spaxel-sim
GO=go
GOFLAGS=-v
SRC_DIR=.

# Build variables
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

# Default target
.PHONY: all
all: build

# Build the simulator binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	cd $(SRC_DIR) && $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)
	@echo "Build complete: $(BINARY_NAME)"

# Build for multiple platforms
.PHONY: build-all
build-all: build-linux build-macos build-windows

.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	cd $(SRC_DIR) && GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64
	cd $(SRC_DIR) && GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64

.PHONY: build-macos
build-macos:
	@echo "Building for macOS..."
	cd $(SRC_DIR) && GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64
	cd $(SRC_DIR) && GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64

.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	cd $(SRC_DIR) && GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f coverage.out coverage.html

# Install the simulator to $GOPATH/bin
.PHONY: install
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(GOFLAGS) $(LDFLAGS) ./...

# Format code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Lint code
.PHONY: lint
lint:
	@which golangci-lint >/dev/null || (echo "golangci-lint not installed. Run: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin latest" && exit 1)
	golangci-lint run

# Run a quick integration test (requires mothership running on localhost:8080)
.PHONY: integration-test
integration-test: build
	@echo "Running integration test..."
	@echo "Make sure the mothership is running on localhost:8080"
	./$(BINARY_NAME) --mothership ws://localhost:8080/ws/node --nodes 2 --walkers 1 --duration 5s

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all            - Build the simulator (default)"
	@echo "  build          - Build the simulator binary"
	@echo "  build-all      - Build for all platforms"
	@echo "  build-linux    - Build for Linux only"
	@echo "  build-macos    - Build for macOS only"
	@echo "  build-windows  - Build for Windows only"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Remove build artifacts"
	@echo "  install        - Install to GOPATH/bin"
	@echo "  fmt            - Format code"
	@echo "  lint           - Run linter"
	@echo "  integration-test - Quick integration test (requires mothership)"
	@echo "  help           - Show this help message"
