package services

import (
	"GusSync/internal/core"
	"context"
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsJobEmitter implements core.JobEventEmitter for Wails frontend
type WailsJobEmitter struct {
	ctx    context.Context
	logger *log.Logger
}

// EmitJobUpdate sends job update events to the Wails frontend
func (e *WailsJobEmitter) EmitJobUpdate(event core.JobUpdateEvent) {
	if e.ctx == nil {
		return
	}

	// Convert core event to the existing task:update format for frontend compatibility
	taskEvent := TaskUpdateEvent{
		TaskID:   event.JobID,
		Seq:      event.Seq,
		Type:     event.Type,
		State:    TaskState(event.State), // Convert core.JobState to TaskState
		Progress: TaskProgress(event.Progress),
		Message:  event.Message,
		LogLine:  event.LogLine,
		Workers:  event.Workers,
		Artifact: TaskArtifact(event.Artifact),
	}

	if event.Error != nil {
		taskEvent.Error = &TaskError{
			Code:    event.Error.Code,
			Message: event.Error.Message,
			Details: event.Error.Details,
		}
	}

	runtime.EventsEmit(e.ctx, "task:update", taskEvent)
}

// JobManager wraps the core.JobManager and provides Wails-specific functionality.
// This is the adapter layer that translates between core and Wails.
type JobManager struct {
	core    *core.JobManager
	emitter *WailsJobEmitter
	ctx     context.Context
	logger  *log.Logger
}

// NewJobManager creates a new JobManager
func NewJobManager(ctx context.Context, logger *log.Logger) *JobManager {
	emitter := &WailsJobEmitter{ctx: ctx, logger: logger}
	coreManager := core.NewJobManager(emitter)

	return &JobManager{
		core:    coreManager,
		emitter: emitter,
		ctx:     ctx,
		logger:  logger,
	}
}

// SetContext updates the context for the JobManager
func (jm *JobManager) SetContext(ctx context.Context) {
	jm.ctx = ctx
	jm.emitter.ctx = ctx
}

// startTask starts a new task and returns a taskId immediately
func (jm *JobManager) startTask(taskType string, message string, params map[string]string) (string, context.Context, error) {
	jm.logger.Printf("[JobManager] startTask: type=%s msg=%s", taskType, message)
	return jm.core.StartJob(jm.ctx, taskType, message, params)
}

// updateTaskProgress updates the progress of a task
func (jm *JobManager) updateTaskProgress(taskID string, progress TaskProgress, message string, workers map[int]string) {
	coreProgress := core.JobProgress{
		Phase:   progress.Phase,
		Current: progress.Current,
		Total:   progress.Total,
		Percent: progress.Percent,
		Rate:    progress.Rate,
	}
	jm.core.UpdateProgress(taskID, coreProgress, message, workers)
}

// completeTask marks a task as succeeded
func (jm *JobManager) completeTask(taskID string, message string) {
	jm.core.CompleteJob(taskID, message)
}

// failTask marks a task as failed
func (jm *JobManager) failTask(taskID string, err error, details string) {
	jm.core.FailJob(taskID, err, details)
}

// GetTask returns a snapshot of a task
func (jm *JobManager) GetTask(taskID string) (*TaskSnapshot, error) {
	coreSnapshot, err := jm.core.GetJob(taskID)
	if err != nil {
		return nil, err
	}
	return coreSnapshotToTask(coreSnapshot), nil
}

// GetActiveTask returns the currently active task snapshot, or nil if no task is running.
// This is used for startup handshake - UI calls this on mount to get current state before subscribing to events.
func (jm *JobManager) GetActiveTask() *TaskSnapshot {
	coreSnapshot := jm.core.GetActiveJob()
	if coreSnapshot == nil {
		return nil
	}
	return coreSnapshotToTask(coreSnapshot)
}

// ListTasks returns all tasks, sorted by creation time (newest first)
func (jm *JobManager) ListTasks() []*TaskSnapshot {
	coreSnapshots := jm.core.ListJobs()
	result := make([]*TaskSnapshot, len(coreSnapshots))
	for i, cs := range coreSnapshots {
		result[i] = coreSnapshotToTask(cs)
	}
	return result
}

// CancelTask cancels a running task
func (jm *JobManager) CancelTask(taskID string) error {
	return jm.core.CancelJob(taskID)
}

// CancelJob cancels the currently active job
func (jm *JobManager) CancelJob() error {
	return jm.core.CancelActiveJob()
}

// completeJob marks the active job as complete (legacy support)
func (jm *JobManager) completeJob(success bool) error {
	activeSnapshot := jm.core.GetActiveJob()
	if activeSnapshot == nil {
		return nil // No active job
	}
	if success {
		jm.core.CompleteJob(activeSnapshot.JobID, "Completed successfully")
	} else {
		jm.core.FailJob(activeSnapshot.JobID, nil, "Job failed")
	}
	return nil
}

// GetJobStatus returns the current job status (legacy support)
func (jm *JobManager) GetJobStatus() *JobInfo {
	coreSnapshot := jm.core.GetActiveJob()
	if coreSnapshot == nil {
		return nil
	}
	return &JobInfo{
		ID:        coreSnapshot.JobID,
		Type:      coreSnapshot.Type,
		Status:    string(coreSnapshot.State),
		StartTime: coreSnapshot.CreatedAt,
	}
}

// startJob starts a job (legacy support)
func (jm *JobManager) startJob(jobType string) (string, context.Context, error) {
	return jm.startTask(jobType, "Starting "+jobType, nil)
}

// SetProcessPID is not used in the new model but kept for compatibility
func (jm *JobManager) SetProcessPID(pid int, pgid int) {
	// Not used in new Task model
}

// EmitLogLine emits a log line event for a specific job
func (jm *JobManager) EmitLogLine(jobID string, logLine string) {
	jm.core.EmitLogLine(jobID, logLine)
}

// Legacy JobInfo type for backwards compatibility
type JobInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"startTime"`
}

// coreSnapshotToTask converts a core.JobSnapshot to a TaskSnapshot
func coreSnapshotToTask(cs *core.JobSnapshot) *TaskSnapshot {
	if cs == nil {
		return nil
	}

	ts := &TaskSnapshot{
		TaskID:    cs.JobID,
		Seq:       cs.Seq,
		Type:      cs.Type,
		State:     TaskState(cs.State),
		Params:    cs.Params,
		Progress:  TaskProgress(cs.Progress),
		Message:   cs.Message,
		Workers:   cs.Workers,
		Artifact:  TaskArtifact(cs.Artifact),
		CreatedAt: cs.CreatedAt,
		UpdatedAt: cs.UpdatedAt,
	}

	if cs.Error != nil {
		ts.Error = &TaskError{
			Code:    cs.Error.Code,
			Message: cs.Error.Message,
			Details: cs.Error.Details,
		}
	}

	return ts
}
