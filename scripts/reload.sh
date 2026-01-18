#!/bin/bash
# Reload script for GusSync Wails development
# Clears Vite cache, rebuilds frontend, kills existing process, and starts wails dev

set -e

echo "🔄 Reloading GusSync Wails development environment..."

# Ensure wails is in PATH
export PATH="$HOME/go/bin:$PATH"
if ! command -v wails &> /dev/null; then
    echo "❌ Error: wails command not found. Please install it:"
    echo "   go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
PID_FILE="$PROJECT_ROOT/.wails-dev.pid"
LOG_FILE="$PROJECT_ROOT/.wails-dev.log"

cd "$PROJECT_ROOT"

# Step 1: Kill existing wails dev process if running
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if ps -p "$OLD_PID" > /dev/null 2>&1; then
        echo "🛑 Stopping existing wails dev process (PID: $OLD_PID)..."
        # Try to kill gracefully first
        kill -TERM "$OLD_PID" 2>/dev/null || true
        sleep 2
        # Force kill if still running
        if ps -p "$OLD_PID" > /dev/null 2>&1; then
            kill -KILL "$OLD_PID" 2>/dev/null || true
            sleep 1
        fi
        echo "✓ Process stopped"
    else
        echo "ℹ️  PID file exists but process is not running, cleaning up..."
    fi
    rm -f "$PID_FILE"
fi

# Also kill any orphaned wails processes (in case PID file was lost)
WAILS_PIDS=$(pgrep -f "wails dev" 2>/dev/null || true)
if [ -n "$WAILS_PIDS" ]; then
    echo "🧹 Cleaning up any orphaned wails processes..."
    echo "$WAILS_PIDS" | xargs kill -TERM 2>/dev/null || true
    sleep 1
    echo "$WAILS_PIDS" | xargs kill -KILL 2>/dev/null || true
fi

# Step 2: Clear Vite cache, old build artifacts, and logs
echo "🧹 Clearing Vite cache, build artifacts, and logs..."
rm -rf frontend/node_modules/.vite
rm -rf app/frontend_dist
rm -rf frontend_dist  # Root-level frontend_dist (used by wails dev)
rm -rf frontend/dist  # Clean up old dist location if it exists
rm -f .wails-dev.log  # Clear wails dev log
# Clear error log if it exists
if [ -f ~/.gussync/logs/errors.log ]; then
    > ~/.gussync/logs/errors.log  # Clear error log file
    echo "✓ Error log cleared"
fi
echo "✓ Caches and logs cleared"

# Step 3: Build frontend (builds to app/frontend_dist for embedding)
echo "📦 Building frontend..."
cd frontend
npm run build
cd ..
echo "✓ Frontend built to app/frontend_dist"

# Step 3.5: Copy to root frontend_dist for wails dev to serve
echo "📋 Copying to root frontend_dist (for wails dev)..."
rm -rf frontend_dist  # Remove old files first
cp -r app/frontend_dist frontend_dist
echo "✓ Assets copied to root frontend_dist"

# Step 4: Check for webkit symlink and determine wails command
echo ""
echo "🔍 Checking WebKit configuration..."
if pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
    echo "✓ WebKit 4.0 symlink found - using 'wails dev' without tags"
    WAILS_CMD="wails dev"
else
    echo "⚠️  WebKit 4.0 not found - using 'wails dev -tags webkit2_41'"
    WAILS_CMD="wails dev -tags webkit2_41"
fi

# Step 5: Start wails dev
echo ""
echo "🚀 Starting wails dev..."
echo "   Logs will be written to: $LOG_FILE"
echo "   To stop, run: kill \$(cat $PID_FILE) or ./scripts/reload.sh"
echo ""

# Start wails dev in background, save PID, redirect output to log file
# Use setsid to create a new session/process group for better control
setsid bash -c "$WAILS_CMD" > "$LOG_FILE" 2>&1 &
NEW_PID=$!
echo $NEW_PID > "$PID_FILE"

# Wait a moment to see if it starts successfully
sleep 2

if ps -p "$NEW_PID" > /dev/null 2>&1; then
    echo "✅ wails dev started successfully (PID: $NEW_PID)"
    echo ""
    echo "📝 Useful commands:"
    echo "   View logs: tail -f $LOG_FILE"
    echo "   Stop: kill \$(cat $PID_FILE)"
    echo "   Or just run this script again to reload"
    echo ""
else
    echo "❌ Failed to start wails dev. Check logs:"
    echo "   tail $LOG_FILE"
    rm -f "$PID_FILE"
    exit 1
fi
