# GusSync Wails v2 Architecture

## Directory Structure

```
GusSync/
├── app/                          # Wails app package
│   ├── app.go                    # Wails app entry point
│   ├── services/                 # Go services (Wails bindings)
│   │   ├── prereq.go            # PrereqService - Prerequisite checks
│   │   ├── device.go            # DeviceService - Device discovery
│   │   ├── copy.go              # CopyService - Copy operations
│   │   ├── verify.go            # VerifyService - Verification
│   │   ├── log.go               # LogService - Log aggregation
│   │   └── jobmanager.go        # JobManager - Single job execution
│   └── core/                     # Core CLI logic (reference existing code)
│       └── (references parent dir files)
├── frontend/                     # React frontend
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Home.tsx         # Dashboard
│   │   │   ├── Prerequisites.tsx # Prerequisites page
│   │   │   ├── CopyWizard.tsx   # Copy wizard
│   │   │   ├── VerifyWizard.tsx # Verify wizard
│   │   │   └── Logs.tsx         # Logs view
│   │   ├── components/
│   │   │   ├── Layout.tsx       # App layout with navigation
│   │   │   ├── PrereqCard.tsx   # Prerequisite status card
│   │   │   └── ProgressBar.tsx  # Progress indicators
│   │   ├── store/
│   │   │   └── AppState.ts      # Global state management
│   │   └── main.tsx             # React entry point
│   ├── package.json
│   └── ...
├── main.go                       # Existing CLI entry (unchanged)
├── adb_adapter.go               # Existing ADB adapter
├── fs_adapter.go                # Existing FS adapter
├── copy.go                      # Existing copy logic
├── verify.go                    # Existing verify logic
├── state.go                     # Existing state management
└── ...
```

## Service Interfaces

### PrereqService

```go
type PrereqService struct {
    ctx context.Context
}

func NewPrereqService(ctx context.Context) *PrereqService

// GetPrereqReport returns the current prerequisite status report
func (s *PrereqService) GetPrereqReport() PrereqReport

// PrereqCheck represents a single prerequisite check
type PrereqCheck struct {
    ID             string   `json:"id"`
    Name           string   `json:"name"`
    Status         string   `json:"status"` // "ok", "warn", "fail"
    Details        string   `json:"details"`
    RemediationSteps []string `json:"remediationSteps"`
    Links          []string `json:"links,omitempty"`
}

// PrereqReport contains all prerequisite checks
type PrereqReport struct {
    OverallStatus  string       `json:"overallStatus"` // "ok", "warn", "fail"
    OS             string       `json:"os"`            // "linux", "windows", "darwin"
    Checks         []PrereqCheck `json:"checks"`
}
```

### DeviceService

```go
type DeviceService struct {
    ctx context.Context
}

func NewDeviceService(ctx context.Context) *DeviceService

// ScanDevices discovers available devices (MTP/ADB)
func (s *DeviceService) ScanDevices() []Device

type Device struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Type     string `json:"type"` // "mtp", "adb", "gphoto2"
    Path     string `json:"path"`
    Connected bool  `json:"connected"`
}
```

### CopyService

```go
type CopyService struct {
    ctx context.Context
    jobManager *JobManager
}

func NewCopyService(ctx context.Context, jobManager *JobManager) *CopyService

// StartCopy starts a copy operation (non-blocking)
func (s *CopyService) StartCopy(config CopyConfig) error

// CancelCopy cancels the current copy operation
func (s *CopyService) CancelCopy() error

type CopyConfig struct {
    SourcePath      string   `json:"sourcePath"`
    DestPath        string   `json:"destPath"`
    Mode            string   `json:"mode"` // "mount" or "adb"
    Workers         int      `json:"workers"`
    IncludePatterns []string `json:"includePatterns,omitempty"`
    ExcludePatterns []string `json:"excludePatterns,omitempty"`
}
```

### VerifyService

```go
type VerifyService struct {
    ctx context.Context
    jobManager *JobManager
}

func NewVerifyService(ctx context.Context, jobManager *JobManager) *VerifyService

// StartVerify starts a verification operation (non-blocking)
func (s *VerifyService) StartVerify(config VerifyConfig) error

// CancelVerify cancels the current verification operation
func (s *VerifyService) CancelVerify() error

type VerifyConfig struct {
    SourcePath string `json:"sourcePath"`
    DestPath   string `json:"destPath"`
    Level      string `json:"level"` // "quick" or "full"
}
```

### LogService

```go
type LogService struct {
    ctx context.Context
}

func NewLogService(ctx context.Context) *LogService

// GetRecentLogs returns recent log entries
func (s *LogService) GetRecentLogs(limit int) []LogEntry

// ExportDiagnostics creates a diagnostics bundle
func (s *LogService) ExportDiagnostics(outputPath string) error

type LogEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Level     string    `json:"level"` // "info", "warn", "error"
    Message   string    `json:"message"`
}
```

### JobManager

```go
type JobManager struct {
    mu           sync.Mutex
    currentJob   Job
    ctx          context.Context
    cancel       context.CancelFunc
}

type Job interface {
    Cancel()
    Status() JobStatus
}

type JobStatus struct {
    Type      string    `json:"type"` // "copy" or "verify"
    Running   bool      `json:"running"`
    Progress  float64   `json:"progress"`
    Message   string    `json:"message"`
}
```

## Event Payload Shapes

### CopyProgress Event

```json
{
  "type": "CopyProgress",
  "filesTotal": 1000,
  "filesCompleted": 150,
  "filesSkipped": 5,
  "filesFailed": 2,
  "bytesTotal": 10737418240,
  "bytesCopied": 1610612736,
  "speedMBps": 2.34,
  "timeRemaining": "15m 30s",
  "currentFile": "/path/to/file.jpg",
  "workers": [
    {
      "id": 1,
      "status": "Copying file.jpg (2.5 MB)"
    }
  ]
}
```

### VerifyProgress Event

```json
{
  "type": "VerifyProgress",
  "filesTotal": 1000,
  "filesVerified": 750,
  "filesMissing": 5,
  "mismatches": 2,
  "progress": 75.0
}
```

### LogLine Event

```json
{
  "type": "LogLine",
  "timestamp": "2024-01-15T10:30:45Z",
  "level": "info",
  "message": "Starting backup operation..."
}
```

### PrereqReport Event

```json
{
  "type": "PrereqReport",
  "report": {
    "overallStatus": "fail",
    "os": "linux",
    "checks": [...]
  }
}
```


