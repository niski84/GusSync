# GusSync UI Architecture

This document describes the UI technology stack, event subscription patterns, and how the frontend communicates with the Go backend.

---

## Technology Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Desktop Container** | [Wails v2](https://wails.io/) | Native window hosting, Go ↔ JS bindings |
| **UI Framework** | React 18 | Component rendering and state management |
| **Styling** | Tailwind CSS | Utility-first CSS styling |
| **Build Tool** | Vite | Fast dev server and production builds |
| **State** | React Context + Hooks | Global state via `store.jsx`, local state via custom hooks |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (React)                          │
│  ┌─────────────┐  ┌────────────────┐  ┌──────────────────────┐  │
│  │ Dashboard   │  │ Prerequisites  │  │ Logs Page            │  │
│  │ Component   │  │ Page           │  │                      │  │
│  └──────┬──────┘  └───────┬────────┘  └──────────┬───────────┘  │
│         │                 │                      │               │
│         └─────────────────┼──────────────────────┘               │
│                           ▼                                      │
│                 ┌──────────────────┐                             │
│                 │ useBackupState() │  Custom hook for state      │
│                 │ store.jsx        │  Global context             │
│                 └────────┬─────────┘                             │
│                          │                                       │
│    ┌─────────────────────┼─────────────────────┐                 │
│    ▼                     ▼                     ▼                 │
│ EventsOn()         window.go.*           fetch() to API          │
│ (Wails events)     (Go bindings)         (HTTP API)              │
└────────────────────────────────────────────────────────────────┘
         │                     │                     │
         ▼                     ▼                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Backend (Go)                              │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    Wails Runtime                           │  │
│  │  • EventsEmit() → pushes events to frontend                │  │
│  │  • Bound methods → exposed as window.go.services.*         │  │
│  └───────────────────────────────────────────────────────────┘  │
│                               │                                  │
│  ┌─────────────────┐  ┌──────┴──────┐  ┌──────────────────────┐ │
│  │ services/       │  │ core/       │  │ adapters/api/        │ │
│  │ JobManager      │  │ JobManager  │  │ HTTP Server          │ │
│  │ PrereqService   │  │ (business   │  │ REST + SSE           │ │
│  │ DeviceService   │  │  logic)     │  │                      │ │
│  │ CopyService     │  │             │  │                      │ │
│  └─────────────────┘  └─────────────┘  └──────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Event Subscription Patterns

### 1. Wails Events (Primary for Desktop UI)

The desktop UI uses Wails' built-in event system for real-time updates.

**Backend emits events:**
```go
// In Go backend (services/jobmanager.go)
runtime.EventsEmit(ctx, "task:update", taskEvent)
runtime.EventsEmit(ctx, "device:list", devices)
runtime.EventsEmit(ctx, "PrereqReport", report)
```

**Frontend subscribes:**
```javascript
// In React hook (hooks/useBackupState.js)
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

useEffect(() => {
  // Subscribe to task updates
  const cleanup = EventsOn('task:update', (task) => {
    console.log('Task update:', task)
    processTaskUpdate(task)
  })

  // Cleanup on unmount
  return () => cleanup()
}, [])
```

### 2. Startup Handshake Pattern

**Problem:** If the UI loads while a job is running, it might miss earlier events.

**Solution:** On mount, fetch current state before subscribing to events:

```javascript
useEffect(() => {
  // 1. STARTUP HANDSHAKE: Fetch current state
  const fetchActiveTask = async () => {
    const task = await window.go.services.JobManager.GetActiveTask()
    if (task) {
      processTaskUpdate(task)
    }
  }
  fetchActiveTask()

  // 2. Then subscribe to live events
  const cleanup = EventsOn('task:update', processTaskUpdate)
  
  return () => cleanup()
}, [])
```

### 3. Sequence Numbers (Out-of-Order Protection)

**Problem:** Network delays can cause events to arrive out of order.

