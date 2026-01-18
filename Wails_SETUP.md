# Wails Setup & Build Issues Fixed

## Issues Fixed

### ✅ 1. go:embed Directive
The `//go:embed all:../frontend/dist` directive in `app/app.go` is correct (line 18). It correctly references the built frontend from the `app/` package directory.

### ✅ 2. .gitignore
Updated to explicitly ignore `frontend/dist/`:
```
node_modules/
dist/
frontend/dist/
*.log
```

### ✅ 3. Service Bindings & Runtime Access

**Current Setup:**
- Services are registered in `app/app.go` via `Methods: []interface{}` (lines 119-127)
- Frontend uses `window.PrereqService` pattern (Path B - window globals)

**How Wails Exposes Services:**

When you run `wails dev` or `wails generate module`, Wails v2 will:
1. Generate TypeScript/JavaScript bindings in `frontend/wailsjs/go/services/`
2. OR inject services as window globals (depending on config)

**Frontend Service Access:**

The frontend code checks for both patterns:
```javascript
// Pattern 1: Window globals (current)
window.PrereqService?.GetPrereqReport()

// Pattern 2: Generated bindings (after wails dev)
import { GetPrereqReport } from '../wailsjs/go/services/PrereqService'
```

**Runtime Access:**

Runtime is accessed via:
```javascript
window.runtime || (window.wails && window.wails.runtime)
```

## How to Run

### Option A: CLI Mode (Existing)
```bash
./gussync -source /path/to/src -dest /path/to/dest -mode mount
```

### Option B: Wails Desktop Mode
You'll need to create a separate entry point or use build tags. For now, the Wails app can be built separately or integrated into the existing main with a flag check.

**Recommended:** Create a separate binary or use build tags:
```go
//go:build wails
// +build wails

package main

import "GusSync/app"

func main() {
    app.Run()
}
```

Then build with: `go build -tags wails -o gussync-wails`

Or simply call `app.Run()` from your existing main with a flag.

## Next Steps

1. **Build frontend:**
   ```bash
   cd frontend
   npm install
   npm run build
   ```

2. **Generate Wails bindings (if using Path A):**
   ```bash
   wails generate module
   # or just run:
   wails dev
   ```

3. **Test the app:**
   ```bash
   wails dev
   ```

## Service Registration

Services registered in `app/app.go`:
- `prereqService` → `window.PrereqService` (or import from wailsjs)
- `deviceService` → `window.DeviceService`
- `copyService` → `window.CopyService`
- `verifyService` → `window.VerifyService`
- `cleanupService` → `window.CleanupService`
- `logService` → `window.LogService`
- `jobManager` → `window.JobManager`

All exported methods from these services will be available in the frontend.


