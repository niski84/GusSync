// Package core provides the core business logic and types for GusSync.
// This package must NOT import any adapter-specific code (Wails, Cobra, HTTP frameworks).
// It should be fully testable without UI.
package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobState represents the lifecycle state of a job
type JobState string

const (
	JobQueued    JobState = "queued"
	JobRunning   JobState = "running"
	JobSucceeded JobState = "succeeded"
	JobFailed    JobState = "failed"
	JobCanceled  JobState = "canceled"
)

// JobProgress contains progress information for a running job
type JobProgress struct {
	Phase   string  `json:"phase"`
	Current int64   `json:"current"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
	Rate    float64 `json:"rate"` // e.g., MB/s
}

// JobError contains error information when a job fails
type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

// JobArtifact contains output artifacts from a job (logs, reports, etc.)
type JobArtifact struct {
	LogPath     string `json:"logPath"`
	OpenLogHint string `json:"openLogHint"`
}

// JobSnapshot is the authoritative state of a job at a point in time.
// The UI should derive all state from this snapshot.
type JobSnapshot struct {
	JobID     string            `json:"jobId"`
	Seq       int64             `json:"seq"` // Monotonically increasing sequence number
	Type      string            `json:"type"`
	State     JobState          `json:"state"`
	Params    map[string]string `json:"params,omitempty"`
	Progress  JobProgress       `json:"progress"`
	Message   string            `json:"message"`
	Workers   map[int]string    `json:"workers,omitempty"`
	Error     *JobError         `json:"error,omitempty"`
	Artifact  JobArtifact       `json:"artifact"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// JobUpdateEvent is emitted when job state changes.
// Contains same fields as JobSnapshot but is used for event delivery.
type JobUpdateEvent struct {
	JobID    string         `json:"jobId"`
	Seq      int64          `json:"seq"`
	Type     string         `json:"type"`
	State    JobState       `json:"state"`
	Progress JobProgress    `json:"progress"`
	Message  string         `json:"message"`
	LogLine  string         `json:"logLine,omitempty"`
	Workers  map[int]string `json:"workers,omitempty"`
	Error    *JobError      `json:"error,omitempty"`
	Artifact JobArtifact    `json:"artifact"`
}

// JobEventEmitter is the interface adapters must implement to receive job events.
// This allows the core JobManager to be agnostic about how events are delivered.
type JobEventEmitter interface {
	// EmitJobUpdate is called whenever a job's state changes
	EmitJobUpdate(event JobUpdateEvent)
}

// ThrottleConfig controls how often progress updates are emitted
type ThrottleConfig struct {
	MinInterval time.Duration // Minimum time between progress updates (default: 100ms = ~10/sec)
}

// DefaultThrottleConfig returns sensible defaults for throttling
func DefaultThrottleConfig() ThrottleConfig {
	return ThrottleConfig{
		MinInterval: 100 * time.Millisecond, // ~10 updates per second max
	}
}

// JobManager manages the lifecycle of long-running jobs.
// It is the single source of truth for job state.
// Adapters (Wails, CLI, API) use this to start/stop jobs and get state.
type JobManager struct {
	mu           sync.Mutex
	jobs         map[string]*JobSnapshot
	activeJob    string // ID of the currently running job (only one at a time)
	seqCounter   int64  // Global sequence counter for event ordering
	cancels      map[string]context.CancelFunc
	emitter      JobEventEmitter // Adapter-provided event emitter
	throttle     ThrottleConfig  // Throttling configuration
	lastEmitTime map[string]time.Time // Last emit time per job for throttling
}

// NewJobManager creates a new JobManager with default throttling
func NewJobManager(emitter JobEventEmitter) *JobManager {
	return NewJobManagerWithThrottle(emitter, DefaultThrottleConfig())
}

// NewJobManagerWithThrottle creates a new JobManager with custom throttling
func NewJobManagerWithThrottle(emitter JobEventEmitter, throttle ThrottleConfig) *JobManager {
	return &JobManager{
		jobs:         make(map[string]*JobSnapshot),
		cancels:      make(map[string]context.CancelFunc),
		emitter:      emitter,
		throttle:     throttle,
		lastEmitTime: make(map[string]time.Time),
	}
}

// SetEmitter sets the event emitter (used when emitter is available after construction)
func (jm *JobManager) SetEmitter(emitter JobEventEmitter) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.emitter = emitter
}

