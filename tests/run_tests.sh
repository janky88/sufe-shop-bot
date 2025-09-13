#!/bin/bash

# Run all integration tests
# Usage: ./run_tests.sh [test_name]

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Running Epic Sufe Telegram Shop Bot Integration Tests${NC}"
echo "=================================================="

# Set test environment variables
export GIN_MODE=test
export DB_TYPE=sqlite
export DB_DSN=":memory:"
export ADMIN_TOKEN="test-admin-token"
export JWT_SECRET="test-jwt-secret-key-32-bytes-long"
export ENCRYPTION_KEY="test-encryption-key-32-bytes-long"
export ADMIN_CHAT_ID=12345

# Change to project root
cd "$(dirname "$0")/.."

# Run specific test if provided
if [ $# -gt 0 ]; then
    echo -e "${YELLOW}Running specific test: $1${NC}"
    go test -v ./tests/integration -run "$1" -count=1
else
    echo -e "${YELLOW}Running all integration tests${NC}"
    go test -v ./tests/integration -count=1 -cover
fi

# Check test results
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    
    # Generate coverage report if running all tests
    if [ $# -eq 0 ]; then
        echo -e "${YELLOW}Generating coverage report...${NC}"
        go test ./tests/integration -coverprofile=coverage.out
        go tool cover -html=coverage.out -o coverage.html
        echo -e "${GREEN}✓ Coverage report generated: coverage.html${NC}"
    fi
else
    echo -e "${RED}✗ Tests failed${NC}"
    exit 1
fi