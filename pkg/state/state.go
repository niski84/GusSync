package state

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// StateManager manages the markdown state file with thread-safe operations
type StateManager struct {
	mu                 sync.Mutex
	stateFile          string
	stateMap           map[string]string   // path -> hash (for completed files) - OLD FORMAT
	hashMap            map[string]string   // hash -> normalizedPath (for hash-based lookup) - NEW FORMAT
	failureMap         map[string]int      // path -> failure count
	deletedMap         map[string]string   // path -> hash (for deleted files)
	cleanupFailureMap  map[string]int      // path -> cleanup failure count
	dirMap             map[string]string   // directory path -> status (completed, timeout, error, partial)
	dirDiscoveredFiles map[string][]string // directory path -> list of discovered file paths
	hasSuccess         bool                // track if we've had any success in this run
	lastCompletedPath  string              // last file path that was completed (for resume)
	resumePointReached bool                // flag to track if we've passed the resume point
	fileHandle         *os.File
	writer             *bufio.Writer
}

// NewStateManager creates a new StateManager and loads existing state
func NewStateManager(stateFile string) (*StateManager, error) {
	sm := &StateManager{
		stateFile:          stateFile,
		stateMap:           make(map[string]string),
		hashMap:            make(map[string]string), // NEW: hash-based lookup
		failureMap:         make(map[string]int),
		deletedMap:         make(map[string]string),
		cleanupFailureMap:  make(map[string]int),
		dirMap:             make(map[string]string),
		dirDiscoveredFiles: make(map[string][]string),
		hasSuccess:         false,
	}

	// Load existing state if file exists
	if err := sm.loadState(); err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	sm.mu.Lock()
	dirsToClear := make([]string, 0)
	for dirPath, status := range sm.dirMap {
		if status == "completed" {
			// Check if this directory has discovered files tracking
			// dirDiscoveredFiles is initialized, but old directories won't have entries
			if _, hasTracking := sm.dirDiscoveredFiles[dirPath]; !hasTracking {
				// Old format - no discovered files tracking - clear status to force rescan
				dirsToClear = append(dirsToClear, dirPath)
			}
		}
	}
	// Clear old "completed" directory statuses (they'll be rescanned and re-marked if truly complete)
	for _, dirPath := range dirsToClear {
		delete(sm.dirMap, dirPath)
	}
	clearedCount := len(dirsToClear)
	sm.mu.Unlock()

	if clearedCount > 0 {
		fmt.Printf("Updating old state format: rescanning %d directories for completeness...\n", clearedCount)
		// Note: We don't write this to the file because we want to keep the old entries for reference
		// New entries will be written when directories are rescanned and completed properly
	}

	// Find last completed file path from state map (lexicographically last)
	if len(sm.stateMap) > 0 {
		fmt.Printf("Analyzing resume point...\n")
		var lastPath string
		for path := range sm.stateMap {
			if path > lastPath {
				lastPath = path
			}
		}
		sm.lastCompletedPath = lastPath
	}

	// Open file for appending (create if doesn't exist)
	var err error
	sm.fileHandle, err = os.OpenFile(stateFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}

	sm.writer = bufio.NewWriter(sm.fileHandle)

	return sm, nil
}

