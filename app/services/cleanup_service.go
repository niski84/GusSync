package services

import (
	"GusSync/pkg/engine"
	"GusSync/pkg/state"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// CleanupService handles cleanup operations using the core engine
type CleanupService struct {
	ctx           context.Context
	logger        *log.Logger
	jobManager    *JobManager
	deviceService *DeviceService
}

const stateFileName = "gus_state.md"

// NewCleanupService creates a new CleanupService
func NewCleanupService(ctx context.Context, logger *log.Logger, jobManager *JobManager, deviceService *DeviceService) *CleanupService {
	return &CleanupService{
		ctx:           ctx,
		logger:        logger,
		jobManager:    jobManager,
		deviceService: deviceService,
	}
}

// SetContext updates the service context
func (s *CleanupService) SetContext(ctx context.Context) {
	s.ctx = ctx
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
func (s *CleanupService) StartCleanup(req CleanupRequest) (string, error) {
	s.logger.Printf("[CleanupService] StartCleanup: sourceRoot=%s destRoot=%s stateFiles=%v processBoth=%v", req.SourceRoot, req.DestRoot, req.StateFiles, req.ProcessBoth)

	params := map[string]string{
		"sourceRoot": req.SourceRoot,
		"destRoot":   req.DestRoot,
	}

	// Start job
	jobID, jobCtx, err := s.jobManager.startTask("cleanup.sync", "Initializing cleanup...", params)
	if err != nil {
		return "", fmt.Errorf("failed to start cleanup task: %w", err)
	}

	// Determine which state files to process
	stateFilesToProcess := req.StateFiles
	if req.ProcessBoth || len(stateFilesToProcess) == 0 {
		// Detect all available state files
		detected, err := s.DetectStateFiles(req.DestRoot)
		if err != nil {
			return "", fmt.Errorf("failed to detect state files: %w", err)
		}
		if len(detected) == 0 {
			return "", fmt.Errorf("no state files found in %s/mount or %s/adb", req.DestRoot, req.DestRoot)
		}
		// Extract modes
		stateFilesToProcess = []string{}
		for _, info := range detected {
			stateFilesToProcess = append(stateFilesToProcess, info.Mode)
		}
	}

	// Run cleanup in goroutine (non-blocking)
	go func() {
		for _, mode := range stateFilesToProcess {
			// Check if job was cancelled
			select {
			case <-jobCtx.Done():
				s.logger.Printf("[CleanupService] StartCleanup: Job cancelled")
				return
			default:
			}

			// Process this state file
			_ = s.processCleanupForMode(jobCtx, jobID, req.SourceRoot, req.DestRoot, mode)
		}
	}()

	return jobID, nil
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

	reporter := &WailsReporter{ctx: s.ctx, jobID: jobID, jobManager: s.jobManager}
	reporter.ReportLog("info", fmt.Sprintf("Processing cleanup for %s mode (state file: %s)", mode, stateFile))
	reporter.ReportLog("info", "Loading state file...")

	stateManager, err := state.NewStateManager(stateFile)
	if err != nil {
		return err
	}
	defer stateManager.Close()

	cfg := engine.EngineConfig{
		SourcePath: sourceRoot,
		DestRoot:   destPath,
		Mode:       mode,
		NumWorkers: 2,
		Reporter:   reporter,
	}

	e := engine.NewEngine(cfg, stateManager)
	
	s.jobManager.updateTaskProgress(jobID, TaskProgress{Phase: "cleaning"}, "Cleanup in progress...", nil)

	results, err := e.RunCleanup(ctx)
	if err != nil {
		s.jobManager.failTask(jobID, err, "")
		return err
	}

	message := fmt.Sprintf("Cleanup complete for %s: %d deleted, %d failed", mode, results.Deleted, results.Failed)
	s.jobManager.completeTask(jobID, message)

	return nil
}

// CancelCleanup cancels the current cleanup operation
func (s *CleanupService) CancelCleanup() error {
	s.logger.Printf("[CleanupService] CancelCleanup: Cancelling cleanup operation")
	return s.jobManager.CancelJob()
}


