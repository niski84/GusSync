# Core + Adapters Architecture

This document explains how GusSync separates core business logic from interface-specific code, allowing the CLI, Wails Desktop UI, and HTTP API to share the same underlying logic.

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              ADAPTERS                                    │
│  ┌──────────────┐   ┌──────────────────┐   ┌──────────────────────────┐ │
│  │     CLI      │   │   Wails (UI)     │   │      HTTP API            │ │
│  │  cli/main.go │   │ app/services/    │   │ internal/adapters/api/   │ │
│  │              │   │                  │   │                          │ │
│  │ • Cobra/flag │   │ • EventsEmit()   │   │ • REST endpoints         │ │
│  │ • --json mode│   │ • Go bindings    │   │ • SSE streaming          │ │
│  │ • Console    │   │ • Window mgmt    │   │ • JSON responses         │ │
│  └──────┬───────┘   └────────┬─────────┘   └────────────┬─────────────┘ │
│         │                    │                          │               │
│         │    Implements      │    Implements            │ Implements    │
│         │  JobEventEmitter   │  JobEventEmitter         │ JobEventEmitter│
│         │                    │                          │               │
└─────────┼────────────────────┼──────────────────────────┼───────────────┘
          │                    │                          │
          └────────────────────┼──────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                               CORE                                       │
│                        internal/core/                                    │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                         JobManager                                  │ │
│  │  • StartJob(ctx, type, message, params) → (jobID, ctx, error)      │ │
│  │  • UpdateProgress(jobID, progress, message, workers)               │ │
│  │  • CompleteJob(jobID, message)                                     │ │
│  │  • FailJob(jobID, error, details)                                  │ │
│  │  • CancelJob(jobID) → error                                        │ │
│  │  • GetJob(jobID) → JobSnapshot                                     │ │
│  │  • GetActiveJob() → JobSnapshot                                    │ │
│  │  • ListJobs() → []JobSnapshot                                      │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  Types: JobSnapshot, JobUpdateEvent, JobProgress, JobError, JobArtifact │
│  Interface: JobEventEmitter                                              │
│                                                                          │
│  Rules:                                                                  │
│  • NO imports from Wails, Cobra, HTTP frameworks                        │
│  • NO UI-specific code                                                  │
│  • Fully testable without any adapter                                   │
└─────────────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                              ENGINE                                      │
│                            pkg/engine/                                   │
│  • File copying logic                                                    │
│  • Directory walking                                                     │
│  • Hash verification                                                     │
│  • MTP/ADB adapters                                                      │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## The JobEventEmitter Interface

The key to adapter independence is the `JobEventEmitter` interface. Each adapter implements this interface to receive job updates in its own way.

```go
// internal/core/job.go

// JobEventEmitter is the interface adapters must implement to receive job events.
// This allows the core JobManager to be agnostic about how events are delivered.
type JobEventEmitter interface {
    EmitJobUpdate(event JobUpdateEvent)
}
```

### How Each Adapter Implements It

| Adapter | Implementation | What It Does |
|---------|---------------|--------------|
| **Wails** | `WailsJobEmitter` | Calls `runtime.EventsEmit()` to push to React frontend |
| **API** | `api.Server` | Broadcasts to SSE clients via channels |
| **CLI** | `JSONReporter` | Writes JSON to stdout (in `--json` mode) |

---

## Adapter Details

### 1. CLI Adapter (`cli/`)

