package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DirReadTimeout is the timeout for reading a single directory (important for MTP)
	DirReadTimeout = 60 * time.Second
)

// getPathPriority returns a priority score for a path (lower = higher priority)
// Priority paths (DCIM, Camera, Pictures, etc.) get lower scores
func getPathPriority(relPath string, rootPath string) int {
	// Calculate relative path from root
	rel, err := filepath.Rel(rootPath, relPath)
	if err != nil {
		return 999 // Low priority for errors
	}
	
	// Get the first directory component
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "." {
		return 999
	}
	
	firstDir := parts[0]
	
	// Check if this is a priority path
	for i, priorityPath := range PriorityPaths {
		// Check exact match or if path starts with priority path
		if firstDir == priorityPath || strings.HasPrefix(rel, priorityPath) {
			return i // Lower number = higher priority
		}
	}
	
	// Default priority for non-priority paths
	return 100
}

// FSScanner implements Scanner for filesystem-based scanning
type FSScanner struct {
	closeJobChan func() // Function to safely close jobChan (uses sync.Once)
}

// NewFSScanner creates a new filesystem scanner
func NewFSScanner(closeJobChan func()) *FSScanner {
	return &FSScanner{
		closeJobChan: closeJobChan,
	}
}

// Scan discovers files using filesystem traversal
func (fs *FSScanner) Scan(ctx context.Context, root string, jobs chan<- FileJob, errors chan<- error) {
	defer func() {
		// Use the safe close function (sync.Once ensures it's only closed once)
		fs.closeJobChan()
	}()

	// Connection health check: periodically verify the root is still accessible
	healthTicker := time.NewTicker(30 * time.Second)
	healthDone := make(chan bool)
	defer healthTicker.Stop()
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-healthDone:
				return
			case <-healthTicker.C:
				// Check if root is still accessible (connection alive check)
				_, err := os.Stat(root)
				if err != nil {
					if os.IsNotExist(err) {
						errors <- fmt.Errorf("CRITICAL: Source path no longer exists - connection may have dropped: %s", root)
						return
					}
					// Check for connection errors - these indicate the device disconnected
					errStr := err.Error()
					if strings.Contains(errStr, "input/output error") || 
					   strings.Contains(errStr, "No such device") ||
					   strings.Contains(errStr, "Transport endpoint is not connected") ||
					   (strings.Contains(errStr, "No such file or directory") && strings.Contains(root, "gvfs")) ||
					   strings.Contains(errStr, "Stale file handle") {
						errors <- fmt.Errorf("CRITICAL: Connection dropped - source path no longer accessible: %s: %v", root, err)
						return
					}
					// Other errors (permissions, etc.) are logged but don't kill the process
					errors <- fmt.Errorf("WARNING: Source path stat check failed (non-fatal): %s: %v", root, err)
				}
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	fs.scanDir(ctx, root, root, jobs, errors, &wg)
	wg.Wait() // Wait for all subdirectories to finish
	
	// Stop health checker when scan completes
	close(healthDone)
}

