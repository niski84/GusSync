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

// VerifyService handles verification operations using the core engine
type VerifyService struct {
	ctx           context.Context
	logger        *log.Logger
	jobManager    *JobManager
	deviceService *DeviceService
}

// NewVerifyService creates a new VerifyService
func NewVerifyService(ctx context.Context, logger *log.Logger, jobManager *JobManager, deviceService *DeviceService) *VerifyService {
	return &VerifyService{
		ctx:           ctx,
		logger:        logger,
		jobManager:    jobManager,
		deviceService: deviceService,
	}
}

// SetContext updates the context for the VerifyService
func (s *VerifyService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// VerifyRequest represents a verification operation request
type VerifyRequest struct {
	SourcePath string `json:"sourcePath"`
	DestPath   string `json:"destPath"`
	Mode       string `json:"mode"`
}

// StartVerify starts a verification operation (non-blocking)
func (s *VerifyService) StartVerify(req VerifyRequest) (string, error) {
	s.logger.Printf("[VerifyService] StartVerify: sourcePath=%s destPath=%s mode=%s", req.SourcePath, req.DestPath, req.Mode)

	// Auto-detect source if empty
	sourcePath := req.SourcePath
	if sourcePath == "" {
		devices, _ := s.deviceService.GetDeviceStatus()
		if len(devices) > 0 {
			sourcePath = devices[0].Path
		}
	}

	if sourcePath == "" {
		sourcePath = "/sdcard"
	}

	params := map[string]string{
		"sourcePath": sourcePath,
		"destPath":   req.DestPath,
		"mode":       req.Mode,
	}

	jobID, jobCtx, err := s.jobManager.startTask("verify.backup", "Initializing verification...", params)
	if err != nil {
		return "", err
	}

	// Modes to process
	modes := []string{req.Mode}
	if req.Mode == "" || req.Mode == "auto" {
		modes = []string{}
		if _, err := os.Stat(filepath.Join(req.DestPath, "mount", "gus_state.md")); err == nil {
			modes = append(modes, "mount")
		}
		if _, err := os.Stat(filepath.Join(req.DestPath, "adb", "gus_state.md")); err == nil {
			modes = append(modes, "adb")
		}
	}

	if len(modes) == 0 {
		s.jobManager.failTask(jobID, fmt.Errorf("no state files found"), "No gus_state.md found in destination")
		return jobID, nil
	}

	go func() {
		for _, mode := range modes {
			select {
			case <-jobCtx.Done():
				return
			default:
			}

			fullDestPath := filepath.Join(req.DestPath, mode)
			reporter := &WailsReporter{ctx: s.ctx, jobID: jobID, jobManager: s.jobManager}
			reporter.ReportLog("info", fmt.Sprintf("Verifying %s mode...", mode))
			
			stateFile := filepath.Join(fullDestPath, "gus_state.md")
			stateManager, err := state.NewStateManager(stateFile)
			if err != nil {
				reporter.ReportError(fmt.Errorf("failed to initialize state for %s: %w", mode, err))
				continue
			}

			cfg := engine.EngineConfig{
				SourcePath: sourcePath,
				DestRoot:   fullDestPath,
				Mode:       mode,
				NumWorkers: 2,
				Reporter:   reporter,
			}

			e := engine.NewEngine(cfg, stateManager)
			s.jobManager.updateTaskProgress(jobID, TaskProgress{Phase: "verifying"}, fmt.Sprintf("Verifying %s...", mode), nil)

			results, err := e.VerifyBackup(jobCtx)
			stateManager.Close()

			if err != nil {
				reporter.ReportLog("error", fmt.Sprintf("%s verification failed: %v", mode, err))
			} else {
				message := fmt.Sprintf("%s verification complete: %d verified, %d mismatches", mode, results.Verified, results.Mismatches)
				reporter.ReportLog("info", message)
			}
		}
		s.jobManager.completeTask(jobID, "Verification process finished")
	}()

	return jobID, nil
}

// CancelVerify cancels the current verification operation
func (s *VerifyService) CancelVerify() error {
	s.logger.Printf("[VerifyService] CancelVerify: Cancelling verification operation")
	return s.jobManager.CancelJob()
}