The CLI is a standalone binary that uses the engine directly (currently doesn't use core.JobManager, but could be refactored to).

**Entry Point:** `cli/main.go`

```go
// CLI flags
flag.StringVar(&sourcePath, "source", "", "Source directory")
flag.StringVar(&destPath, "dest", "", "Destination directory")
flag.BoolVar(&jsonOutput, "json", false, "Output JSON events")

// Choose reporter based on --json flag
var reporter Reporter
if jsonOutput {
    reporter = &JSONReporter{}  // Structured output
} else {
    reporter = &ConsoleReporter{}  // Human-readable
}
```

**Output Modes:**

```bash
# Human-readable (default)
$ gussync -source /phone -dest /backup
Copying: photo.jpg (45%) 12.5 MB/s

# Machine-readable JSON
$ gussync -source /phone -dest /backup --json
{"event":"start","jobId":"copy-123","timestamp":"..."}
{"event":"progress","percent":45,"rate":12.5}
{"event":"complete","filesCount":1234}
```

---

### 2. Wails Adapter (`app/services/`)

The Wails adapter wraps core.JobManager and translates events for the React frontend.

**Files:**
- `app/services/jobmanager.go` - Wraps core.JobManager
- `app/services/task_types.go` - Frontend-compatible types

```go
// app/services/jobmanager.go

// WailsJobEmitter implements core.JobEventEmitter
type WailsJobEmitter struct {
    ctx    context.Context
    logger *log.Logger
}

func (e *WailsJobEmitter) EmitJobUpdate(event core.JobUpdateEvent) {
    // Convert to frontend-compatible format
    taskEvent := TaskUpdateEvent{
        TaskID:   event.JobID,
        Seq:      event.Seq,
        State:    TaskState(event.State),
        Progress: TaskProgress(event.Progress),
        // ...
    }
    
    // Push to React frontend
    runtime.EventsEmit(e.ctx, "task:update", taskEvent)
}

// JobManager wraps core.JobManager for Wails
type JobManager struct {
    core    *core.JobManager  // The shared core
    emitter *WailsJobEmitter
    ctx     context.Context
}

func NewJobManager(ctx context.Context, logger *log.Logger) *JobManager {
    emitter := &WailsJobEmitter{ctx: ctx, logger: logger}
    coreManager := core.NewJobManager(emitter)  // Inject emitter
    
    return &JobManager{
        core:    coreManager,
        emitter: emitter,
    }
}
```

**Frontend receives events via:**
```javascript
// React hook
EventsOn('task:update', (task) => {
    setActiveTask(task)
})
```

---

### 3. HTTP API Adapter (`internal/adapters/api/`)

The API adapter provides REST endpoints and SSE streaming.

**Files:**
- `internal/adapters/api/server.go` - HTTP server setup
- `internal/adapters/api/handlers.go` - Route handlers
- `internal/adapters/api/sse.go` - SSE streaming

```go
// internal/adapters/api/server.go

// Server implements core.JobEventEmitter
type Server struct {
    jobManager *core.JobManager
    sseClients map[chan core.JobUpdateEvent]struct{}
    // ...
}

// EmitJobUpdate broadcasts to all SSE clients
func (s *Server) EmitJobUpdate(event core.JobUpdateEvent) {
    s.sseClientsMu.Lock()
    defer s.sseClientsMu.Unlock()
    
    for clientChan := range s.sseClients {
        select {
        case clientChan <- event:
        default:
            // Client slow, skip
        }
    }
}

// NewServer creates API server with shared core.JobManager
func NewServer(port int, logger *log.Logger, jobManager *core.JobManager, opts ...ServerOption) *Server {
    s := &Server{
        jobManager: jobManager,  // Use the same core
        sseClients: make(map[chan core.JobUpdateEvent]struct{}),
    }
    // ...
}
```

**Clients receive events via SSE:**
```bash
$ curl -N http://localhost:8090/api/events
event: connected
data: {"message":"Connected to GusSync event stream"}

event: job:update
data: {"jobId":"copy-123","seq":5,"progress":{"percent":45}}
```

---

## Multiple Emitters (MultiEmitter)

When both Wails UI and HTTP API are active, events go to both:

```go
// internal/core/job.go

// MultiEmitter broadcasts to multiple emitters
type MultiEmitter struct {
    mu       sync.Mutex
    emitters []JobEventEmitter
}

func (m *MultiEmitter) EmitJobUpdate(event JobUpdateEvent) {
    m.mu.Lock()
    emitters := make([]JobEventEmitter, len(m.emitters))
    copy(emitters, m.emitters)
    m.mu.Unlock()
    
    for _, e := range emitters {
        e.EmitJobUpdate(event)  // Broadcast to all
    }
}

// AddEmitter registers an additional emitter
func (jm *JobManager) AddEmitter(emitter JobEventEmitter) {
    // Wraps existing + new in MultiEmitter
}
```

**Usage in app.go:**
```go
// Start API server
apiServer := api.NewServer(port, logger, coreJobManager, ...)

// Register API as additional emitter (Wails emitter is already registered)
coreJobManager.AddEmitter(apiServer)
```

---

## Data Flow Example: Starting a Backup

### Via Wails UI (React)

```
React Component
    │
    │ await window.go.services.CopyService.StartBackup(src, dest, mode)
    ▼
app/services/copy.go (Wails binding)
    │
    │ jm.startTask("copy", "Starting backup...", params)
    ▼
app/services/jobmanager.go
    │
    │ jm.core.StartJob(ctx, "copy", msg, params)
    ▼
internal/core/job.go
    │
    │ Creates JobSnapshot, emits via emitter
    ▼
WailsJobEmitter.EmitJobUpdate(event)
    │
    │ runtime.EventsEmit(ctx, "task:update", taskEvent)
    ▼
React (EventsOn listener receives update)
```

### Via HTTP API

```
curl -X POST http://localhost:8090/api/copy/start
    │
    ▼
internal/adapters/api/handlers.go
    │
    │ s.startCopyFunc(ctx, req)  // Calls into app services
    ▼
app/services/copy.go
    │
    │ jm.startTask("copy", "Starting backup...", params)
    ▼
internal/core/job.go
    │
    │ Creates JobSnapshot, emits via MultiEmitter
    ▼
    ├──► WailsJobEmitter → React UI
    │
    └──► api.Server.EmitJobUpdate → SSE clients
```

### Via CLI

```
$ gussync -source /phone -dest /backup --json
    │
    ▼
cli/main.go
    │
    │ engine.Copy(ctx, src, dest, reporter)
    ▼
pkg/engine/copy.go
    │
    │ reporter.OnProgress(...)
    ▼
cli/reporter.go (JSONReporter)
    │
    │ json.Marshal → stdout
    ▼
{"event":"progress","percent":45}
```

---

## Directory Structure

```
GusSync/
├── internal/
│   ├── core/                    # CORE (no external dependencies)
│   │   ├── job.go               # JobManager, JobSnapshot, JobEventEmitter
│   │   └── job_test.go          # Unit tests (no UI needed)
│   │
│   └── adapters/
│       └── api/                 # HTTP API Adapter
│           ├── server.go        # HTTP server, implements JobEventEmitter
│           ├── handlers.go      # REST endpoint handlers
│           ├── sse.go           # SSE streaming
│           └── types.go         # API-specific types
│
├── app/
│   ├── app.go                   # Wails app lifecycle
│   └── services/                # Wails Adapter
│       ├── jobmanager.go        # Wraps core.JobManager
│       ├── task_types.go        # Frontend-compatible types
│       ├── copy.go              # Copy service (uses jobmanager)
│       ├── prereq.go            # Prerequisites service
│       └── device.go            # Device detection service
│
├── cli/                         # CLI Adapter
│   ├── main.go                  # CLI entry point
│   └── reporter.go              # Console/JSON reporters
│
└── pkg/
    ├── engine/                  # Copy engine (shared by all)
    │   ├── copy.go
    │   └── walker.go
    └── state/                   # State management
        └── state.go
```

---

## Key Principles

### 1. Core Has No Dependencies on Adapters

```go
// ❌ BAD: Core importing adapter code
package core
import "github.com/wailsapp/wails/v2/pkg/runtime"

// ✅ GOOD: Core defines interface, adapters implement it
package core
type JobEventEmitter interface {
    EmitJobUpdate(event JobUpdateEvent)
}
```

### 2. Adapters Are Thin Translation Layers

Adapters should:
- Parse input (CLI flags, HTTP requests, UI events)
- Call core methods
- Format output (console, JSON, events)

Adapters should NOT:
- Contain business logic
- Duplicate functionality between adapters
- Make decisions about job lifecycle

### 3. Same Job, Different Views

A single job running in core can be observed via:
- Wails UI (real-time React updates)
- HTTP API (SSE stream or polling)
- CLI (if integrated with core)

All see the same `JobSnapshot` data.

### 4. Testing Without UI

```go
// internal/core/job_test.go

func TestJobManager_StartJob(t *testing.T) {
    // Create mock emitter (no Wails, no HTTP)
    mockEmitter := &MockEmitter{}
    jm := NewJobManager(mockEmitter)
    
    // Test core logic directly
    jobID, ctx, err := jm.StartJob(context.Background(), "test", "msg", nil)
    assert.NoError(t, err)
    assert.NotEmpty(t, jobID)
    
    // Verify emitter was called
    assert.Len(t, mockEmitter.events, 1)
}
```

---

## Adding a New Adapter

To add a new adapter (e.g., gRPC, WebSocket):

1. **Create adapter package:** `internal/adapters/grpc/`

2. **Implement JobEventEmitter:**
   ```go
   type GRPCServer struct {
       clients []GRPCClient
   }
   
   func (s *GRPCServer) EmitJobUpdate(event core.JobUpdateEvent) {
       for _, client := range s.clients {
           client.Send(event)
       }
   }
   ```

3. **Register with core:**
   ```go
   coreJobManager.AddEmitter(grpcServer)
   ```

4. **Expose core methods:**
   ```go
   func (s *GRPCServer) StartBackup(req *pb.StartRequest) (*pb.StartResponse, error) {
       jobID, _, err := s.core.StartJob(...)
       return &pb.StartResponse{JobId: jobID}, err
   }
   ```

---

## See Also

- [`frontend/tasks-model.md`](frontend/tasks-model.md) - Architectural contract
- [`UI_ARCHITECTURE.md`](UI_ARCHITECTURE.md) - UI stack and event patterns
- [`ARCHITECTURE.md`](ARCHITECTURE.md) - CLI and engine internals

