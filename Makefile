.PHONY: all build run test clean docker-build docker-up docker-down help

# Variables
BINARY_NAME=shopbot
MAIN_PATH=./cmd/server
BUILD_DIR=./build
DOCKER_IMAGE=telegram-shop-bot
DOCKER_TAG=latest

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-w -s"

# Default target
all: test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run $(MAIN_PATH)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f shop.db
	@echo "Clean complete"

# Update dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded"

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy
	@echo "Dependencies tidied"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Code formatted"

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Run with Docker Compose
docker-up:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d
	@echo "Services started. View logs with: docker-compose logs -f"

# Stop Docker Compose services
docker-down:
	@echo "Stopping services..."
	docker-compose down
	@echo "Services stopped"

# View Docker Compose logs
docker-logs:
	docker-compose logs -f

# Run database migrations
migrate-up:
	@echo "Running database migrations..."
	$(GOCMD) run $(MAIN_PATH) migrate up
	@echo "Migrations complete"

# Rollback database migrations
migrate-down:
	@echo "Rolling back database migrations..."
	$(GOCMD) run $(MAIN_PATH) migrate down
	@echo "Rollback complete"

# Create a new migration
migrate-create:
	@read -p "Enter migration name: " name; \
	$(GOCMD) run $(MAIN_PATH) migrate create $$name

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	@cp config.yaml.example config.yaml
	@echo "Please edit config.yaml with your settings"
	$(MAKE) deps
	@echo "Development setup complete"

# Production build
prod-build:
	@echo "Building for production..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "Production build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# Seed database
seed: clean run

# Test payment callback
test-callback:
	@echo "Usage: make test-callback OUT_TRADE_NO=<order_id>-<timestamp> AMOUNT=<amount>"
	@echo "Example: make test-callback OUT_TRADE_NO=1-1234567890 AMOUNT=4.00"
	go run ./cmd/test_callback $(OUT_TRADE_NO) $(AMOUNT)

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Run the application"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make tidy           - Tidy dependencies"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-up      - Start services with Docker Compose"
	@echo "  make docker-down    - Stop Docker Compose services"
	@echo "  make docker-logs    - View Docker Compose logs"
	@echo "  make migrate-up     - Run database migrations"
	@echo "  make migrate-down   - Rollback database migrations"
	@echo "  make migrate-create - Create a new migration"
	@echo "  make dev-setup      - Setup development environment"
	@echo "  make prod-build     - Build for production (Linux AMD64)"
	@echo "  make seed           - Seed database with test data"
	@echo "  make test-callback  - Test payment callback"
	@echo "  make help           - Show this help message"