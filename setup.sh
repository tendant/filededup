#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}==== File Deduplication System Setup ====${NC}"

# Check if dbstrap is installed
if ! command -v dbstrap &> /dev/null; then
    echo -e "${RED}dbstrap is not installed. Please install it first.${NC}"
    echo "You can install it with: go install github.com/jackc/dbstrap@latest"
    exit 1
fi

# Set environment variables
export FILEDEDUP_USER_PASSWORD=${FILEDEDUP_USER_PASSWORD:-pwd}
export DATABASE_URL="postgres://postgres:${POSTGRES_PASSWORD:-pwd}@localhost:5432/postgres?sslmode=disable"

echo -e "${BLUE}Setting up database...${NC}"
echo "Using DATABASE_URL: $DATABASE_URL"
echo "FILEDEDUP_USER_PASSWORD is set (not shown for security)"

# Run dbstrap to set up the database
echo -e "${BLUE}Running dbstrap...${NC}"
dbstrap run --config=bootstrap.yaml

# Update DATABASE_URL to point to the filededup database
export DATABASE_URL="postgres://filededup:${FILEDEDUP_USER_PASSWORD}@localhost:5432/filededup?sslmode=disable"

# Initialize schema if not already done
echo -e "${BLUE}Initializing database schema...${NC}"
psql -h localhost -d filededup -U filededup -f pkg/record/recorddb/schema.sql || {
    echo -e "${RED}Failed to initialize schema. This is normal if tables already exist.${NC}"
}

echo -e "${GREEN}Setup complete!${NC}"
echo -e "${BLUE}To run the server:${NC}"
echo "DATABASE_URL=\"$DATABASE_URL\" go run ./cmd/server"
echo -e "${BLUE}To run the agent:${NC}"
echo "go run ./cmd/agent -dir \"/path/to/scan\" -server \"http://localhost:8080\" -machine-id \"my-machine\""
