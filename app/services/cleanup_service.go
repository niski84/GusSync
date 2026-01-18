package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CleanupService handles cleanup operations (wraps existing cleanup logic)
type CleanupService struct {
	ctx        context.Context
	logger     *log.Logger
	jobManager *JobManager
}

const stateFileName = "gus_state.md"

// NewCleanupService creates a new CleanupService
func NewCleanupService(ctx context.Context, logger *log.Logger, jobManager *JobManager) *CleanupService {
	return &CleanupService{
		ctx:        ctx,
		logger:     logger,
		jobManager: jobManager,
	}
}

// CleanupRequest represents a cleanup operation request
type CleanupRequest struct {
	SourceRoot  string   `json:"sourceRoot"`
	DestRoot    string   `json:"destRoot"`
	StateFiles  []string `json:"stateFiles"` // List of state files to process (e.g., ["mount", "adb"] or just one)
	ProcessBoth bool     `json:"processBoth"` // If true, process both mount and adb state files
}

// StateFileInfo represents information about a detected state file
type StateFileInfo struct {
	Path string `json:"path"` // Full path to state file
	Mode string `json:"mode"` // "mount" or "adb"
}

// DetectStateFiles detects available state files in destRoot
func (s *CleanupService) DetectStateFiles(destRoot string) ([]StateFileInfo, error) {
	s.logger.Printf("[CleanupService] DetectStateFiles: destRoot=%s", destRoot)

	stateFiles := []StateFileInfo{}

	// Check mount mode state file
	mountDestPath := filepath.Join(destRoot, "mount")
	mountStateFile := filepath.Join(mountDestPath, stateFileName)
	if _, err := os.Stat(mountStateFile); err == nil {
		stateFiles = append(stateFiles, StateFileInfo{
			Path: mountStateFile,
			Mode: "mount",
		})
		s.logger.Printf("[CleanupService] DetectStateFiles: Found mount state file: %s", mountStateFile)
	}

	// Check adb mode state file
	adbDestPath := filepath.Join(destRoot, "adb")
	adbStateFile := filepath.Join(adbDestPath, stateFileName)
	if _, err := os.Stat(adbStateFile); err == nil {
		stateFiles = append(stateFiles, StateFileInfo{
			Path: adbStateFile,
			Mode: "adb",
		})
		s.logger.Printf("[CleanupService] DetectStateFiles: Found adb state file: %s", adbStateFile)
	}

	return stateFiles, nil
}

// StartCleanup starts a cleanup operation (non-blocking)
func (s *CleanupService) StartCleanup(req CleanupRequest) error {
	s.logger.Printf("[CleanupService] StartCleanup: sourceRoot=%s destRoot=%s stateFiles=%v processBoth=%v", req.SourceRoot, req.DestRoot, req.StateFiles, req.ProcessBoth)

	// Start job
	jobID, jobCtx, err := s.jobManager.StartJob("cleanup")
	if err != nil {
		return fmt.Errorf("failed to start cleanup job: %w", err)
	}

	// Determine which state files to process
	stateFilesToProcess := req.StateFiles
	if req.ProcessBoth || len(stateFilesToProcess) == 0 {
		// Detect all available state files
		detected, err := s.DetectStateFiles(req.DestRoot)
		if err != nil {
			return fmt.Errorf("failed to detect state files: %w", err)
		}
		if len(detected) == 0 {
			return fmt.Errorf("no state files found in %s/mount or %s/adb", req.DestRoot, req.DestRoot)
		}
		// Extract modes
		stateFilesToProcess = []string{}
		for _, info := range detected {
			stateFilesToProcess = append(stateFilesToProcess, info.Mode)
		}
	}

	// Run cleanup in goroutine (non-blocking)
	go func() {
		defer s.jobManager.CompleteJob(true)

		for _, mode := range stateFilesToProcess {
			// Check if job was cancelled
			select {
			case <-jobCtx.Done():
				s.logger.Printf("[CleanupService] StartCleanup: Job cancelled")
				runtime.EventsEmit(s.ctx, "CleanupProgress", map[string]interface{}{
					"jobId":  jobID,
					"status": "cancelled",
				})
				return
			default:
			}

			// Process this state file
			err := s.processCleanupForMode(jobCtx, jobID, req.SourceRoot, req.DestRoot, mode)
			if err != nil {
				s.logger.Printf("[CleanupService] StartCleanup: Error processing %s mode: %v", mode, err)
				runtime.EventsEmit(s.ctx, "CleanupProgress", map[string]interface{}{
					"jobId":  jobID,
					"status": "error",
					"mode":   mode,
					"error":  err.Error(),
				})
				// Continue to next mode
				continue
			}
		}

		// Emit completion
		runtime.EventsEmit(s.ctx, "CleanupProgress", map[string]interface{}{
			"jobId":  jobID,
			"status": "completed",
		})
	}()

	return nil
}

// processCleanupForMode processes cleanup for a specific mode (mount or adb)
func (s *CleanupService) processCleanupForMode(ctx context.Context, jobID string, sourceRoot, destRoot, mode string) error {
	s.logger.Printf("[CleanupService] processCleanupForMode: mode=%s sourceRoot=%s destRoot=%s", mode, sourceRoot, destRoot)

	// Build state file path
	destPath := filepath.Join(destRoot, mode)
	stateFile := filepath.Join(destPath, stateFileName)

	// Check if state file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return fmt.Errorf("state file not found: %s", stateFile)
	}

	// Emit log line
	runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
		"timestamp": fmt.Sprintf("%d", ctx.Value("timestamp")),
		"level":     "info",
		"message":   fmt.Sprintf("Processing cleanup for %s mode (state file: %s)", mode, stateFile),
	})

	// TODO: Call existing runCleanupMode logic
	// For now, this is a stub that will be wired to the actual cleanup function
	// The existing cleanup logic is in cleanup.go:runCleanupMode()
	// We need to refactor it slightly to support processing multiple state files

	// Note: The existing runCleanupMode only processes one state file (first found)
	// We'll need to modify it or create a wrapper that can process a specific state file

	return nil
}

// CancelCleanup cancels the current cleanup operation
func (s *CleanupService) CancelCleanup() error {
	s.logger.Printf("[CleanupService] CancelCleanup: Cancelling cleanup operation")
	return s.jobManager.CancelJob()
}


