# GusSync Architecture Documentation

This document describes the internal logic and architecture of GusSync, a robust CLI tool for backing up files from MTP/FUSE filesystems.

## Table of Contents

1. [Overview](#overview)
2. [Core Components](#core-components)
3. [Data Flow](#data-flow)
4. [State Management](#state-management)
5. [Worker Pool System](#worker-pool-system)
6. [Directory Traversal](#directory-traversal)
7. [File Copying Logic](#file-copying-logic)
8. [Progress Tracking](#progress-tracking)
9. [Error Handling & Retry Logic](#error-handling--retry-logic)
10. [Verification Pass](#verification-pass)
11. [Shutdown & Cleanup](#shutdown--cleanup)

---

## Overview

GusSync is designed to solve the problem of backing up large phone filesystems mounted via MTP/FUSE, where standard tools fail because they enumerate the entire tree before copying. GusSync uses a streaming architecture that discovers and copies files concurrently.

### Key Design Principles

- **Streaming**: Files are discovered and copied simultaneously (not sequential enumeration)
- **Lightweight**: Minimal memory footprint, no full tree loading
- **Resumable**: State tracked in human-readable Markdown file
- **Robust**: Handles MTP latency, stalls, and connection issues
- **Thread-safe**: Uses `sync.Mutex` for concurrent state access

---

## Core Components

### 1. **main.go** - Orchestrator
- Entry point and CLI argument parsing
- Worker pool coordination
- Statistics collection and display
- Shutdown signal handling
- Verification pass execution

### 2. **state.go** - State Manager
- Thread-safe Markdown state file management
- Tracks completed files with SHA256 hashes
- Tracks file failures (up to 10 attempts)
- In-memory maps for fast lookups
- Buffered writing for performance

### 3. **walker.go** - Directory Crawler
- Recursive directory traversal with timeouts
- Priority-based path processing (DCIM, Camera, etc. first)
- Concurrent subdirectory processing
- MTP latency handling (60s directory read timeout)
- Streams file paths immediately via channels

### 4. **copy.go** - File Copy Engine
- Robust copying with stall detection (30s timeout)
- Real-time progress reporting
- SHA256 hash verification (source vs destination)
- Directory structure preservation
- Timeout-based retry on stalls

---

## Data Flow

```
┌─────────────┐
│   main()    │  Entry point
└──────┬──────┘
       │
       ├─> Initialize StateManager
       │   └─> Load gus_state.md into memory maps
       │
       ├─> Create Channels
       │   ├─> jobChan (buffered, 1000)
       │   ├─> errorChan (buffered, 100)
       │   └─> statsChan (buffered, 100)
       │
       ├─> Start Workers (numWorkers = min(CPU cores, 4))
       │   └─> Workers wait on jobChan
       │
       ├─> Start Walker (goroutine)
       │   └─> Discovers files, sends to jobChan
       │
       ├─> Workers Process Jobs
       │   ├─> Check state (skip if done)
       │   ├─> Copy file with progress tracking
       │   ├─> Hash verification
       │   └─> Update state if successful
       │
       ├─> Stats Display (every 2s)
       │   └─> Shows worker activity, progress, speed
       │
       ├─> After Completion
       │   └─> Verification pass (hash comparison)
       │
       └─> Shutdown Handling
           └─> Graceful (10s timeout) → Force exit
```

---

## State Management

### State File Format (`gus_state.md`)

**Completed Files:**
```markdown
- [x] /path/to/file.jpg | Hash: abc123def456...
```

**Failed Files:**
```markdown
- [ ] /path/to/file.mp4 | Failures: 3
```

### StateManager Logic

1. **Initialization** (`NewStateManager`)
   - Opens `gus_state.md` for reading
   - Parses completed files → `stateMap[path] = hash`
   - Parses failed files → `failureMap[path] = count`
   - Opens file for appending (buffered writer)

2. **Checking Status** (`IsDone`, `ShouldRetry`)
   - `IsDone(path)`: Returns true if file exists in `stateMap`
   - `ShouldRetry(path)`: Returns true if `failureMap[path] < 10`

3. **Recording Success** (`MarkDone`)
   - Updates `stateMap[path] = hash`
   - Appends to state file: `- [x] path | Hash: hash`
   - Thread-safe (uses Mutex)

4. **Recording Failure** (`RecordFailure`)
   - Only increments if `hasSuccess == true` (prevents counting failures during initial connection issues)
   - Max 1 failure per run per file
   - Increments `failureMap[path]++`
   - Appends to state file: `- [ ] path | Failures: N`
   - After 10 failures, file is skipped permanently

5. **Flushing**
   - Buffered writer flushed periodically (worker 0 does this)
   - Flushed on shutdown/close

---

## Worker Pool System

### Worker Initialization

- Number of workers: `min(runtime.NumCPU(), MTPMaxWorkers)` where `MTPMaxWorkers = 4`
- Workers started **before** walker (so they're ready to consume jobs)
- Each worker runs in its own goroutine

### Worker Lifecycle

```
Worker Loop:
  1. Read from jobChan (blocks until job available)
  2. Check if file is done (IsDone) → skip if yes
  3. Check if should retry (ShouldRetry) → skip if failed 10x
  4. Get file info (size, name)
  5. Update worker status: "Copying: filename (size)"
  6. Create progress channel
  7. Call RobustCopy with progress channel
  8. Monitor progress updates (every 2s)
  9. Update status: "Copying: filename (copied/total %)"
  10. On completion:
      - Success: MarkDone, update stats
      - Failure: RecordFailure, update stats
  11. Clear worker status
  12. Loop back to step 1
```

### Worker Status Tracking

Each worker maintains a status string in a shared map:
- `"Copying: filename (copied/total %)"` - Active copy
- `"Starting: filename (size)"` - Just starting
- `"Failed: filename"` - Copy failed
- `"idle"` - Not processing anything

Status is displayed every 2 seconds in the stats output.

---

## Directory Traversal

### Walker Algorithm

1. **Root Level Priority Sorting**
   - When at root directory, sorts entries by priority
   - Priority paths (DCIM, Camera, Pictures) processed first
   - Other directories processed after

2. **Concurrent Directory Processing**
   - Each subdirectory processed in separate goroutine
   - Uses `sync.WaitGroup` to wait for all to complete
   - Files streamed immediately as discovered

3. **MTP Latency Handling**
   - Directory reads wrapped with 60s timeout
   - If directory read times out, error reported but continues
   - Timeout prevents hanging on slow MTP directories

4. **Context Cancellation**
   - Walker respects context cancellation (for shutdown)
   - On cancel, stops discovering new files
   - Existing discovery continues

### Priority Path Logic

Priority paths (in order):
1. DCIM (Camera photos/videos)
2. Camera
3. Pictures
4. Documents
5. Download
6. Movies, Music, Videos
7. Screenshots, ScreenRecordings
8. WhatsApp/Media
9. Android/media, Android/data

Non-priority paths get score 100, processed after all priority paths.

---

## File Copying Logic

### RobustCopy Flow

```
RobustCopy(sourcePath, sourceRoot, destRoot, progressChan):
  1. Calculate relative path: filepath.Rel(sourceRoot, sourcePath)
  2. Build destination: destRoot + relativePath
  3. Create destination directory (mkdir -p)
  4. Open source file
  5. Create destination file (truncates if exists)
  6. Copy with timeout (copyWithTimeout)
  7. Sync destination file (fsync)
  8. Hash source file (SHA256)
  9. Hash destination file (SHA256)
  10. Verify hashes match
  11. Return result
```

### copyWithTimeout Logic

```
copyWithTimeout(src, dst, timeout, progressChan):
  1. Create progressTracker with progressChan
  2. Start progress monitor goroutine:
     - Every 1s: Check for stall (no progress > timeout)
     - Every 2s: Send progress update via progressChan
  3. Wrap source reader with progressReader
  4. io.Copy from progressReader to dst
  5. On each Read():
     - Update progressTracker.bytes
     - Update progressTracker.lastTime
  6. If stall detected (time.Since(lastTime) > timeout):
     - Cancel context
     - Return error: "copy stalled: no progress for 30s"
  7. Return bytes copied or error
```

### Stall Detection

- **Timeout**: 30 seconds of no progress
- **Progress Check**: Every 1 second
- **Detection**: `time.Since(progressTracker.lastTime) > 30s`
- **Action**: Copy fails, error logged, file marked as failed

### Hash Verification

- **Algorithm**: SHA256
- **Comparison**: Source hash vs destination hash
- **On Mismatch**: Error returned, file not marked as done
- **Timing**: Hashes calculated after copy completion

---

## Progress Tracking

### Real-time Progress Updates

1. **Progress Channel**
   - Created per file copy operation
   - Receives bytes copied every 2 seconds
   - Worker monitors this channel

2. **Progress Display**
   - Worker status shows: `"Copying: filename (X MB/Y MB Z%)"`
   - Updated every 2 seconds during copy
   - Shows current bytes / total bytes / percentage

3. **Stall Detection**
   - If no progress update for 30 seconds → copy fails
   - Allows worker to move on to next file

### Statistics Tracking

Global stats tracked:
- `totalFiles`: Total files processed
- `completed`: Successfully copied
- `failed`: Failed copies
- `skipped`: Skipped (already done or failed 10x)
- `totalBytes`: Total bytes copied
- `lastTotalBytes`: Bytes at last stats check
- `lastStatsTime`: Time of last stats check

Stats displayed every 2 seconds:
```
[150 files] Completed: 145 | Skipped: 3 | Failed: 2 | Speed: 12.34 MB/s | Delta: 25.67 MB
  W0: Copying: IMG_1234.jpg (1.2 MB/5.0 MB 24.0%)
  W1: idle
```

---

## Error Handling & Retry Logic

### Error Logging

- All errors written to `gus_errors.log` in destination directory
- Format: `[timestamp] Error: message`
- File synced immediately (for `tail -f`)
- Includes timestamps for debugging

### Failure Tracking Strategy

1. **Initial State**: `hasSuccess = false`
   - If all files failing (connection issue), failures not counted
   - Prevents marking files as failed during initial connection problems

2. **After First Success**: `hasSuccess = true`
   - Failures now counted and recorded
   - One failure per run per file max
   - Failure count incremented in state file

3. **Retry Limits**
   - Files retried up to 10 times across different runs
   - After 10 failures: `ShouldRetry()` returns false
   - File permanently skipped

4. **Stall Handling**
   - Copy timeout (30s) treated as failure
   - Recorded in state if `hasSuccess == true`
   - Worker immediately moves to next file

### Error Types Handled

- **Directory read timeout**: 60s timeout for MTP directory reads
- **Copy stall**: 30s timeout for file copy progress
- **Hash mismatch**: Source and destination hashes don't match
- **File I/O errors**: Read/write errors, missing files
- **State file errors**: Errors writing to state file

---

## Verification Pass

### Post-Backup Verification

After all files are copied, a verification pass runs:

1. **Get All Completed Files**: From state manager's `stateMap`

2. **Verify Each File**:
   - Check source file exists
   - Check destination file exists
   - Calculate source hash
   - Calculate destination hash
   - Compare hashes

3. **On Hash Mismatch**:
   - Attempt re-copy (overwrites destination)
   - Re-verify hashes
   - If still mismatch: count as verification failure

4. **Results**:
   - `verified`: Files with matching hashes
   - `missingSource`: Source files no longer exist
   - `missingDest`: Destination files missing
   - `mismatches`: Files with hash mismatches

5. **Exit Code**:
   - Exit 0 if all verified
   - Exit 1 if any mismatches or missing files

---

## Shutdown & Cleanup

### Shutdown Signals

- Handles `SIGINT` (Ctrl+C) and `SIGTERM`
- Uses `signal.Notify` to capture signals

### Shutdown Flow

```
On SIGINT/SIGTERM:
  1. Cancel context (stops walker)
  2. Close jobChan (stops workers from accepting new jobs)
  3. Wait up to 10 seconds for workers to finish current operations
  4. If timeout:
     - Flush state file
     - Force exit (os.Exit(130))
  5. Otherwise:
     - Wait for error handler
     - Flush state file
     - Normal exit
```

### Channel Closing Safety

- Uses `sync.Once` to ensure `jobChan` closed exactly once
- Walker's defer calls safe close function
- Main's shutdown also calls safe close function
- Prevents "close of closed channel" panics

### Worker Cancellation

- Workers check `ctx.Done()` in select statement
- When context cancelled, workers exit immediately
- Current file copy may complete or fail
- Workers exit gracefully on channel close

---

## Key Algorithms

### Priority Path Sorting

```go
getPathPriority(relPath, rootPath):
  rel = filepath.Rel(rootPath, relPath)
  firstDir = first component of rel
  for i, priorityPath in PriorityPaths:
    if firstDir == priorityPath or rel starts with priorityPath:
      return i  // Lower = higher priority
  return 100  // Default (low priority)
```

### Stall Detection

```go
Progress Monitor Loop:
  every 1 second:
    if time.Since(progressTracker.lastTime) > 30s:
      cancel context
      return "copy stalled"

  every 2 seconds:
    send progressTracker.bytes via progressChan
```

### Failure Counting Logic

```go
On File Failure:
  if hasSuccess == false:
    return  // Don't count failures yet (likely connection issue)
  
  if failedThisRun[path] == false:
    failureMap[path]++
    append to state file
    failedThisRun[path] = true
```

---

## Constants & Configuration

- `StallTimeout`: 30 seconds (file copy stall detection)
- `DirReadTimeout`: 60 seconds (MTP directory read timeout)
- `ProgressUpdateInterval`: 2 seconds (progress reporting frequency)
- `MTPMaxWorkers`: 4 (maximum concurrent workers for MTP safety)
- `jobBufferSize`: 1000 (buffered channel size)
- `stateFileName`: "gus_state.md" (state file name)

---

## Thread Safety

All shared state protected with `sync.Mutex`:

1. **StateManager**: Mutex protects `stateMap`, `failureMap`, file operations
2. **Statistics**: Mutex protects stats struct fields
3. **Worker Status**: Mutex protects status map
4. **Progress Tracker**: Mutex protects progress bytes and lastTime

---

## File Structure

```
GusSync/
├── main.go      # Entry point, orchestration, worker pool
├── state.go     # State management, Markdown parsing/writing
├── walker.go    # Directory traversal, priority sorting
├── copy.go      # File copying, stall detection, hashing
├── go.mod       # Go module definition
└── test_mtp.sh  # Test script for MTP devices
```

---

## Summary

GusSync implements a robust, streaming backup system designed for MTP/FUSE filesystems:

- **Streams** files as discovered (no full tree enumeration)
- **Tracks** state in human-readable Markdown
- **Handles** stalls, timeouts, and connection issues gracefully
- **Prioritizes** important directories (photos, documents) first
- **Verifies** all copies with hash comparison
- **Resumes** automatically on subsequent runs
- **Limits** retries intelligently (max 10 attempts, only after success)

The architecture is designed to be lightweight, fast, and resilient to the challenges of backing up large MTP-mounted filesystems.