// loadState parses the markdown file and populates the state map
func (sm *StateManager) loadState() error {
	fmt.Printf("Loading backup state from %s...\n", filepath.Base(sm.stateFile))
	startTime := time.Now()

	file, err := os.Open(sm.stateFile)
	if os.IsNotExist(err) {
		return nil // File doesn't exist yet, that's okay
	}
	if err != nil {
		return err
	}
	defer file.Close()

	// Pattern for completed: - [x] /path/to/file | Hash: <hash>
	// Pattern for completed (new hash-based): - [x] Hash: <hash> | Path: <normalizedPath> | SourcePath: <sourcePath>
	// Pattern for failed: - [ ] /path/to/file | Failures: <count>
	// Pattern for deleted: - [d] /path/to/file | Hash: <hash> | Deleted: <timestamp>
	// Pattern for cleanup failures: - [c] /path/to/file | CleanupFailures: <count>
	// Pattern for directories: - [dir] /path/to/dir | Status: <status>
	completedPattern := regexp.MustCompile(`^\s*-\s+\[x\]\s+(.+?)(?:\s*\|\s*Hash:\s*(\S+))?\s*$`)
	completedHashPattern := regexp.MustCompile(`^\s*-\s+\[x\]\s+Hash:\s*(\S+)\s*\|\s*Path:\s*(.+?)(?:\s*\|\s*SourcePath:\s*(.+?))?\s*$`)
	failedPattern := regexp.MustCompile(`^\s*-\s+\[\s\]\s+(.+?)(?:\s*\|\s*Failures:\s*(\d+))?\s*$`)
	deletedPattern := regexp.MustCompile(`^\s*-\s+\[d\]\s+(.+?)(?:\s*\|\s*Hash:\s*(\S+))?\s*$`)
	cleanupFailurePattern := regexp.MustCompile(`^\s*-\s+\[c\]\s+(.+?)(?:\s*\|\s*CleanupFailures:\s*(\d+))?\s*$`)
	dirPattern := regexp.MustCompile(`^\s*-\s+\[dir\]\s+(.+?)(?:\s*\|\s*Status:\s*(\S+))?\s*$`)

	lineCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCount++
		if lineCount % 5000 == 0 {
			fmt.Printf("...processed %d lines of state\n", lineCount)
		}
		line := strings.TrimSpace(scanner.Text())

		// Check for completed files (new hash-based format first)
		if matches := completedHashPattern.FindStringSubmatch(line); matches != nil {
			hash := matches[1]
			normalizedPath := matches[2]
			sourcePath := matches[3]
			// Store in hash map (new format)
			sm.hashMap[hash] = normalizedPath
			// Also store in old format for backward compatibility
			if sourcePath != "" {
				sm.stateMap[sourcePath] = hash
			}
			continue
		}

		// Check for completed files (old path-based format)
		if matches := completedPattern.FindStringSubmatch(line); matches != nil {
			path := matches[1]
			hash := matches[2]
			sm.stateMap[path] = hash
			// Also add to hash map for hash-based lookup (backward compatibility)
			if hash != "" {
				sm.hashMap[hash] = "" // Empty normalized path means we need to compute it
			}
			continue
		}

		// Check for failed files
		if matches := failedPattern.FindStringSubmatch(line); matches != nil {
			path := matches[1]
			var count int
			if len(matches) > 2 && matches[2] != "" {
				fmt.Sscanf(matches[2], "%d", &count)
			} else {
				count = 1 // Default to 1 if not specified
			}
			sm.failureMap[path] = count
			continue
		}

		// Check for deleted files
		if matches := deletedPattern.FindStringSubmatch(line); matches != nil {
			path := matches[1]
			hash := matches[2]
			sm.deletedMap[path] = hash
			continue
		}

		// Check for cleanup failures
		if matches := cleanupFailurePattern.FindStringSubmatch(line); matches != nil {
			path := matches[1]
			var count int
			if len(matches) > 2 && matches[2] != "" {
				fmt.Sscanf(matches[2], "%d", &count)
			} else {
				count = 1 // Default to 1 if not specified
			}
			sm.cleanupFailureMap[path] = count
			continue
		}

		// Check for directory status
		if matches := dirPattern.FindStringSubmatch(line); matches != nil {
			path := matches[1]
			status := matches[2]
			if status == "" {
				status = "completed" // Default to completed if not specified
			}
			sm.dirMap[path] = status
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Finished loading state: %d lines processed in %v\n", lineCount, time.Since(startTime))
	return nil
}

// IsDone checks if a file path is already marked as done
// DEPRECATED: Use IsDoneForSource instead to filter by source path
func (sm *StateManager) IsDone(path string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, exists := sm.stateMap[path]
	return exists
}

// IsDoneForSource checks if a file path is marked as done AND matches the current source path prefix
// This allows rediscovery when switching between mount points (e.g., MTP to gphoto2)
// Files from old mounts won't block discovery of files on new mounts
func (sm *StateManager) IsDoneForSource(path, sourceRoot string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if path exists in state map
	_, exists := sm.stateMap[path]
	if !exists {
		return false
	}

	// If sourceRoot provided, verify path is from current source (not old mount)
	// This self-corrects by ignoring entries from different mount points
	if sourceRoot != "" {
		pathCleaned := filepath.Clean(path)
		sourceCleaned := filepath.Clean(sourceRoot)
		if !strings.HasPrefix(pathCleaned, sourceCleaned) {
			// Path in state file is from a different source - don't consider it done
			// This allows rediscovery when mount points change
			return false
		}
	}

	return true
}

// IsDoneByHash checks if a file hash is already marked as done (protocol-agnostic)
// This is the primary method for checking if a file is already copied
func (sm *StateManager) IsDoneByHash(hash string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, exists := sm.hashMap[hash]
	return exists
}

// GetNormalizedPathByHash returns the normalized destination path for a given hash
// Returns empty string if hash not found
func (sm *StateManager) GetNormalizedPathByHash(hash string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.hashMap[hash]
}

// ShouldSkipForResume - REMOVED: This was using lexicographic comparison which is incorrect
// Files are discovered in directory traversal order, not alphabetical order
// The IsDone check already efficiently skips completed files
// We should NOT skip files based on lexicographic path comparison
func (sm *StateManager) ShouldSkipForResume(path string) bool {
	// Always return false - let IsDone handle skipping completed files
	return false
}

// ShouldRetry checks if a file should be retried (hasn't failed 10 times yet)
func (sm *StateManager) ShouldRetry(path string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// If already done, don't retry
	if _, done := sm.stateMap[path]; done {
		return false
	}

	// If failed 10+ times, don't retry
	failures := sm.failureMap[path]
	return failures < 10
}

// RecordFailure records a failure for a file (only if we've had a success)
func (sm *StateManager) RecordFailure(path string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Only increment failure count if we've had at least one success
	if !sm.hasSuccess {
		// Don't record failure yet - just skip for now
		return nil
	}

	// Increment failure count (max one per run)
	sm.failureMap[path]++
	failures := sm.failureMap[path]

	// Update state file with failure count
	line := fmt.Sprintf("- [ ] %s | Failures: %d\n", path, failures)
	if _, err := sm.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write failure to state file: %w", err)
	}

	return nil
}

