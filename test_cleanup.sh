#!/bin/bash

# GusSync Cleanup Mode Test Script
# Usage: ./test_cleanup.sh [dest_path] [source_path]
#   dest_path: Destination directory (default: from test_mtp.sh)
#   source_path: Source path (default: auto-detect from device)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}GusSync Cleanup Mode Test Script${NC}"
echo "=========================================="

# Same values as test_mtp.sh
MTP_DEVICE="Xiaomi_Mi_11_Ultra_8997acf"
DEST="/media/nick/New Volume/2026/phone ulta 11 mi xiomi 2026"

# Parse command line arguments
if [ $# -ge 1 ]; then
    DEST="$1"
fi

# Auto-detect source path (same logic as test_mtp.sh)
GVFS_BASE="/run/user/$(id -u)/gvfs"
SOURCE=""

if [ $# -ge 2 ]; then
    SOURCE="$2"
else
    # Try to find the mount point automatically
    echo -e "${YELLOW}Auto-detecting device mount...${NC}"
    
    if [ -d "$GVFS_BASE" ]; then
        # Look for mtp:host entries first (MTP mode)
        for mount in "$GVFS_BASE"/mtp:host=*; do
            if [ -d "$mount" ]; then
                if echo "$mount" | grep -q "$MTP_DEVICE"; then
                    SOURCE="$mount"
                    echo -e "${GREEN}Found MTP mount${NC}"
                    break
                fi
            fi
        done

        # If MTP not found, look for gphoto2:host entries
        if [ -z "$SOURCE" ]; then
            for mount in "$GVFS_BASE"/gphoto2:host=*; do
                if [ -d "$mount" ]; then
                    if echo "$mount" | grep -q "$MTP_DEVICE"; then
                        SOURCE="$mount"
                        echo -e "${GREEN}Found gphoto2 mount${NC}"
                        break
                    fi
                fi
            done
        fi
    fi

    # If still not found, prompt user
    if [ -z "$SOURCE" ]; then
        echo -e "${YELLOW}Could not auto-detect mount. Please provide source path:${NC}"
        echo "Example: /run/user/$(id -u)/gvfs/gphoto2:host=$MTP_DEVICE"
        read -p "Source path: " SOURCE
    fi
fi

# Validate source exists
if [ ! -d "$SOURCE" ]; then
    echo -e "${RED}Error: Source path does not exist: $SOURCE${NC}"
    exit 1
fi

# Validate destination exists
if [ ! -d "$DEST" ]; then
    echo -e "${RED}Error: Destination directory does not exist: $DEST${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}Configuration:${NC}"
echo "  Source: $SOURCE"
echo "  Dest:   $DEST"
echo ""
echo -e "${RED}⚠️  WARNING: This will DELETE verified files from the source!${NC}"
echo -e "${YELLOW}   Only files that match hashes in the state file will be deleted.${NC}"
echo -e "${YELLOW}   Files are verified before deletion (source hash == dest hash == stored hash).${NC}"
echo ""
read -p "Are you sure you want to proceed? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo -e "${YELLOW}Cleanup cancelled.${NC}"
    exit 1
fi

# Check if binary exists
GUSSYNC_BIN="./gussync"
if [ ! -f "$GUSSYNC_BIN" ]; then
    GUSSYNC_BIN="gussync"
    if ! command -v "$GUSSYNC_BIN" &> /dev/null; then
        echo -e "${RED}Error: gussync binary not found${NC}"
        echo "Building..."
        go build -o gussync .
        GUSSYNC_BIN="./gussync"
    fi
fi

echo ""
echo -e "${GREEN}Starting cleanup...${NC}"
echo ""

# Run cleanup mode
"$GUSSYNC_BIN" -source "$SOURCE" -dest "$DEST" -mode cleanup

EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}Cleanup completed successfully!${NC}"
else
    echo -e "${RED}Cleanup exited with code: $EXIT_CODE${NC}"
fi

exit $EXIT_CODE