// AddEmitter adds an additional emitter. Events will be sent to all registered emitters.
func (jm *JobManager) AddEmitter(emitter JobEventEmitter) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.emitter == nil {
		jm.emitter = emitter
		return
	}

	// Wrap in multi-emitter if not already
	if multi, ok := jm.emitter.(*MultiEmitter); ok {
		multi.Add(emitter)
	} else {
		jm.emitter = &MultiEmitter{emitters: []JobEventEmitter{jm.emitter, emitter}}
	}
}

// MultiEmitter broadcasts events to multiple emitters
type MultiEmitter struct {
	mu       sync.Mutex
	emitters []JobEventEmitter
}

// Add adds an emitter to the multi-emitter
func (m *MultiEmitter) Add(emitter JobEventEmitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitters = append(m.emitters, emitter)
}

// EmitJobUpdate broadcasts the event to all registered emitters
func (m *MultiEmitter) EmitJobUpdate(event JobUpdateEvent) {
	m.mu.Lock()
	emitters := make([]JobEventEmitter, len(m.emitters))
	copy(emitters, m.emitters)
	m.mu.Unlock()

	for _, e := range emitters {
		if e != nil {
			e.EmitJobUpdate(event)
		}
	}
}

// StartJob starts a new job and returns the job ID and context.
// The context is cancelled when CancelJob is called.
func (jm *JobManager) StartJob(ctx context.Context, jobType string, message string, params map[string]string) (string, context.Context, error) {
	jm.mu.Lock()

	// Check if a job is already running
	if jm.activeJob != "" {
		active := jm.jobs[jm.activeJob]
		if active != nil && active.State == JobRunning {
			jm.mu.Unlock()
			return "", nil, fmt.Errorf("a job is already running: %s (%s)", active.JobID, active.Type)
		}
	}

	jobID := fmt.Sprintf("%s-%d", jobType, time.Now().UnixNano())
	jobCtx, cancel := context.WithCancel(ctx)

	snapshot := &JobSnapshot{
		JobID:     jobID,
		Type:      jobType,
		State:     JobRunning,
		Params:    params,
		Message:   message,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Progress: JobProgress{
			Phase: "starting",
		},
	}

	jm.jobs[jobID] = snapshot
	jm.cancels[jobID] = cancel
	jm.activeJob = jobID
	jm.mu.Unlock()

	// Emit initial event
	jm.emitUpdate(jobID)

	return jobID, jobCtx, nil
}

// emitUpdate sends the current job state to the emitter
func (jm *JobManager) emitUpdate(jobID string) {
	jm.mu.Lock()
	snapshot, exists := jm.jobs[jobID]
	if !exists {
		jm.mu.Unlock()
		return
	}

	// Increment sequence counter
	jm.seqCounter++
	snapshot.Seq = jm.seqCounter

	event := JobUpdateEvent{
		JobID:    snapshot.JobID,
		Seq:      snapshot.Seq,
		Type:     snapshot.Type,
		State:    snapshot.State,
		Progress: snapshot.Progress,
		Message:  snapshot.Message,
		Workers:  snapshot.Workers,
		Error:    snapshot.Error,
		Artifact: snapshot.Artifact,
	}

	emitter := jm.emitter
	jm.mu.Unlock()

	if emitter != nil {
		emitter.EmitJobUpdate(event)
	}
}

// UpdateProgress updates the progress of a running job
func (jm *JobManager) UpdateProgress(jobID string, progress JobProgress, message string, workers map[int]string) {
	jm.mu.Lock()
	snapshot, exists := jm.jobs[jobID]
	if !exists {
		jm.mu.Unlock()
		return
	}

	// Always update the internal state
	snapshot.Progress = progress
	if message != "" {
		snapshot.Message = message
	}
	if workers != nil {
		snapshot.Workers = workers
	}
	snapshot.UpdatedAt = time.Now()

	// Check throttling - only emit if enough time has passed
	lastEmit := jm.lastEmitTime[jobID]
	now := time.Now()
	shouldEmit := now.Sub(lastEmit) >= jm.throttle.MinInterval

	if shouldEmit {
		jm.lastEmitTime[jobID] = now
	}
	jm.mu.Unlock()

	if shouldEmit {
		jm.emitUpdate(jobID)
	}
}

