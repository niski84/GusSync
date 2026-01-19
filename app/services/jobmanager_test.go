package services

import (
	"context"
	"log"
	"os"
	"testing"
	"time"
)

func TestJobManager(t *testing.T) {
	logger := log.New(os.Stderr, "[Test] ", log.LstdFlags)
	ctx := context.Background()
	jm := NewJobManager(ctx, logger)

	// 1. Start a task
	taskId, taskCtx, err := jm.startTask("test.task", "Starting test", nil)
	if err != nil {
		t.Fatalf("failed to start task: %v", err)
	}

	if taskId == "" {
		t.Fatal("taskId should not be empty")
	}

	// 2. Try to start another task (should fail)
	_, _, err = jm.startTask("other.task", "Another one", nil)
	if err == nil {
		t.Fatal("should not be able to start concurrent tasks")
	}

	// 3. Verify snapshot
	snapshot, err := jm.GetTask(taskId)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if snapshot.State != TaskRunning {
		t.Errorf("expected task state running, got %s", snapshot.State)
	}

	// 4. Update progress
	progress := TaskProgress{
		Phase:   "testing",
		Percent: 50,
	}
	jm.updateTaskProgress(taskId, progress, "Halfway there", nil)

	snapshot, _ = jm.GetTask(taskId)
	if snapshot.Progress.Percent != 50 {
		t.Errorf("expected progress 50, got %f", snapshot.Progress.Percent)
	}

	// 5. Cancel task
	err = jm.CancelTask(taskId)
	if err != nil {
		t.Errorf("failed to cancel task: %v", err)
	}

	select {
	case <-taskCtx.Done():
		// Success: context was canceled
	case <-time.After(100 * time.Millisecond):
		t.Error("task context was not canceled")
	}

	snapshot, _ = jm.GetTask(taskId)
	if snapshot.State != TaskCanceled {
		t.Errorf("expected state canceled, got %s", snapshot.State)
	}

	// 6. Start a new task after previous one finished
	newId, _, err := jm.startTask("new.task", "Second task", nil)
	if err != nil {
		t.Fatalf("failed to start second task: %v", err)
	}

	// 7. Succeed task
	jm.completeTask(newId, "All done")
	snapshot, _ = jm.GetTask(newId)
	if snapshot.State != TaskSucceeded {
		t.Errorf("expected state succeeded, got %s", snapshot.State)
	}

	// 8. List tasks
	tasks := jm.ListTasks()
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in history, got %d", len(tasks))
	}
}

