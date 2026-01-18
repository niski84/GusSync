#!/bin/bash

# Test script for GusSync with MTP device or ADB
# Usage: ./test_mtp.sh [mode] [workers]
#   mode: "mount" (default) or "adb"
#   workers: number of worker threads (optional)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}GusSync Test Script${NC}"
echo "=================================="

# Get mode from first argument (default: mount)
MODE=${1:-mount}

# Get workers from second argument (optional)
WORKERS=${2:-}

# MTP device identifier
MTP_DEVICE="Xiaomi_Mi_11_Ultra_8997acf"

# Destination
DEST="/media/nick/New Volume/2026/phone ulta 11 mi xiomi 2026"

# Validate mode
if [ "$MODE" != "mount" ] && [ "$MODE" != "adb" ]; then
    echo -e "${RED}Error: Invalid mode '$MODE'. Must be 'mount' or 'adb'${NC}"
    echo "Usage: $0 [mount|adb] [workers]"
    exit 1
fi

# Determine source path based on mode
if [ "$MODE" == "adb" ]; then
    echo -e "${YELLOW}ADB Mode Selected${NC}"
    SOURCE="/sdcard"
    
    # Check if adb is available
    if ! command -v adb &> /dev/null; then
        echo -e "${RED}Error: adb command not found${NC}"
        echo ""
        echo -e "${YELLOW}Android Debug Bridge (adb) is required for ADB mode.${NC}"
        
        # Detect package manager and suggest installation
        if command -v apt &> /dev/null; then
            echo -e "${YELLOW}On Ubuntu/Debian, you can install it with:${NC}"
            echo "  sudo apt install google-android-platform-tools-installer"
            echo ""
            read -p "Would you like to install it now? (y/n): " INSTALL_ADB
            if [ "$INSTALL_ADB" = "y" ] || [ "$INSTALL_ADB" = "Y" ]; then
                echo -e "${YELLOW}Installing adb...${NC}"
                sudo apt install -y google-android-platform-tools-installer
                if [ $? -eq 0 ]; then
                    echo -e "${GREEN}adb installed successfully!${NC}"
                else
                    echo -e "${RED}Failed to install adb. Please install it manually.${NC}"
                    exit 1
                fi
            else
                echo -e "${YELLOW}Please install adb and try again.${NC}"
                exit 1
            fi
        else
            echo "Please install Android Debug Bridge (adb) for your system."
            exit 1
        fi
    fi
    
    # Check if device is connected
    if ! adb devices | grep -q "device$"; then
        echo -e "${RED}Error: No Android device connected via ADB${NC}"
        echo -e "${YELLOW}Make sure USB debugging is enabled and device is authorized${NC}"
        echo "Run: adb devices"
        exit 1
    fi
    
    echo -e "${GREEN}ADB device connected${NC}"
