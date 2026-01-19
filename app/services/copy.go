package services

import (
	"GusSync/pkg/engine"
	"GusSync/pkg/state"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsReporter reports progress to the Wails frontend
type WailsReporter struct {
	ctx        context.Context
	jobID      string
	jobManager *JobManager
}

func (r *WailsReporter) ReportProgress(update engine.ProgressUpdate) {
	// 1. Legacy stats for backward compatibility
	stats := map[string]interface{}{
		"id":               r.jobID,
		"totalFiles":       float64(update.TotalFiles),
		"filesCompleted":   float64(update.Completed),
		"filesSkipped":     float64(update.Skipped),
		"filesFailed":      float64(update.Failed),
		"timeoutSkips":     float64(update.TimeoutSkips),
		"consecutiveSkips": float64(update.ConsecutiveSkips),
		"speed":            update.Rate / (1024 * 1024),
		"speedUnit":        "MB/s",
		"deltaMB":          update.DeltaMB,
		"progressFiles":    0.0,
	}

	if update.TotalFiles > 0 {
		stats["progressFiles"] = (float64(update.Completed) / float64(update.TotalFiles)) * 100.0
	}

	runtime.EventsEmit(r.ctx, "job:progress", stats)

	// 2. New unified TaskUpdateEvent
	if r.jobManager != nil {
		progress := TaskProgress{
			Phase:   "copying",
			Current: int64(update.Completed),
			Total:   int64(update.TotalFiles),
			Percent: stats["progressFiles"].(float64),
			Rate:    update.Rate / (1024 * 1024), // MB/s
		}
		
		message := fmt.Sprintf("Copied %d/%d files", update.Completed, update.TotalFiles)
		if update.ScanComplete {
			progress.Phase = "finishing"
		}
		
		r.jobManager.updateTaskProgress(r.jobID, progress, message, update.WorkerStatuses)
	}

	// 3. Report worker statuses (legacy)
	for id, status := range update.WorkerStatuses {
		workerData := map[string]interface{}{
			"id":       r.jobID,
			"workerID": id,
			"status":   "active",
			"message":  status,
		}
		runtime.EventsEmit(r.ctx, "job:worker", workerData)
	}
}

func (r *WailsReporter) ReportError(err error) {
	if r.jobManager != nil {
		r.jobManager.failTask(r.jobID, err, "")
	}

	// Legacy error event
	runtime.EventsEmit(r.ctx, "job:error", map[string]interface{}{
		"id":      r.jobID,
		"message": err.Error(),
	})
}

func (r *WailsReporter) ReportLog(level, message string) {
	// New unified event via JobManager (if we want to append logLine)
	if r.jobManager != nil {
		// We could implement a specific EmitTaskLog method if we want to send LogLine
		r.muLogLineEmit(message)
	}

	// Legacy LogLine event
	runtime.EventsEmit(r.ctx, "LogLine", map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     level,
		"message":   message,
	})
}

func (r *WailsReporter) muLogLineEmit(logLine string) {
	r.jobManager.mu.Lock()
	snapshot, exists := r.jobManager.tasks[r.jobID]
	r.jobManager.mu.Unlock()

	if !exists {
		return
	}

	event := TaskUpdateEvent{
		TaskID:   snapshot.TaskID,
		Type:     snapshot.Type,
		State:    snapshot.State,
		Progress: snapshot.Progress,
		Message:  snapshot.Message,
		LogLine:  logLine,
		Artifact: snapshot.Artifact,
	}

	if r.ctx != nil {
		runtime.EventsEmit(r.ctx, "task:update", event)
	}
}

// CopyService handles copy operations using the core engine
type CopyService struct {
	ctx           context.Context
	logger        *log.Logger
	jobManager    *JobManager
	deviceService *DeviceService
	config        *ConfigService
}

// NewCopyService creates a new CopyService
func NewCopyService(ctx context.Context, logger *log.Logger, jobManager *JobManager, deviceService *DeviceService) *CopyService {
	return &CopyService{
		ctx:           ctx,
		logger:        logger,
		jobManager:    jobManager,
		deviceService: deviceService,
	}
}

