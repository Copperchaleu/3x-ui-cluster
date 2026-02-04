#!/bin/bash

# Script to verify Slave configuration sync with Master
# This script compares the configuration in Master's database with the actual running config on Slave

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Default values
DB_PATH="${DB_PATH:-./db/x-ui.db}"
SLAVE_ID=""
CONTAINER_NAME=""
CONFIG_PATH="/app/bin/config.json"
VERBOSE=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Verify that Slave configuration matches Master database settings.

OPTIONS:
    -l, --list              List all slaves in database
    -s, --slave-id ID       Slave ID to verify (from database)
    -c, --container NAME    Docker container name (default: use slave name from DB)
    -p, --config-path PATH  Xray config path in container (default: /app/bin/config.json)
    -d, --db PATH           Path to Master database (default: ./db/x-ui.db)
    -v, --verbose           Verbose output
    -h, --help              Show this help message

EXAMPLES:
    # List all available slaves
    $0 --list

    # Verify slave (auto-detect container name from slave name)
    $0 --slave-id 4

    # Verify with explicit container name
    $0 --slave-id 4 --container slave1

    # With custom config path
    $0 --slave-id 4 --config-path /opt/xray/config.json

NOTES:
    - Requires Docker access on the host machine
    - Slave containers must be running
    - Master database must contain Slave configuration
    - Default config path: /app/bin/config.json
    - Container names typically: 3x-ui-slave1, 3x-ui-slave2, etc.

EOF
    exit 1
}

# Parse arguments
LIST_ONLY=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -l|--list)
            LIST_ONLY="yes"
            shift
            ;;
        -s|--slave-id)
            SLAVE_ID="$2"
            shift 2
            ;;
        -c|--container)
            CONTAINER_NAME="$2"
            shift 2
            ;;
        -p|--config-path)
            CONFIG_PATH="$2"
            shift 2
            ;;
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            ;;
    esac
done

# Check if database exists
if [ ! -f "$DB_PATH" ]; then
    echo -e "${RED}❌ Error: Database not found at $DB_PATH${NC}"
    exit 1
fi

# Build verification tool if needed
VERIFY_TOOL="$PROJECT_DIR/bin/verify-slave-config"
if [ ! -f "$VERIFY_TOOL" ] || [ "$PROJECT_DIR/scripts/verify_slave_config.go" -nt "$VERIFY_TOOL" ]; then
    echo -e "${BLUE}🔨 Building verification tool...${NC}"
    cd "$PROJECT_DIR"
    go build -o "$VERIFY_TOOL" scripts/verify_slave_config.go
    echo -e "${GREEN}✅ Build complete${NC}\n"
fi

# Handle --list option
if [ -n "$LIST_ONLY" ]; then
    "$VERIFY_TOOL" -db "$DB_PATH" -list
    exit $?
fi

# Prepare arguments
ARGS="-db $DB_PATH"

if [ -n "$SLAVE_ID" ]; then
    ARGS="$ARGS -slave $SLAVE_ID"
fi

if [ -n "$CONTAINER_NAME" ]; then
    ARGS="$ARGS -container $CONTAINER_NAME"
fi

if [ -n "$CONFIG_PATH" ]; then
    ARGS="$ARGS -config $CONFIG_PATH"
fi

if [ -n "$VERBOSE" ]; then
    ARGS="$ARGS $VERBOSE"
fi

# Run verification
echo -e "${BLUE}🔍 Starting configuration verification...${NC}\n"
"$VERIFY_TOOL" $ARGS

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "\n${GREEN}✅ Verification completed successfully!${NC}"
else
    echo -e "\n${RED}❌ Verification failed. Please check the details above.${NC}"
fi

exit $EXIT_CODE
