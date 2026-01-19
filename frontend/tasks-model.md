# GusSync Task Model and UI Contract (Wails + React)

## Goal
Make UI and backend tightly integrated for long running operations (copy, scan, prerequisites), with:
- deterministic progress reporting
- cancellable operations equivalent to Ctrl-C
- consistent error reporting and log linking
- minimal frontend timing bugs

This contract is the single source of truth.

---

## Core Concepts

### Task
A Task represents one long running backend operation.

State machine (only these transitions):
queued -> running -> succeeded | failed | canceled

### Task Types
- prerequisites.run
- scan.source
- copy.sync
- cleanup.delete (optional)

---

## Backend API (Wails-exposed methods)

### StartTask
Start a task and return a taskId immediately.

StartTask(req) -> (taskId, error)

req fields:
- type: string (task type)
- params: object (type-specific)
- meta:
  - createdAt: time (optional, backend can set)

### GetTask
Return current snapshot of task state.

GetTask(taskId) -> (TaskSnapshot, error)

### ListTasks
Return recent tasks (for Errors tab and history).

ListTasks() -> ([]TaskSnapshot, error)

### CancelTask
Cancel a running task. Must be safe to call multiple times.

CancelTask(taskId) -> (error)

---

## Backend Event Stream (Wails Events)

All UI updates for long running work come from ONE event channel.

Event name:
- "task:update"

Payload schema (JSON):
TaskUpdateEvent:
- taskId: string
- type: string
- state: "queued" | "running" | "succeeded" | "failed" | "canceled"
- progress:
  - phase: string            (ex: "enumerating", "copying", "verifying")
  - current: int64           (bytes or items)
  - total: int64             (bytes or items, 0 if unknown)
  - percent: float64         (0-100, computed if total>0)
  - rate: float64            (bytes/sec or items/sec, optional)
- message: string            (short user-facing)
- logLine: string            (optional, append-only UI console)
- error:
  - code: string
  - message: string
  - details: string          (optional)
- artifact:
  - logPath: string          (path to logfile)
  - openLogHint: string      (optional display text)

Rules:
- Backend must emit a TaskUpdateEvent whenever state changes.
- Backend may emit periodic progress updates while running.
- Backend emits final update with terminal state.

---

## Cancellation Semantics

Cancel must behave like Ctrl-C:
- Backend stores a cancel function per taskId.
- Running operations check context cancellation frequently:
  - before starting a heavy step
  - during loops (file enumeration, copy loop)
  - between subprocess reads
- On cancel:
  - stop spawning new work immediately
  - terminate subprocesses
  - close open file handles
  - emit terminal event with state="canceled"

Idempotency:
- CancelTask(taskId) can be called multiple times and returns nil if already canceled/finished.

---

## Logging and Errors

### Log file
Each task writes to a dedicated log file:
logs/<taskId>.log

TaskUpdateEvent should include:
artifact.logPath

### Errors tab behavior
Errors tab is fed from ListTasks() filtered by:
state == "failed"

Each error item shows:
- task type
- time
- error message
- button "Open Log" which calls backend OpenPath(logPath)

---

## UX Rules for React UI

### Single source of truth
React state does NOT infer progress.
React stores TaskSnapshot objects keyed by taskId.
All updates come from "task:update" event payloads.

### Buttons
- Each button starts a task type with params.
- While a task type is running, disable its Start button and show Cancel.
- "Re-run prerequisites" calls StartTask(type="prerequisites.run").

### Progress bar
- If total > 0, show percent and current/total.
- If total == 0, show indeterminate bar with phase text.

### Output console
Append logLine entries from events into a scrollable view for that task.

---

## Destination and Configuration

Destination selection is configuration, not hard-coded.

Backend config:
- destinationRoot: string
- exclusionRules: []string
- mode: "ptp" | "mtp" | "auto"

UI:
- Settings tab
- SaveConfig() and LoadConfig()

StartTask params should include any per-run overrides, but default to config.

---