// MarkSuccess records that we've had at least one successful copy
func (sm *StateManager) MarkSuccess() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hasSuccess = true
}

// MarkDone marks a file as done and appends to the state file
// sourcePath: original source path (for backward compatibility)
// hash: file hash (SHA256)
// normalizedPath: protocol-agnostic normalized path (for new format)
func (sm *StateManager) MarkDone(sourcePath, hash, normalizedPath string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update in-memory maps
	sm.stateMap[sourcePath] = hash    // Old format (backward compatibility)
	sm.hashMap[hash] = normalizedPath // New format (hash-based)

	// Update last completed path if this file comes after it lexicographically
	if sourcePath > sm.lastCompletedPath {
		sm.lastCompletedPath = sourcePath
	}

	// Append to file using new hash-based format (more efficient and protocol-agnostic)
	// Format: - [x] Hash: <hash> | Path: <normalizedPath> | SourcePath: <sourcePath>
	line := fmt.Sprintf("- [x] Hash: %s | Path: %s | SourcePath: %s\n", hash, normalizedPath, sourcePath)
	if _, err := sm.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write to state file: %w", err)
	}

	// Flush periodically (but don't sync every time for performance)
	// We'll rely on the deferred flush in Close()
	return nil
}

// Flush forces a flush of the buffered writer
func (sm *StateManager) Flush() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.writer.Flush()
}

// Close closes the state file
func (sm *StateManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if err := sm.writer.Flush(); err != nil {
		return err
	}
	return sm.fileHandle.Close()
}

// GetStats returns the number of completed files
func (sm *StateManager) GetStats() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.stateMap)
}

// GetAllCompletedFiles returns a copy of all completed file paths and their hashes
func (sm *StateManager) GetAllCompletedFiles() map[string]string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make(map[string]string, len(sm.stateMap))
	for path, hash := range sm.stateMap {
		result[path] = hash
	}
	return result
}

// IsDeleted checks if a file path is already marked as deleted
func (sm *StateManager) IsDeleted(path string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, exists := sm.deletedMap[path]
	return exists
}

// MarkDeleted marks a file as deleted and appends to the state file
func (sm *StateManager) MarkDeleted(sourcePath, hash string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update in-memory map
	sm.deletedMap[sourcePath] = hash

	// Append to file with timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("- [d] %s | Hash: %s | Deleted: %s\n", sourcePath, hash, timestamp)
	if _, err := sm.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write deletion to state file: %w", err)
	}

	return nil
}

