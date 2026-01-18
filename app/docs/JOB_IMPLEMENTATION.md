# Job Implementation Plan

This document outlines the implementation of job management, process execution, and UI integration for Start Backup, Verify, Cleanup, and Prerequisites.

## Implementation Status

- ✅ Config persistence service (`config.go`)
- 🔄 Enhanced JobManager with process groups (in progress)
- ⏳ Process execution service with streaming
- ⏳ UI event wiring
- ⏳ Frontend job status display

## Architecture

### Backend Services

1. **ConfigService** - Manages `~/.gussync/config.json`
   - DestinationPath, SourcePath, LastLogPath, LogDir
   - Load/Save methods

2. **JobManager** (enhanced)
   - Track process groups for SIGINT cancellation
   - Store process PIDs
   - CancelJob sends SIGINT to process group

3. **JobExecutor Service** (new)
   - Wraps exec.CommandContext
   - Handles process groups (Setpgid=true on Unix)
   - Streams stdout/stderr to events
   - Parses progress from output
   - Writes to log file

4. **CopyService, VerifyService, CleanupService** (enhanced)
   - Use JobExecutor to run actual processes
   - Emit events: job:status, job:progress, job:log, job:error
   - Integrate with existing CLI logic where possible

### Frontend

1. **Store enhancements**
   - Job status (idle/running/done/error/canceled)
   - Progress (0-100)
   - Current log lines

2. **Home page updates**
   - Destination selector
   - Job status banner
   - Progress bar
   - Cancel button (when running)

3. **Event subscriptions**
   - job:status, job:progress, job:log, job:error
   - Update UI reactively

4. **Prerequisites page**
   - Rerun button wired to PrereqService.RefreshNow()

## Key Methods

### Backend (Wails bindings)

- `ChooseDestination()` -> string (uses runtime.DialogOpenDirectory)
- `GetConfig()` -> Config
- `StartBackup(source, dest, mode)` -> error
- `VerifyBackup(source, dest)` -> error
- `CleanupSource(source, dest)` -> error
- `CancelJob()` -> error
- `RerunPrerequisites()` -> PrereqReport
- `OpenLogFile()` -> error (uses runtime.BrowserOpenURL)

### Events

- `job:status` {id, state, message}
- `job:progress` {percent, phase, detail}
- `job:log` {id, line}
- `job:error` {id, message}
- `prereq:status` {ok, message} (emitted by RefreshNow)

## Process Cancellation

On Unix:
```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,
}
// On cancel: kill(-pgid, syscall.SIGINT)
```

On Windows:
```go
// Use cmd.Process.Kill() or signal handling
```

## Progress Tracking

1. Try parsing rsync `--info=progress2` output
2. Fallback: estimate from bytes transferred / total size
3. Emit job:progress events periodically

## Log Files

- Location: `~/.gussync/logs/gussync-YYYYMMDD-HHMMSS.log`
- Format: Timestamp | Level | Message
- Rotate: Keep last N logs (configurable)

