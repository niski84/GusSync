package core

import (
	"context"
	"sync"
	"testing"
	"time"
)

// MockEmitter captures emitted events for testing
type MockEmitter struct {
	mu     sync.Mutex
	events []JobUpdateEvent
}

func NewMockEmitter() *MockEmitter {
	return &MockEmitter{
		events: make([]JobUpdateEvent, 0),
	}
}

func (m *MockEmitter) EmitJobUpdate(event JobUpdateEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *MockEmitter) Events() []JobUpdateEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]JobUpdateEvent{}, m.events...)
}

func (m *MockEmitter) LastEvent() *JobUpdateEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	return &m.events[len(m.events)-1]
}

func (m *MockEmitter) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = m.events[:0]
}

func TestJobManager_StartJob(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	// Start a job
	jobID, jobCtx, err := jm.StartJob(ctx, "copy.sync", "Starting backup", map[string]string{"src": "/test"})
	if err != nil {
		t.Fatalf("StartJob failed: %v", err)
	}

	if jobID == "" {
		t.Error("jobID should not be empty")
	}

	if jobCtx == nil {
		t.Error("jobCtx should not be nil")
	}

	// Verify event was emitted
	events := emitter.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].State != JobRunning {
		t.Errorf("expected state running, got %s", events[0].State)
	}

	if events[0].Seq != 1 {
		t.Errorf("expected seq 1, got %d", events[0].Seq)
	}
}

func TestJobManager_SingleJobAtATime(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	// Start first job
	_, _, err := jm.StartJob(ctx, "job1", "First job", nil)
	if err != nil {
		t.Fatalf("first StartJob failed: %v", err)
	}

	// Try to start second job - should fail
	_, _, err = jm.StartJob(ctx, "job2", "Second job", nil)
	if err == nil {
		t.Error("expected error when starting second job, got nil")
	}
}

func TestJobManager_CancelJob(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	jobID, jobCtx, _ := jm.StartJob(ctx, "test", "Test job", nil)

	// Cancel the job
	err := jm.CancelJob(jobID)
	if err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	// Verify context is cancelled
	select {
	case <-jobCtx.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("job context was not cancelled")
	}

	// Verify state
	snapshot, _ := jm.GetJob(jobID)
	if snapshot.State != JobCanceled {
		t.Errorf("expected state canceled, got %s", snapshot.State)
	}
}

func TestJobManager_CompleteJob(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	jobID, _, _ := jm.StartJob(ctx, "test", "Test job", nil)

	// Complete the job
	jm.CompleteJob(jobID, "All done!")

	// Verify state
	snapshot, _ := jm.GetJob(jobID)
	if snapshot.State != JobSucceeded {
		t.Errorf("expected state succeeded, got %s", snapshot.State)
	}
	if snapshot.Progress.Percent != 100 {
		t.Errorf("expected progress 100, got %f", snapshot.Progress.Percent)
	}
	if snapshot.Message != "All done!" {
		t.Errorf("expected message 'All done!', got '%s'", snapshot.Message)
	}
}

func TestJobManager_FailJob(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	jobID, _, _ := jm.StartJob(ctx, "test", "Test job", nil)

	// Fail the job
	jm.FailJob(jobID, &testError{msg: "disk full"}, "No space left")

	// Verify state
	snapshot, _ := jm.GetJob(jobID)
	if snapshot.State != JobFailed {
		t.Errorf("expected state failed, got %s", snapshot.State)
	}
	if snapshot.Error == nil {
		t.Error("expected error to be set")
	} else if snapshot.Error.Message != "disk full" {
		t.Errorf("expected error message 'disk full', got '%s'", snapshot.Error.Message)
	}
}

func TestJobManager_UpdateProgress(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	jobID, _, _ := jm.StartJob(ctx, "test", "Test job", nil)
	emitter.Clear() // Clear start event

	// Update progress
	progress := JobProgress{
		Phase:   "copying",
		Current: 50,
		Total:   100,
		Percent: 50.0,
		Rate:    5.5,
	}
	workers := map[int]string{0: "copying file.txt", 1: "idle"}
	jm.UpdateProgress(jobID, progress, "Halfway done", workers)

	// Verify state
	snapshot, _ := jm.GetJob(jobID)
	if snapshot.Progress.Percent != 50.0 {
		t.Errorf("expected progress 50, got %f", snapshot.Progress.Percent)
	}
	if snapshot.Progress.Phase != "copying" {
		t.Errorf("expected phase 'copying', got '%s'", snapshot.Progress.Phase)
	}
	if snapshot.Message != "Halfway done" {
		t.Errorf("expected message 'Halfway done', got '%s'", snapshot.Message)
	}
	if len(snapshot.Workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(snapshot.Workers))
	}
}