// CompleteJob marks a job as succeeded
func (jm *JobManager) CompleteJob(jobID string, message string) {
	jm.mu.Lock()
	snapshot, exists := jm.jobs[jobID]
	if exists {
		snapshot.State = JobSucceeded
		if message != "" {
			snapshot.Message = message
		}
		snapshot.Progress.Percent = 100
		snapshot.UpdatedAt = time.Now()
		if jm.activeJob == jobID {
			jm.activeJob = ""
		}
	}
	jm.mu.Unlock()

	if exists {
		jm.emitUpdate(jobID)
	}
}

// FailJob marks a job as failed
func (jm *JobManager) FailJob(jobID string, err error, details string) {
	jm.mu.Lock()
	snapshot, exists := jm.jobs[jobID]
	if exists {
		snapshot.State = JobFailed
		snapshot.Error = &JobError{
			Message: err.Error(),
			Details: details,
		}
		snapshot.UpdatedAt = time.Now()
		if jm.activeJob == jobID {
			jm.activeJob = ""
		}
	}
	jm.mu.Unlock()

	if exists {
		jm.emitUpdate(jobID)
	}
}

// CancelJob cancels a running job
func (jm *JobManager) CancelJob(jobID string) error {
	jm.mu.Lock()
	cancel, cancelExists := jm.cancels[jobID]
	snapshot, snapshotExists := jm.jobs[jobID]
	jm.mu.Unlock()

	if !cancelExists {
		return fmt.Errorf("job not found or not cancellable: %s", jobID)
	}

	// Cancel the context
	cancel()

	if snapshotExists {
		jm.mu.Lock()
		snapshot.State = JobCanceled
		snapshot.Message = "Job canceled by user"
		snapshot.UpdatedAt = time.Now()
		if jm.activeJob == jobID {
			jm.activeJob = ""
		}
		jm.mu.Unlock()
		jm.emitUpdate(jobID)
	}

	return nil
}

// CancelActiveJob cancels the currently active job
func (jm *JobManager) CancelActiveJob() error {
	jm.mu.Lock()
	active := jm.activeJob
	jm.mu.Unlock()
	if active == "" {
		return fmt.Errorf("no active job to cancel")
	}
	return jm.CancelJob(active)
}

// GetJob returns a snapshot of a specific job
func (jm *JobManager) GetJob(jobID string) (*JobSnapshot, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	snapshot, exists := jm.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Return a copy to prevent race conditions
	copySnapshot := *snapshot
	return &copySnapshot, nil
}

// GetActiveJob returns the currently active job snapshot, or nil if none
func (jm *JobManager) GetActiveJob() *JobSnapshot {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if jm.activeJob == "" {
		return nil
	}

	snapshot, exists := jm.jobs[jm.activeJob]
	if !exists {
		return nil
	}

	// Return a copy to prevent race conditions
	copySnapshot := *snapshot
	return &copySnapshot
}

// ListJobs returns all jobs, sorted by creation time (newest first)
func (jm *JobManager) ListJobs() []*JobSnapshot {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	list := make([]*JobSnapshot, 0, len(jm.jobs))
	for _, j := range jm.jobs {
		copy := *j
		list = append(list, &copy)
	}

	// Sort by creation time (newest first)
	for i := 0; i < len(list)-1; i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].CreatedAt.After(list[i].CreatedAt) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	return list
}

// EmitLogLine emits a log line event for a specific job
func (jm *JobManager) EmitLogLine(jobID string, logLine string) {
	jm.mu.Lock()
	snapshot, exists := jm.jobs[jobID]
	if !exists {
		jm.mu.Unlock()
		return
	}

	// Increment sequence counter
	jm.seqCounter++
	snapshot.Seq = jm.seqCounter

	event := JobUpdateEvent{
		JobID:    snapshot.JobID,
		Seq:      snapshot.Seq,
		Type:     snapshot.Type,
		State:    snapshot.State,
		Progress: snapshot.Progress,
		Message:  snapshot.Message,
		LogLine:  logLine,
		Workers:  snapshot.Workers,
		Error:    snapshot.Error,
		Artifact: snapshot.Artifact,
	}

	emitter := jm.emitter
	jm.mu.Unlock()

	if emitter != nil {
		emitter.EmitJobUpdate(event)
	}
}