else
    echo -e "${YELLOW}Mount Mode Selected${NC}"
    
    # Find GVFS mount point (supports both MTP and gphoto2)
    GVFS_BASE="/run/user/$(id -u)/gvfs"
    echo -e "${YELLOW}Looking for device mount in GVFS (MTP or gphoto2)...${NC}"

    # Try to find the mount point
    MOUNT_POINT=""
    if [ -d "$GVFS_BASE" ]; then
        # Look for mtp:host entries first (MTP mode)
        for mount in "$GVFS_BASE"/mtp:host=*; do
            if [ -d "$mount" ]; then
                # Check if this matches our device (partial match)
                if echo "$mount" | grep -q "$MTP_DEVICE"; then
                    MOUNT_POINT="$mount"
                    echo -e "${GREEN}Found MTP mount${NC}"
                    break
                fi
            fi
        done

        # If MTP not found, look for gphoto2:host entries
        if [ -z "$MOUNT_POINT" ]; then
            for mount in "$GVFS_BASE"/gphoto2:host=*; do
                if [ -d "$mount" ]; then
                    # Check if this matches our device (partial match)
                    if echo "$mount" | grep -q "$MTP_DEVICE"; then
                        MOUNT_POINT="$mount"
                        echo -e "${GREEN}Found gphoto2 mount${NC}"
                        break
                    fi
                fi
            done
        fi

        # If not found by device name, list available mounts
        if [ -z "$MOUNT_POINT" ]; then
            echo -e "${YELLOW}Device not found by exact name. Available mounts:${NC}"
            echo -e "${YELLOW}MTP mounts:${NC}"
            ls -1 "$GVFS_BASE"/mtp:host=* 2>/dev/null || echo "  (none)"
            echo -e "${YELLOW}gphoto2 mounts:${NC}"
            ls -1 "$GVFS_BASE"/gphoto2:host=* 2>/dev/null || echo "  (none)"
            echo ""
            echo -e "${YELLOW}Please select one of the above or provide the full path:${NC}"
            read -p "Enter mount path (or press Enter to search): " USER_MOUNT
            if [ -n "$USER_MOUNT" ] && [ -d "$USER_MOUNT" ]; then
                MOUNT_POINT="$USER_MOUNT"
            fi
        fi
    fi

    # If still not found, prompt user
    if [ -z "$MOUNT_POINT" ]; then
        echo -e "${RED}Could not automatically find device mount.${NC}"
        echo -e "${YELLOW}Please provide the full path to the mount:${NC}"
        echo "Example: /run/user/$(id -u)/gvfs/mtp:host=Xiaomi_Mi_11_Ultra_8997acf"
        echo "Example: /run/user/$(id -u)/gvfs/gphoto2:host=Xiaomi_Mi_11_Ultra_8997acf"
        read -p "Mount path: " MOUNT_POINT
    fi

    # Validate mount exists
    if [ ! -d "$MOUNT_POINT" ]; then
        echo -e "${RED}Error: Mount point does not exist: $MOUNT_POINT${NC}"
        echo -e "${YELLOW}Make sure the phone is connected and unlocked.${NC}"
        echo -e "${YELLOW}Try opening it in Nemo first to ensure it's mounted.${NC}"
        exit 1
    fi

    SOURCE="$MOUNT_POINT"
    echo -e "${GREEN}Found mount: $SOURCE${NC}"
fi

# Validate destination
if [ ! -d "$(dirname "$DEST")" ]; then
    echo -e "${RED}Error: Destination parent directory does not exist: $(dirname "$DEST")${NC}"
    exit 1
fi

# Create destination if it doesn't exist
mkdir -p "$DEST"

echo ""
echo -e "${GREEN}Configuration:${NC}"
echo "  Mode:   $MODE"
echo "  Source: $SOURCE"
echo "  Dest:   $DEST"
if [ -n "$WORKERS" ]; then
    echo "  Workers: $WORKERS"
else
    echo "  Workers: (default - CPU cores)"
fi
echo ""

# Check if gussync binary exists
GUSSYNC_BIN="./gussync"
if [ ! -f "$GUSSYNC_BIN" ]; then
    GUSSYNC_BIN="gussync"
    if ! command -v "$GUSSYNC_BIN" &> /dev/null; then
        echo -e "${RED}Error: gussync binary not found${NC}"
        echo "Please build it first: go build -o gussync ."
        exit 1
    fi
fi

echo -e "${GREEN}Starting GusSync...${NC}"
echo ""

# Run gussync with mode and optional workers flag
if [ -n "$WORKERS" ]; then
    "$GUSSYNC_BIN" -source "$SOURCE" -dest "$DEST" -mode "$MODE" -workers "$WORKERS"
else
    "$GUSSYNC_BIN" -source "$SOURCE" -dest "$DEST" -mode "$MODE"
fi

EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}GusSync completed successfully!${NC}"
else
    echo -e "${RED}GusSync exited with code: $EXIT_CODE${NC}"
fi

exit $EXIT_CODE
