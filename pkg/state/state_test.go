package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateManager(t *testing.T) {
	// Create a temporary directory for the state file
	tmpDir, err := os.MkdirTemp("", "gussync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "gus_state.md")

	// 1. Test creation and MarkDone
	sm, err := NewStateManager(stateFile)
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}

	testPath := "/sdcard/DCIM/Camera/test.jpg"
	testHash := "abc123hash"
	testNormalized := "DCIM/Camera/test.jpg"

	if err := sm.MarkDone(testPath, testHash, testNormalized); err != nil {
		t.Errorf("MarkDone failed: %v", err)
	}

	if sm.GetStats() != 1 {
		t.Errorf("expected stats 1, got %d", sm.GetStats())
	}

	if !sm.IsDoneByHash(testHash) {
		t.Errorf("expected hash to be done")
	}

	sm.Close()

	// 2. Test loading existing state
	sm2, err := NewStateManager(stateFile)
	if err != nil {
		t.Fatalf("failed to reload state manager: %v", err)
	}
	defer sm2.Close()

	if sm2.GetStats() != 1 {
		t.Errorf("reloaded: expected stats 1, got %d", sm2.GetStats())
	}

	if !sm2.IsDoneByHash(testHash) {
		t.Errorf("reloaded: expected hash to be done")
	}

	if sm2.GetNormalizedPathByHash(testHash) != testNormalized {
		t.Errorf("reloaded: expected normalized path %s, got %s", testNormalized, sm2.GetNormalizedPathByHash(testHash))
	}

	// 3. Test failure tracking
	failPath := "/sdcard/broken.file"
	sm2.MarkSuccess() // Must have a success before failures are recorded
	if err := sm2.RecordFailure(failPath); err != nil {
		t.Errorf("RecordFailure failed: %v", err)
	}

	if sm2.ShouldRetry(failPath) != true {
		t.Errorf("expected ShouldRetry to be true for 1 failure")
	}

	for i := 0; i < 10; i++ {
		sm2.RecordFailure(failPath)
	}

	if sm2.ShouldRetry(failPath) != false {
		t.Errorf("expected ShouldRetry to be false after 11 failures")
	}

	// 4. Test directory status
	dirPath := "/sdcard/DCIM"
	sm2.MarkDirStatus(dirPath, "completed")
	if sm2.GetDirStatus(dirPath) != "completed" {
		t.Errorf("expected dir status completed, got %s", sm2.GetDirStatus(dirPath))
	}
}

