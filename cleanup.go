package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// runCleanupMode handles the cleanup mode: deletes source files that are verified in the destination
func runCleanupMode(sourceRoot, destRoot string) error {
	// Build state file path from destination (same location as backup mode)
	// For cleanup, we need to determine which mode was used (mount or adb)
	// Try mount first, then adb
	var stateFile string
	var stateManager *StateManager
	var err error

	// Try mount mode state file
	mountDestPath := filepath.Join(destRoot, "mount")
	mountStateFile := filepath.Join(mountDestPath, stateFileName)
	if _, statErr := os.Stat(mountStateFile); statErr == nil {
		stateFile = mountStateFile
		stateManager, err = NewStateManager(stateFile)
		if err != nil {
			return fmt.Errorf("failed to open mount state file: %w", err)
		}
		defer stateManager.Close()
	} else {
		// Try adb mode state file
		adbDestPath := filepath.Join(destRoot, "adb")
		adbStateFile := filepath.Join(adbDestPath, stateFileName)
		if _, statErr := os.Stat(adbStateFile); statErr == nil {
			stateFile = adbStateFile
			stateManager, err = NewStateManager(adbStateFile)
			if err != nil {
				return fmt.Errorf("failed to open adb state file: %w", err)
			}
			defer stateManager.Close()
		} else {
			return fmt.Errorf("no state file found in %s/mount or %s/adb", destRoot, destRoot)
		}
	}

	// Determine error log file path (same directory as state file)
	errorLogFile := filepath.Join(filepath.Dir(stateFile), "gus_errors.log")
	errorLog, err := os.OpenFile(errorLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create error log file: %w", err)
	}
	defer errorLog.Close()

	// Helper function to log errors
	logError := func(format string, args ...interface{}) {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf(format, args...)
		logLine := fmt.Sprintf("[%s] cleanup: %s\n", timestamp, msg)
		errorLog.WriteString(logLine)
		errorLog.Sync() // Flush immediately for tail -f
	}

	fmt.Printf("GusSync - Cleanup Mode\n")
	fmt.Printf("Source: %s\n", sourceRoot)
	fmt.Printf("State file: %s\n", stateFile)
	fmt.Printf("Error log: %s\n\n", errorLogFile)

	// Get all completed files with hashes
	completedFiles := stateManager.GetAllCompletedFiles()
	totalFiles := len(completedFiles)

	if totalFiles == 0 {
		fmt.Println("No completed files found in state file. Nothing to clean up.")
		return nil
	}

	fmt.Printf("Found %d completed files in state file.\n", totalFiles)
	fmt.Printf("Verifying hashes and deleting verified source files (shuffle-bag pattern)...\n\n")

	// Convert map to slice for shuffle-bag processing
	type fileEntry struct {
		path string
		hash string
	}
	filesToProcess := make([]fileEntry, 0, totalFiles)
	for path, hash := range completedFiles {
		// Only include files that aren't already deleted and have retries left
		if !stateManager.IsDeleted(path) && stateManager.ShouldRetryCleanup(path) {
			filesToProcess = append(filesToProcess, fileEntry{path: path, hash: hash})
		}
	}

	// Shuffle-bag pattern: shuffle once, process sequentially, reshuffle when done
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(filesToProcess), func(i, j int) {
		filesToProcess[i], filesToProcess[j] = filesToProcess[j], filesToProcess[i]
	})

	fmt.Printf("Processing %d files (shuffled once, processing sequentially)...\n\n", len(filesToProcess))

	// Progress tracking with mutex for thread-safe updates
	type progressStatus struct {
		sync.Mutex
		currentFile      string
		deletedCount     int
		failedCount      int
		skippedCount     int
		ioErrorCount     int
		processedCount   int
		totalFiles       int
	}
	status := &progressStatus{
		totalFiles: len(filesToProcess),
	}
	
	var verifiedCount int
	var alreadyDeletedCount int
	var connectionDead bool
	
	// Track files that failed in this run (to avoid multiple failures per run)
	failedThisRun := make(map[string]bool)

	// Count already deleted files (separate from processing loop)
	for path := range completedFiles {
		if stateManager.IsDeleted(path) {
			alreadyDeletedCount++
		} else if !stateManager.ShouldRetryCleanup(path) {
			status.skippedCount++
		}
	}

	// Start progress reporting goroutine
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				status.Lock()
				current := status.currentFile
				deleted := status.deletedCount
				failed := status.failedCount
				skipped := status.skippedCount
				ioErrs := status.ioErrorCount
				processed := status.processedCount
				total := status.totalFiles
				status.Unlock()

				// Truncate current file path for display
				displayFile := current
				if len(displayFile) > 60 {
					displayFile = "..." + displayFile[len(displayFile)-57:]
				}

				percent := 0.0
				if total > 0 {
					percent = float64(processed) / float64(total) * 100.0
				}

				// Print status line (using \r to overwrite)
				fmt.Printf("\r[Cleanup] %d/%d (%.1f%%) | Deleted: %d | Failed: %d | Skipped: %d | I/O Errors: %d | Current: %s", 
					processed, total, percent, deleted, failed, skipped, ioErrs, displayFile)
			case <-progressDone:
				return
			}
		}
	}()

	for _, file := range filesToProcess {
		sourcePath := file.path
		expectedHash := file.hash

		// Update current file being processed
		status.Lock()
		status.currentFile = sourcePath
		status.processedCount++
		status.Unlock()

		// Double-check deletion status and retry eligibility (in case state changed)
		if stateManager.IsDeleted(sourcePath) {
			alreadyDeletedCount++
			continue
		}
		if !stateManager.ShouldRetryCleanup(sourcePath) {
			status.Lock()
			status.skippedCount++
			status.Unlock()
			continue
		}

		// Check if source file exists
		sourceStat, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist - might already be deleted but not recorded
				status.Lock()
				status.skippedCount++
				status.Unlock()
				continue
			}
			// Other error accessing source file - log and skip
			logError("failed to stat source file %s: %v", sourcePath, err)
			errStr := err.Error()
			if strings.Contains(errStr, "input/output error") ||
			   strings.Contains(errStr, "No such device") ||
			   strings.Contains(errStr, "Transport endpoint is not connected") ||
			   strings.Contains(errStr, "Stale file handle") {
				// Connection issue - don't count as failure
				status.Lock()
				status.ioErrorCount++
				status.Unlock()
			} else {
				// Other error - record as cleanup failure
				if !failedThisRun[sourcePath] {
					stateManager.RecordCleanupFailure(sourcePath)
					failedThisRun[sourcePath] = true
				}
				status.Lock()
				status.failedCount++
				status.Unlock()
			}
			continue
		}
		// Check if it's actually a file (not a directory)
		if sourceStat.IsDir() {
			// Skip directories
			status.Lock()
			status.skippedCount++
			status.Unlock()
			continue
		}

		// Check connection health - if source root is not accessible, connection is dead
		if _, err := os.Stat(sourceRoot); err != nil {
			errStr := err.Error()
			if os.IsNotExist(err) || 
			   strings.Contains(errStr, "input/output error") ||
			   strings.Contains(errStr, "No such device") ||
			   strings.Contains(errStr, "Transport endpoint is not connected") ||
			   strings.Contains(errStr, "Stale file handle") {
				fmt.Fprintf(os.Stderr, "\n🔴 CRITICAL: Connection dropped - source path no longer accessible: %s\n", sourceRoot)
				fmt.Fprintf(os.Stderr, "Cleanup interrupted. Progress saved. Reconnect and run again to resume.\n\n")
				connectionDead = true
				break
			}
		}

		// Calculate relative path to find destination file
		relPath, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil {
			// Silent skip for path calculation errors
			status.Lock()
			status.skippedCount++
			status.Unlock()
			continue
		}

		// Determine destination path based on which state file we're using
		var destPath string
		if filepath.Base(filepath.Dir(stateFile)) == "mount" {
			destPath = filepath.Join(destRoot, "mount", relPath)
		} else {
			destPath = filepath.Join(destRoot, "adb", relPath)
		}

		// Check if destination file exists
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Destination missing - copy from source first
			logError("destination file missing for %s: %s - copying from source", sourcePath, destPath)
			fmt.Printf("\n⚠️  Destination missing, copying from source: %s\n", filepath.Base(sourcePath))
			
			// Copy file from source to destination
			// Determine destination root (without mode subdirectory)
			var destRootForCopy string
			if filepath.Base(filepath.Dir(stateFile)) == "mount" {
				destRootForCopy = filepath.Join(destRoot, "mount")
			} else {
				destRootForCopy = filepath.Join(destRoot, "adb")
			}
			
			copyResult := RobustCopy(sourcePath, sourceRoot, destRootForCopy, nil)
			if copyResult.Error != nil || !copyResult.Success {
				logError("failed to copy file from source %s to destination %s: %v", sourcePath, destPath, copyResult.Error)
				// Record as cleanup failure
				if !failedThisRun[sourcePath] {
					stateManager.RecordCleanupFailure(sourcePath)
					failedThisRun[sourcePath] = true
				}
				status.Lock()
				status.failedCount++
				status.Unlock()
				continue
			}
			
			// Verify copied file hash matches expected hash
			if copyResult.SourceHash != expectedHash {
				logError("hash mismatch after copy: expected %s, got %s for %s", expectedHash, copyResult.SourceHash, sourcePath)
				// Hash mismatch - record as cleanup failure
				if !failedThisRun[sourcePath] {
					stateManager.RecordCleanupFailure(sourcePath)
					failedThisRun[sourcePath] = true
				}
				status.Lock()
				status.failedCount++
				status.Unlock()
				continue
			}
			
			// Copy successful and hash matches - file is now restored, continue with cleanup
			fmt.Printf("✓ File copied successfully: %s\n", filepath.Base(sourcePath))
			// Continue to hash verification below (destination now exists)
		}

		// Verify destination hash matches expected hash
		destHash, err := calculateFileHash(destPath)
		if err != nil {
			// Can't hash destination - log and record failure
			logError("failed to hash destination file %s (source: %s): %v", destPath, sourcePath, err)
			if !failedThisRun[sourcePath] {
				stateManager.RecordCleanupFailure(sourcePath)
				failedThisRun[sourcePath] = true
			}
			status.Lock()
			status.failedCount++
			status.Unlock()
			continue
		}

		// Also verify source hash matches (double-check)
		sourceHash, err := calculateFileHash(sourcePath)
		if err != nil {
			// Check if this is an I/O error (connection issue)
			errStr := err.Error()
			if strings.Contains(errStr, "input/output error") ||
			   strings.Contains(errStr, "No such device") ||
			   strings.Contains(errStr, "Transport endpoint is not connected") ||
			   strings.Contains(errStr, "Stale file handle") {
				// Connection issue - log and skip this file, don't count as failure
				logError("I/O error hashing source file %s: %v", sourcePath, err)
				status.Lock()
				status.ioErrorCount++
				ioErrCount := status.ioErrorCount
				status.Unlock()
				if ioErrCount == 1 {
					fmt.Fprintf(os.Stderr, "\n⚠️  I/O errors detected (connection may be unstable). Skipping affected files...\n")
				}
				// Check if connection is completely dead
				if _, statErr := os.Stat(sourceRoot); statErr != nil {
					statErrStr := statErr.Error()
					if strings.Contains(statErrStr, "input/output error") ||
					   strings.Contains(statErrStr, "No such device") ||
					   strings.Contains(statErrStr, "Transport endpoint is not connected") {
						logError("CRITICAL: Connection dropped during cleanup (source root: %s)", sourceRoot)
						fmt.Fprintf(os.Stderr, "\n🔴 CRITICAL: Connection dropped during cleanup\n")
						connectionDead = true
						break
					}
				}
				continue
			}
			// Other errors - log and record as cleanup failure
			logError("failed to hash source file %s: %v", sourcePath, err)
			if !failedThisRun[sourcePath] {
				stateManager.RecordCleanupFailure(sourcePath)
				failedThisRun[sourcePath] = true
			}
			status.Lock()
			status.failedCount++
			status.Unlock()
			continue
		}

		// Verify both hashes match expected hash
		if sourceHash == expectedHash && destHash == expectedHash && sourceHash == destHash {
			// All hashes match - safe to delete source
			if err := os.Remove(sourcePath); err != nil {
				logError("failed to delete source file %s: %v", sourcePath, err)
				fmt.Fprintf(os.Stderr, "\n⚠️  Failed to delete %s: %v\n", sourcePath, err)
				// Record as cleanup failure
				if !failedThisRun[sourcePath] {
					stateManager.RecordCleanupFailure(sourcePath)
					failedThisRun[sourcePath] = true
				}
				status.Lock()
				status.failedCount++
				status.Unlock()
				continue
			}
			
			// Record deletion in state file
			if err := stateManager.MarkDeleted(sourcePath, expectedHash); err != nil {
				logError("failed to record deletion for %s: %v", sourcePath, err)
				fmt.Fprintf(os.Stderr, "\n⚠️  Failed to record deletion for %s: %v\n", sourcePath, err)
				// Continue anyway - file is deleted, just not recorded
			}
			
			status.Lock()
			status.deletedCount++
			status.Unlock()
			verifiedCount++
		} else {
			// Hash mismatch - log and record failure but don't delete
			logError("hash mismatch for %s (expected: %s, source: %s, dest: %s)", sourcePath, expectedHash, sourceHash, destHash)
			if !failedThisRun[sourcePath] {
				stateManager.RecordCleanupFailure(sourcePath)
				failedThisRun[sourcePath] = true
			}
			status.Lock()
			status.failedCount++
			status.Unlock()
		}
	}

	// Stop progress ticker and print final status line
	close(progressDone)
	time.Sleep(100 * time.Millisecond) // Allow final update
	fmt.Printf("\n") // New line after progress updates

	// Get final counts
	status.Lock()
	finalDeleted := status.deletedCount
	finalFailed := status.failedCount
	finalSkipped := status.skippedCount
	finalIOErrs := status.ioErrorCount
	status.Unlock()

	// If connection died, exit early
	if connectionDead {
		fmt.Printf("Cleanup interrupted due to connection loss.\n")
		fmt.Printf("  Files verified and deleted: %d\n", finalDeleted)
		fmt.Printf("  Files already deleted (from previous run): %d\n", alreadyDeletedCount)
		fmt.Printf("  Files with I/O errors (connection issues): %d\n", finalIOErrs)
		fmt.Printf("  Files with hash mismatch: %d\n", finalFailed)
		fmt.Printf("  Files skipped (missing): %d\n", finalSkipped)
		fmt.Printf("  Reconnect and run again to resume.\n")
		return fmt.Errorf("cleanup interrupted: connection lost")
	}

	// Flush state file to ensure all deletions are recorded
	if err := stateManager.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to flush state file: %v\n", err)
	}

	fmt.Printf("Cleanup complete:\n")
	fmt.Printf("  Files verified and deleted: %d\n", finalDeleted)
	fmt.Printf("  Files already deleted (from previous run): %d\n", alreadyDeletedCount)
	if finalIOErrs > 0 {
		fmt.Printf("  Files with I/O errors (connection issues): %d\n", finalIOErrs)
	}
	fmt.Printf("  Files with hash mismatch: %d\n", finalFailed)
	fmt.Printf("  Files skipped (missing): %d\n", finalSkipped)
	fmt.Printf("  Total processed: %d\n", len(filesToProcess))

	if finalFailed > 0 {
		return fmt.Errorf("cleanup completed with %d failures", finalFailed)
	}

	return nil
}