func TestJobManager_Throttling(t *testing.T) {
	emitter := NewMockEmitter()
	// Use fast throttle for testing
	throttle := ThrottleConfig{MinInterval: 50 * time.Millisecond}
	jm := NewJobManagerWithThrottle(emitter, throttle)
	ctx := context.Background()

	jobID, _, _ := jm.StartJob(ctx, "test", "Test job", nil)
	emitter.Clear() // Clear start event

	// Rapid updates - only some should emit
	for i := 0; i < 10; i++ {
		progress := JobProgress{Current: int64(i), Total: 10, Percent: float64(i) * 10}
		jm.UpdateProgress(jobID, progress, "", nil)
	}

	// Should have fewer than 10 events due to throttling
	events := emitter.Events()
	if len(events) >= 10 {
		t.Errorf("expected throttling to reduce events, got %d", len(events))
	}

	// Wait for throttle to expire
	time.Sleep(60 * time.Millisecond)

	// This update should emit
	jm.UpdateProgress(jobID, JobProgress{Percent: 100}, "Done", nil)

	// Verify state is always updated even when throttled
	snapshot, _ := jm.GetJob(jobID)
	if snapshot.Progress.Percent != 100 {
		t.Errorf("expected final progress 100, got %f", snapshot.Progress.Percent)
	}
}

func TestJobManager_SequenceNumbers(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	// Start first job
	jobID1, _, _ := jm.StartJob(ctx, "test1", "First", nil)
	jm.CompleteJob(jobID1, "Done")

	// Start second job
	jobID2, _, _ := jm.StartJob(ctx, "test2", "Second", nil)
	jm.CompleteJob(jobID2, "Done")

	events := emitter.Events()
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Verify monotonically increasing sequence numbers
	for i := 1; i < len(events); i++ {
		if events[i].Seq <= events[i-1].Seq {
			t.Errorf("seq numbers not increasing: %d <= %d", events[i].Seq, events[i-1].Seq)
		}
	}
}

func TestJobManager_GetActiveJob(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	// No active job initially
	if jm.GetActiveJob() != nil {
		t.Error("expected no active job initially")
	}

	// Start a job
	jobID, _, _ := jm.StartJob(ctx, "test", "Test", nil)

	// Should return active job
	active := jm.GetActiveJob()
	if active == nil {
		t.Fatal("expected active job, got nil")
	}
	if active.JobID != jobID {
		t.Errorf("expected jobID %s, got %s", jobID, active.JobID)
	}

	// Complete the job
	jm.CompleteJob(jobID, "Done")

	// No active job after completion
	if jm.GetActiveJob() != nil {
		t.Error("expected no active job after completion")
	}
}

func TestJobManager_ListJobs(t *testing.T) {
	emitter := NewMockEmitter()
	jm := NewJobManager(emitter)
	ctx := context.Background()

	// Create and complete several jobs sequentially
	// Sleep briefly between jobs to ensure different timestamps
	for i := 0; i < 3; i++ {
		jobID, _, err := jm.StartJob(ctx, "test", "Test", nil)
		if err != nil {
			t.Fatalf("StartJob %d failed: %v", i, err)
		}
		jm.CompleteJob(jobID, "Done")
		time.Sleep(2 * time.Millisecond) // Ensure different timestamps
	}

	jobs := jm.ListJobs()
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}

	// Verify sorted by creation time (newest first)
	for i := 1; i < len(jobs); i++ {
		if jobs[i].CreatedAt.After(jobs[i-1].CreatedAt) {
			t.Error("jobs not sorted newest first")
		}
	}
}

func TestJobManager_NilEmitter(t *testing.T) {
	// Should not panic with nil emitter
	jm := NewJobManager(nil)
	ctx := context.Background()

	jobID, _, err := jm.StartJob(ctx, "test", "Test", nil)
	if err != nil {
		t.Fatalf("StartJob failed: %v", err)
	}

	// These should not panic
	jm.UpdateProgress(jobID, JobProgress{Percent: 50}, "", nil)
	jm.CompleteJob(jobID, "Done")
}

// testError implements error interface for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

