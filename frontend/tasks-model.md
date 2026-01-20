# Architecture Contract: Core + Adapters, Job-Based State, and UI Reliability (2026)

Purpose  
This document defines the architectural rules, frameworks, and design patterns used in this project.

The goal is to:
- eliminate UI state bugs
- avoid parsing stdout as an API
- share logic across CLI, API, and desktop UI
- make refactoring and AI-assisted development predictable
- prevent missed events, stuck buttons, and desynchronized progress

This document is authoritative. New features must conform to it.

---

## High-Level Philosophy

1) **Core logic is written once**
2) **Every interface (CLI, API, UI) is an adapter**
3) **State lives in the backend, not the UI**
4) **Snapshots are truth, events are hints**
5) **Text output is presentation, not a contract**

---

## Framework and Tooling Overview

### Wails
Role: Desktop container

Wails provides:
- Go backend
- embedded web UI
- direct Go ↔ frontend bindings
- event emission

Wails is not a UI framework. It hosts one.

---

### Svelte
Role: UI rendering layer

Chosen because:
- significantly less code than React
- simpler reactivity model
- fewer accidental state bugs
- easier to keep “dumb renderer” discipline

The frontend must not own business state.

---

### Tailwind CSS
Role: Styling only

Tailwind is used to:
- avoid custom CSS sprawl
- keep UI styling declarative
- remain framework-agnostic

Tailwind has zero impact on application state.

---

### Why NOT Next.js, SSR frameworks, etc.
This is a desktop app, not a deployed website.

Wails already:
- hosts assets
- controls lifecycle
- exposes backend methods

Adding Next.js would add complexity without benefit.

---

## Core Architectural Pattern: Core + Adapters

### Core
The Core is a pure Go library.

It contains:
- job execution logic
- prerequisite checks
- progress calculation
- structured state models
- cancellation logic

The Core must not import:
- Cobra
- HTTP frameworks
- Wails
- UI code

The Core must be fully testable without UI.

---

### Adapters
Adapters translate external interfaces into Core calls.

Adapters must be thin.

#### CLI Adapter
- Uses Cobra
- Parses flags and arguments
- Calls Core methods
- Formats structured state as text or JSON

#### API Adapter
- Exposes REST or RPC endpoints
- Returns JSON snapshots
- Optional event streaming (SSE)

#### Wails Adapter
- Binds Core methods to frontend
- Emits structured events
- Handles UI startup handshake

Adapters must not reimplement business logic.

---

## Job Model (Universal Across All Interfaces)

All long-running operations are Jobs.

### Job Lifecycle
Queued → Running → Succeeded  
Queued → Running → Failed  
Queued → Running → Canceled  

No backward transitions.

---

### JobSnapshot (Authoritative State)

The JobSnapshot must contain everything needed to render the UI.

Required fields:
- ID
- Seq (monotonic)
- Status (queued, running, succeeded, failed, canceled)
- Phase (short label)
- ProgressCurrent
- ProgressTotal
- Message
- Error (only when failed)
- StartedAt
- UpdatedAt
- FinishedAt

Optional structured fields:
- CurrentItem (file, operation, sizes, deltas)
- Counters (files processed, bytes processed, errors)
- Artifacts (reports, outputs)
- PrerequisiteResults

Rule:
If the UI needs it, it must exist in the snapshot.

---

## Events vs Snapshots (Critical Rule)

### Snapshots
- Canonical truth
- Pulled on startup
- Emitted on every meaningful change
- Used to derive all UI state

### Events
- Used for responsiveness
- May be missed
- Never trusted alone

Rule:
**If an event is missed, the UI must still recover via snapshot.**

---

## Sequence Numbers (Out-of-Order Protection)

Every JobSnapshot includes:
- Seq (monotonically increasing integer)

UI rule:
- Ignore any snapshot with Seq ≤ lastSeqSeen

This prevents UI regressions from delayed or reordered events.

---

## UI Startup Handshake (Fixes Missed Events)

Required flow:

1) UI loads
2) UI immediately calls `GetAppSnapshot()` or `GetJob(jobID)`
3) UI renders snapshot
4) UI subscribes to events
5) UI ignores out-of-order snapshots using Seq

Prerequisites must never be communicated only via events.

---

## Structured Telemetry (Fixing CLI-First Pain)

### Problem
CLI stdout is unstructured and temporal.
UI cannot safely parse it.

### Solution
Core emits structured data.
Adapters decide how to present it.

---

### Structured Outputs from Core

Core emits:
- JobSnapshot (state)
- JobLogEvent (human-readable logs)
- JobDetailEvent (optional, high-frequency structured data)

No adapter parses text.

---

### CLI Output Modes

The CLI must support:

- Human mode (default)
- Machine mode (`--json`)

Human mode:
- Pretty formatted text
- Progress bars
- Tables

Machine mode:
- JSON snapshots and events
- Stable schema
- No formatting guarantees

Text output is never a stable API.

---

## Progress and Rich UI Updates

Progress bars, tables, and panels are driven by structured fields:

- CurrentItem
- Counters
- ProgressCurrent / ProgressTotal
- Phase

Backend throttling rules:
- Snapshot updates no more than ~5–10 per second
- High-frequency detail events optional
- Logs may be batched

UI renders latest snapshot only.

---

## Cancellation Semantics

Cancel is a request, not an instant result.

Rules:
- CancelJob returns immediately
- Worker checks context
- Job transitions to canceled
- Final snapshot emitted

UI:
- shows “canceling” until terminal snapshot arrives

---

## Prerequisites Handling

Prerequisites must be stateful.

Options:
- Implement as a Job
- Or persist results in snapshot

Rules:
- Never emit prereq results only as events
- Must be visible via snapshot
- UI must render correctly even if prereqs ran before UI loaded

---

## Logging Rules (Go)

Every exported Core and Adapter method:
- Logs inputs using log.Printf
- Logs before business logic
- Returns error

Functions always return error.

---

## Repo Layout (Recommended)

- internal/core
- internal/adapters/cli
- internal/adapters/api
- internal/adapters/wails
- cmd/app-cli
- cmd/app-api
- cmd/app-wails
- docs/

---

## Definition of Done (Mandatory)

A feature is complete only if:
- Job can be started via CLI, API, and UI
- Snapshot fully represents state
- UI derives all state from snapshot
- No UI flags are manually toggled
- Missed events do not break UI
- go test ./... passes

---

## Why This Exists

This contract exists to:
- prevent UI regressions
- enable refactors
- support AI-driven development
- allow new frontends without rewriting logic

If something feels hard to implement under this contract, the design is probably wrong.

---

End of document.
