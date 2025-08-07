.PHONY: build test lint clean install deps fmt vet

# Binary name and output directory
BINARY_NAME := bundle-tool
BUILD_DIR := bin

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOCLEAN := $(GOCMD) clean
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Build flags
LDFLAGS := -ldflags "-s -w"

# Default target
all: deps fmt vet lint test build

# Install dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	$(GOFMT) ./...

# Vet code
vet:
	$(GOVET) ./...

# Lint code (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

# Run tests
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Build binary
build: deps
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd

# Install binary to GOPATH/bin
install: build
	$(GOCMD) install ./cmd

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Development target - format, vet, and test
dev: fmt vet test

# CI target - comprehensive checks
ci: deps fmt vet lint test build