**Solution:** Every event includes a `seq` number. Ignore events with `seq <= lastSeenSeq`.

```javascript
const lastSeqRef = useRef(0)

const processTaskUpdate = (task) => {
  const taskSeq = task.seq || 0
  
  // Ignore out-of-order events
  if (taskSeq > 0 && taskSeq <= lastSeqRef.current) {
    console.log('Ignoring stale event, seq:', taskSeq)
    return false
  }
  
  lastSeqRef.current = taskSeq
  // ... process the update
}
```

---

## Event Types

### Task Events (`task:update`)

```typescript
interface TaskUpdateEvent {
  taskId: string
  seq: number              // Monotonic sequence number
  type: string             // "copy", "verify", "cleanup"
  state: TaskState         // "queued" | "running" | "succeeded" | "failed" | "canceled"
  progress: {
    phase: string          // "starting" | "scanning" | "copying" | "verifying" | "cleaning"
    current: number        // Files/bytes processed
    total: number          // Total files/bytes
    percent: number        // 0-100
    rate: number           // MB/s
  }
  message: string          // Human-readable status
  logLine?: string         // Optional log line
  workers?: Record<int, string>  // Worker status map
  error?: {
    code: string
    message: string
    details: string
  }
  artifact?: {
    logPath: string
    openLogHint: string
  }
}

type TaskState = "queued" | "running" | "succeeded" | "failed" | "canceled"
```

### Device Events (`device:list`)

```typescript
interface DeviceInfo {
  id: string
  model: string
  displayName: string
  path: string
  protocol: "mtp" | "adb"
}

// Event payload is DeviceInfo[]
```

### Prerequisite Events (`PrereqReport`)

```typescript
interface PrereqReport {
  overallStatus: "ok" | "warn" | "fail"
  seq: number
  os: string
  checks: PrereqCheck[]
  timestamp: string
}

interface PrereqCheck {
  id: string
  name: string
  status: "ok" | "warn" | "fail"
  details: string
  remediationSteps: string[]
}
```

---

## Go Bindings (Direct Method Calls)

Wails auto-generates JavaScript bindings for exported Go methods.

**Location:** `frontend/wailsjs/go/services/`

### Available Services

| Service | Methods | Purpose |
|---------|---------|---------|
| `JobManager` | `GetActiveTask()`, `ListTasks()`, `CancelTask(id)` | Job lifecycle |
| `CopyService` | `StartBackup(src, dest, mode)` | Start backups |
| `DeviceService` | `GetDeviceStatus()` | Get connected devices |
| `PrereqService` | `GetPrereqReport()`, `RefreshNow()` | Prerequisites |
| `ConfigService` | `GetConfig()`, `SetDestinationPath(path)` | Configuration |
| `LogService` | `GetLogContent(path)` | Read log files |
| `SystemService` | `OpenFileManager(path)`, `OpenInBrowser(url)` | OS integration |

### Usage in React

```javascript
// Direct method call (returns Promise)
const devices = await window.go.services.DeviceService.GetDeviceStatus()

// Start a backup
const taskId = await window.go.services.CopyService.StartBackup(
  sourcePath,
  destPath,
  "smart"  // mode: "smart" | "full" | "verify"
)

// Cancel active task
await window.go.services.JobManager.CancelTask(taskId)
```

---

## HTTP API (Optional, for Remote Control)

When enabled via `GUSSYNC_API_PORT=8090`, an HTTP API is available.

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Server health check |
| GET | `/api/jobs` | List all jobs |
| GET | `/api/jobs/active` | Get active job |
| GET | `/api/jobs/:id` | Get specific job |
| DELETE | `/api/jobs/:id` | Cancel job |
| GET | `/api/events` | SSE event stream |
| GET | `/api/prereqs` | Prerequisites report |
| GET | `/api/devices` | Device status |
| GET | `/api/config` | Current configuration |
| POST | `/api/copy/start` | Start copy operation |

