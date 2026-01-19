package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// JobManager manages long-running tasks (copy, verify, cleanup)
// Ensures only one task runs at a time and maintains task history
type JobManager struct {
	mu         sync.Mutex
	tasks      map[string]*TaskSnapshot
	activeTask string // ID of the currently running task
	ctx        context.Context
	logger     *log.Logger
	cancels    map[string]context.CancelFunc
}

// NewJobManager creates a new JobManager
func NewJobManager(ctx context.Context, logger *log.Logger) *JobManager {
	return &JobManager{
		ctx:     ctx,
		logger:  logger,
		tasks:   make(map[string]*TaskSnapshot),
		cancels: make(map[string]context.CancelFunc),
	}
}

// SetContext updates the context for the JobManager
func (jm *JobManager) SetContext(ctx context.Context) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.ctx = ctx
}

// startTask starts a new task and returns a taskId immediately
func (jm *JobManager) startTask(taskType string, message string, params map[string]string) (string, context.Context, error) {
	jm.mu.Lock()

	// Check if a task is already running
	if jm.activeTask != "" {
		active := jm.tasks[jm.activeTask]
		if active != nil && active.State == TaskRunning {
			jm.mu.Unlock()
			return "", nil, fmt.Errorf("a task is already running: %s (%s)", active.TaskID, active.Type)
		}
	}

	taskID := fmt.Sprintf("%s-%d", taskType, time.Now().Unix())
	jobCtx, cancel := context.WithCancel(jm.ctx)

	snapshot := &TaskSnapshot{
		TaskID:    taskID,
		Type:      taskType,
		State:     TaskRunning,
		Params:    params,
		Message:   message,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Progress: TaskProgress{
			Phase: "starting",
		},
	}

	jm.tasks[taskID] = snapshot
	jm.cancels[taskID] = cancel
	jm.activeTask = taskID

	jm.logger.Printf("[JobManager] startTask: %s type=%s", taskID, taskType)
	
	// IMPORTANT: Release lock BEFORE calling emitTaskUpdate to avoid deadlock
	jm.mu.Unlock()
	
	// Emit initial event (must be outside lock to avoid deadlock)
	jm.emitTaskUpdate(taskID)

	return taskID, jobCtx, nil
}

// emitTaskUpdate sends the current task state to the frontend
func (jm *JobManager) emitTaskUpdate(taskID string) {
	jm.mu.Lock()
	snapshot, exists := jm.tasks[taskID]
	jm.mu.Unlock()

	if !exists {
		return
	}

	event := TaskUpdateEvent{
		TaskID:   snapshot.TaskID,
		Type:     snapshot.Type,
		State:    snapshot.State,
		Progress: snapshot.Progress,
		Message:  snapshot.Message,
		Workers:  snapshot.Workers,
		Error:    snapshot.Error,
		Artifact: snapshot.Artifact,
	}

	if jm.ctx != nil {
		runtime.EventsEmit(jm.ctx, "task:update", event)
	}
}

// updateTaskProgress updates the progress of a task
func (jm *JobManager) updateTaskProgress(taskID string, progress TaskProgress, message string, workers map[int]string) {
	jm.mu.Lock()
	snapshot, exists := jm.tasks[taskID]
	if exists {
		snapshot.Progress = progress
		if message != "" {
			snapshot.Message = message
		}
		if workers != nil {
			snapshot.Workers = workers
		}
		snapshot.UpdatedAt = time.Now()
	}
	jm.mu.Unlock()

	if exists {
		jm.emitTaskUpdate(taskID)
	}
}

// completeTask marks a task as succeeded
func (jm *JobManager) completeTask(taskID string, message string) {
	jm.mu.Lock()
	snapshot, exists := jm.tasks[taskID]
	if exists {
		snapshot.State = TaskSucceeded
		if message != "" {
			snapshot.Message = message
		}
		snapshot.Progress.Percent = 100
		snapshot.UpdatedAt = time.Now()
		if jm.activeTask == taskID {
			jm.activeTask = ""
		}
	}
	jm.mu.Unlock()

	if exists {
		jm.emitTaskUpdate(taskID)
	}
}

// failTask marks a task as failed
func (jm *JobManager) failTask(taskID string, err error, details string) {
	jm.mu.Lock()
	snapshot, exists := jm.tasks[taskID]
	if exists {
		snapshot.State = TaskFailed
		snapshot.Error = &TaskError{
			Message: err.Error(),
			Details: details,
		}
		snapshot.UpdatedAt = time.Now()
		if jm.activeTask == taskID {
			jm.activeTask = ""
		}
	}
	jm.mu.Unlock()

	if exists {
		jm.emitTaskUpdate(taskID)
	}
}

// GetTask returns a snapshot of a task
func (jm *JobManager) GetTask(taskID string) (*TaskSnapshot, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	snapshot, exists := jm.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return snapshot, nil
}

// ListTasks returns all tasks, sorted by creation time (newest first)
func (jm *JobManager) ListTasks() []*TaskSnapshot {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	list := make([]*TaskSnapshot, 0, len(jm.tasks))
	for _, t := range jm.tasks {
		list = append(list, t)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list
}

// CancelTask cancels a running task
func (jm *JobManager) CancelTask(taskID string) error {
	jm.mu.Lock()
	cancel, exists := jm.cancels[taskID]
	snapshot, sExists := jm.tasks[taskID]
	jm.mu.Unlock()

	if !exists {
		return fmt.Errorf("task not found or not cancellable: %s", taskID)
	}

	cancel()

	if sExists {
		jm.mu.Lock()
		snapshot.State = TaskCanceled
		snapshot.Message = "Task canceled by user"
		snapshot.UpdatedAt = time.Now()
		if jm.activeTask == taskID {
			jm.activeTask = ""
		}
		jm.mu.Unlock()
		jm.emitTaskUpdate(taskID)
	}

	return nil
}

// Legacy support for JobManager (to be removed once refactoring complete)

type JobInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"startTime"`
}

func (jm *JobManager) startJob(jobType string) (string, context.Context, error) {
	return jm.startTask(jobType, "Starting "+jobType, nil)
}

func (jm *JobManager) CancelJob() error {
	jm.mu.Lock()
	active := jm.activeTask
	jm.mu.Unlock()
	if active == "" {
		return fmt.Errorf("no active task to cancel")
	}
	return jm.CancelTask(active)
}

func (jm *JobManager) completeJob(success bool) error {
	jm.mu.Lock()
	active := jm.activeTask
	jm.mu.Unlock()
	if active == "" {
		return fmt.Errorf("no active task to complete")
	}
	if success {
		jm.completeTask(active, "Completed successfully")
	} else {
		jm.failTask(active, fmt.Errorf("job failed"), "")
	}
	return nil
}

func (jm *JobManager) GetJobStatus() *JobInfo {
	jm.mu.Lock()
	activeID := jm.activeTask
	jm.mu.Unlock()

	if activeID == "" {
		return nil
	}

	jm.mu.Lock()
	t := jm.tasks[activeID]
	jm.mu.Unlock()

	if t == nil {
		return nil
	}

	return &JobInfo{
		ID:        t.TaskID,
		Type:      t.Type,
		Status:    string(t.State),
		StartTime: t.CreatedAt,
	}
}

func (jm *JobManager) SetProcessPID(pid int, pgid int) {
	// Not used in new Task model for now, but keeping for compatibility
}



