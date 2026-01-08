.PHONY: build clean test lint run install

# Binary name
BINARY_NAME=emed
MAIN_PATH=cmd/emed/main.go

# Build variables
BUILD_DIR=build
INSTALL_PATH ?= /usr/local/bin

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

# Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

build-windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Run the application
run: build
	./$(BINARY_NAME)

# Install the binary
install: build
	sudo install -d $(INSTALL_PATH)
	sudo install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run benchmarks
bench:
	$(GOTEST) -bench=. -benchmem ./...

# Format code
fmt:
	$(GOFMT) -s -w .

# Vet code
vet:
	$(GOVET) ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Update dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Check for dependency updates
deps-check:
	$(GOCMD) list -u -m all

# Development build with debug symbols
dev:
	$(GOBUILD) -gcflags="all=-N -l" -o $(BINARY_NAME) $(MAIN_PATH)

# Quick check (format, vet, build)
check: fmt vet build