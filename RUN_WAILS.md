# Running GusSync Wails UI

## Prerequisites (Linux)

Install required GTK/WebKit dependencies:

```bash
sudo apt-get update
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev
```

## Running the Dev Server

```bash
# Make sure Wails CLI is in PATH
export PATH="$HOME/go/bin:$PATH"

# Run the dev server
wails dev
```

## Accessing the UI

**The Wails UI is a native desktop application window, NOT a web browser.**

When you run `wails dev`, it will:
1. Build the Go backend
2. Start the Vite frontend dev server (usually on `http://localhost:34115`)
3. **Automatically open a native window** with the Wails application

**You don't need to navigate anywhere manually** - the window should open automatically.

### If the Window Doesn't Open

1. Check terminal output for errors
2. Look for a window titled "GusSync" 
3. The window may appear behind other windows - check your taskbar/dock
4. Check the terminal for the dev server URL (e.g., `Using DevServer URL: http://localhost:34115`)

### Frontend Routes

Once the app is running, the UI has these pages:
- **Home** (`/`) - Dashboard with overall status
- **Prerequisites** (`/prereqs`) - System prerequisite checks and fixes
- **Logs** (`/logs`) - Application logs

These are accessed via the navigation menu in the app, not via browser URLs.

## Building for Production

```bash
wails build
```

This creates a binary in `build/bin/` that you can run directly.

## Troubleshooting

### "Connection Refused" Error

This usually means the build failed. Check terminal output for:
- Missing GTK/WebKit libraries (install with command above)
- Go compilation errors
- Missing frontend dependencies (run `cd frontend && npm install`)

### Build Fails with GTK/WebKit Errors

```bash
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev
```

### Frontend Not Building

```bash
cd frontend
npm install
npm run build
```

### Wails Command Not Found

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
export PATH="$HOME/go/bin:$PATH"
```