// IsDirScanned checks if a directory has been fully scanned (completed status)
// IMPORTANT: If a directory is marked as "completed" but we don't have discovered files
// tracking for it (backward compatibility), we return false to force a rescan.
// This ensures directories from old versions get rescanned to find missed files.
func (sm *StateManager) IsDirScanned(dirPath string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	status, exists := sm.dirMap[dirPath]
	if !exists || status != "completed" {
		return false
	}

	// Check if we have discovered files tracking for this directory
	// If not, this is an old "completed" directory - rescan it to find missed files
	discoveredFiles, hasTracking := sm.dirDiscoveredFiles[dirPath]
	if !hasTracking || len(discoveredFiles) == 0 {
		// Old format - no discovered files tracking for this directory - force rescan
		return false
	}
	for _, filePath := range discoveredFiles {
		if _, completed := sm.stateMap[filePath]; !completed {
			// At least one discovered file is not completed - don't skip
			return false
		}
	}

	// All discovered files are completed - safe to skip
	return true
}

// MarkDirStatus marks a directory with a status (completed, timeout, error)
func (sm *StateManager) MarkDirStatus(dirPath, status string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update in-memory map
	sm.dirMap[dirPath] = status

	// Append to file
	line := fmt.Sprintf("- [dir] %s | Status: %s\n", dirPath, status)
	if _, err := sm.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write directory status to state file: %w", err)
	}

	return nil
}

// GetDirStatus returns the status of a directory (empty if not tracked)
func (sm *StateManager) GetDirStatus(dirPath string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.dirMap[dirPath]
}

// AddDiscoveredFileToDir tracks a discovered file in a directory
func (sm *StateManager) AddDiscoveredFileToDir(dirPath, filePath string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.dirDiscoveredFiles == nil {
		sm.dirDiscoveredFiles = make(map[string][]string)
	}
	// Check if file is already in the list (avoid duplicates)
	existing := sm.dirDiscoveredFiles[dirPath]
	for _, existingFile := range existing {
		if existingFile == filePath {
			return // Already tracked
		}
	}
	sm.dirDiscoveredFiles[dirPath] = append(sm.dirDiscoveredFiles[dirPath], filePath)
}

// AreAllDiscoveredFilesCompleted checks if all discovered files in a directory were successfully copied
func (sm *StateManager) AreAllDiscoveredFilesCompleted(dirPath string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.dirDiscoveredFiles == nil {
		return false // No files discovered yet
	}

	discoveredFiles := sm.dirDiscoveredFiles[dirPath]
	if len(discoveredFiles) == 0 {
		return false // No files discovered in this directory
	}

	// Check if ALL discovered files are marked as completed
	for _, filePath := range discoveredFiles {
		if _, completed := sm.stateMap[filePath]; !completed {
			// At least one file is not completed
			return false
		}
	}

	return true // All discovered files are completed
}

// ShouldRetryCleanup checks if a cleanup operation should be retried (hasn't failed 10 times yet)
func (sm *StateManager) ShouldRetryCleanup(path string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	failures := sm.cleanupFailureMap[path]
	return failures < 10
}

// RecordCleanupFailure records a cleanup failure for a file (once per run)
func (sm *StateManager) RecordCleanupFailure(path string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Increment failure count (max one per run)
	sm.cleanupFailureMap[path]++
	failures := sm.cleanupFailureMap[path]

	// Update state file with cleanup failure count
	line := fmt.Sprintf("- [c] %s | CleanupFailures: %d\n", path, failures)
	if _, err := sm.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write cleanup failure to state file: %w", err)
	}

	return nil
}

// DirSummary contains summary of directory statuses
type DirSummary struct {
	Completed int
	Timeout   int
	Error     int
}

// GetDirSummary returns a summary of directory statuses
func (sm *StateManager) GetDirSummary() DirSummary {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	var summary DirSummary
	for _, status := range sm.dirMap {
		switch status {
		case "completed":
			summary.Completed++
		case "timeout":
			summary.Timeout++
		case "error":
			summary.Error++
		}
	}
	return summary
}