### SSE Event Stream

```bash
# Subscribe to real-time events
curl -N http://localhost:8090/api/events

# Output:
event: connected
data: {"message":"Connected to GusSync event stream"}

event: job:update
data: {"jobId":"copy-123","seq":5,"state":"running","progress":{"percent":45}}

event: job:completed
data: {"jobId":"copy-123","seq":10,"state":"succeeded"}
```

### Usage from External Tools

```bash
# Check if backup is running
curl -s http://localhost:8090/api/jobs/active | jq .

# Start a backup via API
curl -X POST http://localhost:8090/api/copy/start

# Monitor progress
curl -N http://localhost:8090/api/events | while read line; do echo "$line"; done
```

---

## State Management

### Global State (`store.jsx`)

Global state is managed via React Context:

```javascript
// store.jsx
export const AppContext = createContext()

export function AppProvider({ children }) {
  const [prereqReport, setPrereqReport] = useState(null)
  
  // Sequence-protected setter
  const setPrereqReportWithSeq = useCallback((newReport) => {
    setPrereqReport((prev) => {
      if (!prev || newReport.seq > prev.seq) {
        return newReport
      }
      return prev  // Ignore stale update
    })
  }, [])

  return (
    <AppContext.Provider value={{ prereqReport, setPrereqReport: setPrereqReportWithSeq }}>
      {children}
    </AppContext.Provider>
  )
}
```

### Local State (`useBackupState` hook)

Task-related state is managed by the `useBackupState` custom hook:

```javascript
const {
  activeTask,      // Current task snapshot
  isRunning,       // Boolean: is a task running?
  progress,        // 0-100 percentage
  status,          // "idle" | "running" | "success" | "error"
  deviceConnected, // Boolean: is a device connected?
  workers,         // Worker status map
  cancelTask,      // Function to cancel
} = useBackupState()
```

---

## File Structure

```
frontend/
├── src/
│   ├── main.jsx              # App entry point
│   ├── App.jsx               # Root component with routing
│   ├── store.jsx             # Global state context
│   ├── index.css             # Tailwind imports
│   ├── components/
│   │   ├── Dashboard.jsx     # Main backup UI
│   │   ├── CheckCard.jsx     # Prerequisite check cards
│   │   ├── Sidebar.jsx       # Navigation sidebar
│   │   └── StatusBadge.jsx   # Status indicator badges
│   ├── hooks/
│   │   └── useBackupState.js # Task state management hook
│   └── pages/
│       ├── Home.jsx          # Home page (Dashboard)
│       ├── Prerequisites.jsx # Prerequisites page
│       └── Logs.jsx          # Logs viewer page
├── wailsjs/
│   ├── runtime/
│   │   └── runtime.js        # Wails runtime (EventsOn, etc.)
│   └── go/
│       ├── models.ts         # Generated TypeScript types
│       └── services/         # Generated Go bindings
│           ├── JobManager.js
│           ├── CopyService.js
│           ├── DeviceService.js
│           └── ...
├── package.json
├── vite.config.js
└── tailwind.config.js
```

---

## Key Principles

1. **Snapshots are truth, events are hints** - Always fetch current state on startup; use events for real-time updates.

2. **Sequence numbers prevent regressions** - Never apply an update with `seq <= lastSeenSeq`.

3. **Backend owns all state** - The UI is a "dumb renderer" that reflects backend state.

4. **Multiple adapters, one core** - The same job logic works via Wails UI, CLI, or HTTP API.

5. **Graceful degradation** - If events are missed, the UI recovers via snapshot fetch.

---

## See Also

- [`frontend/tasks-model.md`](frontend/tasks-model.md) - Architectural contract
- [`ARCHITECTURE.md`](ARCHITECTURE.md) - CLI and engine architecture
- [`README_UI.md`](README_UI.md) - User-facing UI documentation

