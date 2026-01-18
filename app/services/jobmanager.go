package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"syscall"
	"time"
)

// JobManager manages long-running jobs (copy, verify, cleanup)
// Ensures only one job runs at a time
type JobManager struct {
	mu         sync.Mutex
	currentJob *JobInfo
	ctx        context.Context
	logger     *log.Logger
}

// NewJobManager creates a new JobManager
func NewJobManager(ctx context.Context, logger *log.Logger) *JobManager {
	return &JobManager{
		ctx:    ctx,
		logger: logger,
	}
}

// JobInfo represents information about a running job
type JobInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "copy", "verify", "cleanup"
	Status    string    `json:"status"` // "running", "completed", "failed", "cancelled"
	StartTime time.Time `json:"startTime"`
	Cancel    context.CancelFunc
	ctx       context.Context
	processPID int // Process PID for signal handling
	processPGID int // Process group ID for SIGINT on Unix
	mu        sync.Mutex
}

// StartJob starts a new job if no job is currently running
// Returns job ID and error if job already running
func (jm *JobManager) StartJob(jobType string) (string, context.Context, error) {
	jm.logger.Printf("[JobManager] StartJob: type=%s", jobType)

	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.currentJob != nil && jm.currentJob.Status == "running" {
		return "", nil, fmt.Errorf("job %s is already running (type: %s)", jm.currentJob.ID, jm.currentJob.Type)
	}

	// Create new job
	jobCtx, cancel := context.WithCancel(jm.ctx)
	jobID := fmt.Sprintf("%s-%d", jobType, time.Now().Unix())

	jm.currentJob = &JobInfo{
		ID:        jobID,
		Type:      jobType,
		Status:    "running",
		StartTime: time.Now(),
		Cancel:    cancel,
		ctx:       jobCtx,
	}

	jm.logger.Printf("[JobManager] StartJob: Created job id=%s type=%s", jobID, jobType)
	return jobID, jobCtx, nil
}

// SetProcessPID stores the process PID and PGID for signal handling
func (jm *JobManager) SetProcessPID(pid int, pgid int) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.currentJob != nil {
		jm.currentJob.mu.Lock()
		jm.currentJob.processPID = pid
		jm.currentJob.processPGID = pgid
		jm.currentJob.mu.Unlock()
		jm.logger.Printf("[JobManager] SetProcessPID: pid=%d pgid=%d for job %s", pid, pgid, jm.currentJob.ID)
	}
}

// CancelJob cancels the current job if running
// On Unix, sends SIGINT to the process group (like Ctrl-C)
// On Windows, kills the process
func (jm *JobManager) CancelJob() error {
	jm.logger.Printf("[JobManager] CancelJob: Attempting to cancel current job")

	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.currentJob == nil {
		return fmt.Errorf("no job is currently running")
	}

	if jm.currentJob.Status != "running" {
		return fmt.Errorf("job %s is not running (status: %s)", jm.currentJob.ID, jm.currentJob.Status)
	}

	jm.currentJob.mu.Lock()
	jm.currentJob.Status = "cancelled"
	pid := jm.currentJob.processPID
	pgid := jm.currentJob.processPGID
	jm.currentJob.mu.Unlock()

	// Cancel context first
	if jm.currentJob.Cancel != nil {
		jm.currentJob.Cancel()
	}

	// Send SIGINT to process group (Unix) or kill process (Windows)
	if pid > 0 {
		if pgid > 0 {
			// Send SIGINT to entire process group (like Ctrl-C)
			jm.logger.Printf("[JobManager] CancelJob: Sending SIGINT to process group %d", pgid)
			if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil {
				jm.logger.Printf("[JobManager] CancelJob: Failed to send SIGINT to pgid %d: %v", pgid, err)
				// Fallback to individual process
				syscall.Kill(pid, syscall.SIGINT)
			}
		} else {
			// Fallback: send SIGINT to individual process
			jm.logger.Printf("[JobManager] CancelJob: Sending SIGINT to process %d", pid)
			syscall.Kill(pid, syscall.SIGINT)
		}
	}

	jm.logger.Printf("[JobManager] CancelJob: Cancelled job id=%s", jm.currentJob.ID)
	return nil
}

// GetJobStatus returns the status of the current job
func (jm *JobManager) GetJobStatus() *JobInfo {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.currentJob == nil {
		return nil
	}

	// Return a copy
	return &JobInfo{
		ID:        jm.currentJob.ID,
		Type:      jm.currentJob.Type,
		Status:    jm.currentJob.Status,
		StartTime: jm.currentJob.StartTime,
	}
}

// CompleteJob marks the current job as completed
func (jm *JobManager) CompleteJob(success bool) error {
	jm.logger.Printf("[JobManager] CompleteJob: success=%v", success)

	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.currentJob == nil {
		return fmt.Errorf("no job is currently running")
	}

	jm.currentJob.mu.Lock()
	if success {
		jm.currentJob.Status = "completed"
	} else {
		jm.currentJob.Status = "failed"
	}
	jm.currentJob.mu.Unlock()

	jm.logger.Printf("[JobManager] CompleteJob: Job id=%s status=%s", jm.currentJob.ID, jm.currentJob.Status)
	return nil
}