// scanDir recursively scans a directory with timeout protection
func (fs *FSScanner) scanDir(ctx context.Context, root, current string, jobs chan<- FileJob, errors chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	// Create a context with timeout for this directory read
	dirCtx, cancel := context.WithTimeout(ctx, DirReadTimeout)
	defer cancel()

	// Channel to receive directory entries
	entriesChan := make(chan dirEntryResult, 100)

	// Read directory in a goroutine with timeout
	go func() {
		defer close(entriesChan)
		entries, err := os.ReadDir(current)
		if err != nil {
			entriesChan <- dirEntryResult{err: err}
			return
		}

		// Sort entries: directories first, then by priority
		// Always prioritize common paths (DCIM, Camera, etc.)
		sort.Slice(entries, func(i, j int) bool {
			// Directories come first
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			// Then sort by priority (for both root and subdirectories)
			if entries[i].IsDir() && entries[j].IsDir() {
				pathI := filepath.Join(current, entries[i].Name())
				pathJ := filepath.Join(current, entries[j].Name())
				priI := getPathPriority(pathI, root)
				priJ := getPathPriority(pathJ, root)
				return priI < priJ
			}
			return entries[i].Name() < entries[j].Name()
		})

		for _, entry := range entries {
			select {
			case <-dirCtx.Done():
				entriesChan <- dirEntryResult{err: fmt.Errorf("directory read timeout: %s", current)}
				return
			case entriesChan <- dirEntryResult{entry: entry}:
			}
		}
	}()

	// Process entries with timeout
	for {
		select {
		case <-dirCtx.Done():
			errors <- fmt.Errorf("directory read timeout: %s", current)
			return
		case result, ok := <-entriesChan:
			if !ok {
				// Channel closed, directory read complete
				return
			}

			if result.err != nil {
				// Report error but continue
				errors <- fmt.Errorf("error reading %s: %w", current, result.err)
				return
			}

			entry := result.entry
			path := filepath.Join(current, entry.Name())

			if entry.IsDir() {
				// For priority paths, process sequentially (to ensure they're discovered first)
				// For other paths, process concurrently
				pri := getPathPriority(path, root)
				if pri < 100 {
					// Priority path - process immediately (sequentially)
					wg.Add(1)
					fs.scanDir(ctx, root, path, jobs, errors, wg)
				} else {
					// Non-priority path - process concurrently
					wg.Add(1)
					go fs.scanDir(ctx, root, path, jobs, errors, wg)
				}
			} else {
				// Calculate relative path
				relPath, err := filepath.Rel(root, path)
				if err != nil {
					errors <- fmt.Errorf("failed to calculate relative path for %s: %w", path, err)
					continue
				}

				// Stream file job immediately
				select {
				case jobs <- FileJob{SourcePath: path, RelPath: relPath}:
				case <-dirCtx.Done():
					return
				case <-ctx.Done():
					// Context cancelled (shutdown requested)
					return
				}
			}
		}
	}
}

// dirEntryResult wraps a directory entry or error
type dirEntryResult struct {
	entry fs.DirEntry
	err   error
}

// FSCopier implements Copier for filesystem-based copying
type FSCopier struct{}

// NewFSCopier creates a new filesystem copier
func NewFSCopier() *FSCopier {
	return &FSCopier{}
}

// Copy copies a file using filesystem operations with stall detection
func (fc *FSCopier) Copy(ctx context.Context, sourcePath, sourceRoot, destRoot string, progressChan chan<- int64) (int64, error) {
	// Calculate relative path from source root
	relPath, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate relative path: %w", err)
	}

	// Build destination path preserving directory structure
	destPath := filepath.Join(destRoot, relPath)

	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create dest dir: %w", err)
	}

	// Open source file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open source: %w", err)
	}
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create dest: %w", err)
	}
	defer destFile.Close()

	// Create connection checker for mount mode: verify source root is still accessible
	var connChecker ConnectionChecker
	if sourceRoot != "" {
		connChecker = func() error {
			_, err := os.Stat(sourceRoot)
			if err != nil {
				// Check for connection errors
				errStr := err.Error()
				if os.IsNotExist(err) || 
				   strings.Contains(errStr, "input/output error") ||
				   strings.Contains(errStr, "No such device") ||
				   strings.Contains(errStr, "Transport endpoint is not connected") ||
				   (strings.Contains(errStr, "No such file or directory") && strings.Contains(sourceRoot, "gvfs")) ||
				   strings.Contains(errStr, "Stale file handle") {
					return fmt.Errorf("connection lost: %w", err)
				}
			}
			return nil
		}
	}
	
	// Copy with timeout/stall detection, progress reporting, and connection checking
	bytesCopied, err := copyWithTimeout(sourceFile, destFile, StallTimeout, progressChan, connChecker)
	if err != nil {
		return bytesCopied, err
	}

	// Sync destination to ensure data is written
	if err := destFile.Sync(); err != nil {
		return bytesCopied, fmt.Errorf("failed to sync dest: %w", err)
	}

	return bytesCopied, nil
}

