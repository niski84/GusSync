# Wails Bindings Setup

## Current Approach: Window Globals (Path B)

The frontend currently expects services to be available as `window.PrereqService`, `window.DeviceService`, etc.

When you run `wails dev` or `wails generate module`, Wails v2 will generate TypeScript/JavaScript bindings in `frontend/wailsjs/go/services/`.

## Migration to Generated Bindings (Path A - Recommended)

After running `wails dev` once, you should see:

```
frontend/wailsjs/go/services/
  ├── PrereqService.js
  ├── DeviceService.js
  ├── CopyService.js
  ├── VerifyService.js
  ├── CleanupService.js
  ├── LogService.js
  └── JobManager.js
```

Then update frontend imports:

```javascript
// Instead of: window.PrereqService.GetPrereqReport()
import { GetPrereqReport, RefreshNow } from '../wailsjs/go/services/PrereqService'

// Use: GetPrereqReport().then(report => ...)
```

## Current Service Access Pattern

Services are registered in `app/app.go` via `Methods: []interface{}`. Wails will expose them either:
1. As generated bindings (preferred)
2. As window globals (current fallback)

The frontend code checks for both patterns:
- `window.PrereqService` (window global)
- Imported from `wailsjs/go/services/` (generated bindings)

## Testing

Run `wails dev` to generate bindings and test the app.


