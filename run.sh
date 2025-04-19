#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
FILEDEDUP_USER_PASSWORD=${FILEDEDUP_USER_PASSWORD:-pwd}
DATABASE_URL="postgres://filededup:${FILEDEDUP_USER_PASSWORD}@localhost:5432/filededup?sslmode=disable"
SCAN_DIR=${1:-"."}
MACHINE_ID=${2:-"default"}

echo -e "${BLUE}==== File Deduplication System ====${NC}"
echo "Database URL: $DATABASE_URL"
echo "Scan directory: $SCAN_DIR"
echo "Machine ID: $MACHINE_ID"

# Start the server in the background
echo -e "${BLUE}Starting server...${NC}"
DATABASE_URL="$DATABASE_URL" go run ./cmd/server &
SERVER_PID=$!

# Wait for the server to start
echo "Waiting for server to start..."
sleep 3

# Run the agent
echo -e "${BLUE}Running agent to scan $SCAN_DIR...${NC}"
go run ./cmd/agent -dir "$SCAN_DIR" -server "http://localhost:8080" -machine-id "$MACHINE_ID"

# Check for duplicates
echo -e "${BLUE}Checking for duplicates...${NC}"
DUPES_RESPONSE=$(curl -s http://localhost:8080/duplicates)

if [[ -z "$DUPES_RESPONSE" ]]; then
    echo "No response from server"
elif [[ "$DUPES_RESPONSE" == *"Failed to"* ]]; then
    echo "Error: $DUPES_RESPONSE"
else
    echo "$DUPES_RESPONSE" | jq . || echo "Raw response: $DUPES_RESPONSE"
    
    # Count duplicates
    DUPE_COUNT=$(echo "$DUPES_RESPONSE" | jq '. | length')
    if [[ "$DUPE_COUNT" == "0" ]]; then
        echo -e "${GREEN}No duplicate files found.${NC}"
    else
        echo -e "${GREEN}Found $DUPE_COUNT sets of duplicate files.${NC}"
    fi
fi

# Clean up
echo -e "${BLUE}Stopping server...${NC}"
kill $SERVER_PID

echo -e "${GREEN}Done!${NC}"
