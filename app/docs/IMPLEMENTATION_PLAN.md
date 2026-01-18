# GusSync Wails v2 Implementation Plan

## Phase 1: Prerequisites System (✅ In Progress)

### Completed:
- ✅ `PrereqService` implementation with checks for:
  - ADB binary detection
  - MTP/GVFS tools (Linux)
  - Device connection (ADB/MTP)
  - Destination write access
  - Disk space
  - WebView2 (Windows)
  - File system support
- ✅ Architecture documentation

### Next Steps:
1. Create Wails app scaffold (`app/app.go`)
2. Create basic JobManager (`app/services/jobmanager.go`)
3. Initialize Wails with PrereqService binding
4. Create React frontend structure
5. Build Prerequisites UI page

## Phase 2: Device Discovery and Copy Services

### Tasks:
1. Implement `DeviceService` for device scanning
2. Implement `CopyService` wrapping existing copy logic
3. Integrate with existing `main.go` copy flow
4. Add progress event emission

## Phase 3: Verify Service

### Tasks:
1. Implement `VerifyService` wrapping existing verify logic
2. Integrate with existing `verify.go` flow
3. Add progress event emission

## Phase 4: UI Pages

### Tasks:
1. Home Dashboard (device status, quick actions)
2. Copy Wizard (multi-step form)
3. Verify Wizard (config and run)
4. Logs View (live log stream)
5. Layout and Navigation

## Phase 5: Integration and Testing

### Tasks:
1. Test prerequisite checks on all platforms
2. Test copy/verify flow end-to-end
3. Test cancellation and error handling
4. UI polish and responsiveness

## Notes

- The existing CLI code (`main.go`, `copy.go`, `verify.go`, etc.) should remain mostly unchanged
- Services will wrap/call existing functions, not rewrite them
- Use Wails Events for progress updates instead of direct stdout/stderr
- JobManager ensures only one job runs at a time (copy OR verify)