// SetContext sets the Wails runtime context for the service
func (s *CopyService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// SetConfig sets the config service
func (s *CopyService) SetConfig(config *ConfigService) {
	s.config = config
}

// ChooseDestination opens a directory selection dialog
func (s *CopyService) ChooseDestination() (string, error) {
	path, err := runtime.OpenDirectoryDialog(s.ctx, runtime.OpenDialogOptions{
		Title: "Choose Backup Destination",
	})
	if err != nil {
		return "", err
	}
	
	if s.config != nil && path != "" {
		_ = s.config.SetDestinationPath(path)
	}
	
	return path, nil
}

// StartBackup starts a backup operation
func (s *CopyService) StartBackup(sourcePath, destPath, mode string) (string, error) {
	s.logger.Printf("[CopyService] StartBackup CALLED: sourcePath=%s destPath=%s mode=%s", sourcePath, destPath, mode)
	
	// Auto-detect source if empty
	if sourcePath == "" {
		s.logger.Printf("[CopyService] No sourcePath provided, auto-detecting...")
		devices, _ := s.deviceService.GetDeviceStatus()
		s.logger.Printf("[CopyService] Found %d devices", len(devices))
		if len(devices) > 0 {
			sourcePath = devices[0].Path
			s.logger.Printf("[CopyService] Using device path: %s", sourcePath)
		}
	}

	if sourcePath == "" {
		sourcePath = "/sdcard" // Default
		s.logger.Printf("[CopyService] No device found, using default: %s", sourcePath)
	}

	// Get destination from config if empty
	if destPath == "" && s.config != nil {
		destPath = s.config.GetConfig().DestinationPath
		s.logger.Printf("[CopyService] Got destPath from config: %s", destPath)
	}

	if destPath == "" {
		s.logger.Printf("[CopyService] ERROR: No destination selected")
		return "", fmt.Errorf("destination not selected")
	}
	
	s.logger.Printf("[CopyService] About to call jobManager.startTask...")

	params := map[string]string{
		"sourcePath": sourcePath,
		"destPath":   destPath,
		"mode":       mode,
	}

	jobID, jobCtx, err := s.jobManager.startTask("copy.sync", "Initializing backup...", params)
	if err != nil {
		return "", err
	}

	// Update destination with mode
	fullDestPath := filepath.Join(destPath, mode)
	_ = os.MkdirAll(fullDestPath, 0755)

	// Run engine in goroutine
	go func() {
		reporter := &WailsReporter{ctx: s.ctx, jobID: jobID, jobManager: s.jobManager}
		reporter.ReportLog("info", fmt.Sprintf("Starting backup from %s to %s...", sourcePath, fullDestPath))

		// Initialize state inside goroutine to prevent blocking the UI
		// Large state files can take seconds to load
		stateFile := filepath.Join(fullDestPath, "gus_state.md")
		reporter.ReportLog("info", "Loading state file...")
		stateManager, err := state.NewStateManager(stateFile)
		if err != nil {
			reporter.ReportError(fmt.Errorf("CRITICAL: failed to initialize state: %w", err))
			s.jobManager.completeJob(false)
			return
		}
		defer stateManager.Close()
		defer s.jobManager.completeJob(true)

		cfg := engine.EngineConfig{
			SourcePath: sourcePath,
			DestRoot:   fullDestPath,
			Mode:       mode,
			NumWorkers: 2, // Default
			Reporter:   reporter,
		}

		e := engine.NewEngine(cfg, stateManager)
		
		runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
			"id":         jobID,
			"state":      "running",
			"message":    "Backup in progress...",
			"sourcePath": sourcePath,
			"destPath":   fullDestPath,
		})

		if err := e.Run(jobCtx); err != nil {
			reporter.ReportError(err)
		}

		s.jobManager.completeTask(jobID, "Backup completed successfully")
	}()

	return jobID, nil
}

func (s *CopyService) CancelCopy() error {
	return s.jobManager.CancelJob()
}
