package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// StallTimeout is the duration to wait for bytes before considering a transfer stalled
	StallTimeout = 30 * time.Second
	// BufferSize for copying
	BufferSize = 64 * 1024 // 64KB
	// ProgressUpdateInterval is how often to report progress
	ProgressUpdateInterval = 2 * time.Second
)

// progressTracker tracks copy progress for stall detection and reporting
type progressTracker struct {
	lastTime     time.Time
	lastBytes    int64 // Bytes at last check (for stall detection)
	bytes        int64
	progressChan chan<- int64 // Channel to report progress (optional)
	sync.Mutex
}

// CopyResult represents the result of a copy operation
type CopyResult struct {
	Success     bool
	SourceHash  string
	DestHash    string
	BytesCopied int64
	Error       error
}

// RobustCopy copies a file with stall detection and hash verification
// sourcePath: full path to source file
// sourceRoot: root directory of source (to calculate relative path)
// destRoot: root directory of destination
// progressChan: optional channel to receive progress updates (bytes copied)
func RobustCopy(sourcePath, sourceRoot, destRoot string, progressChan chan<- int64) *CopyResult {
	result := &CopyResult{}

	// Calculate relative path from source root
	relPath, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to calculate relative path: %w", err)
		return result
	}

	// Build destination path preserving directory structure
	destPath := filepath.Join(destRoot, relPath)

	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create dest dir: %w", err)
		return result
	}

	// Open source file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to open source: %w", err)
		return result
	}
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to create dest: %w", err)
		return result
	}
	defer destFile.Close()

	// No connection checker for RobustCopy (legacy function, used internally)
	result.BytesCopied, result.Error = copyWithTimeout(sourceFile, destFile, StallTimeout, progressChan, nil)
	if result.Error != nil {
		return result
	}

	// Sync destination to ensure data is written
	if err := destFile.Sync(); err != nil {
		result.Error = fmt.Errorf("failed to sync dest: %w", err)
		return result
	}

	// Calculate hashes for both files
	result.SourceHash, err = calculateFileHash(sourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to hash source: %w", err)
		return result
	}

	result.DestHash, err = calculateFileHash(destPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to hash dest: %w", err)
		return result
	}

	// Verify hashes match
	if result.SourceHash != result.DestHash {
		result.Error = fmt.Errorf("hash mismatch: source=%s, dest=%s", result.SourceHash, result.DestHash)
		return result
	}

	result.Success = true
	return result
}

// ConnectionChecker is a function that checks if the connection is still alive
// Returns error if connection is dead, nil if connection is alive
type ConnectionChecker func() error

// copyWithTimeout copies data with stall detection and progress reporting
func copyWithTimeout(src io.Reader, dst io.Writer, timeout time.Duration, progressChan chan<- int64, connChecker ConnectionChecker) (int64, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track progress atomically
	prog := &progressTracker{
		lastTime:     time.Now(),
		lastBytes:    0,
		progressChan: progressChan,
	}

	// Progress checker and reporter goroutine
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		progressTicker := time.NewTicker(ProgressUpdateInterval)
		connTicker := time.NewTicker(10 * time.Second) // Check connection every 10 seconds
		defer ticker.Stop()
		defer progressTicker.Stop()
		defer connTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-connTicker.C:
				// Check connection health if checker provided
				if connChecker != nil {
					if err := connChecker(); err != nil {
						// Connection is dead - cancel copy
						cancel()
						return
					}
				}
			case <-ticker.C:
				// Check for stalls: if bytes haven't increased in timeout period
				prog.Lock()
				currentBytes := prog.bytes
				timeSinceLastUpdate := time.Since(prog.lastTime)

				// If bytes haven't changed AND enough time has passed, consider it stalled
				if currentBytes == prog.lastBytes && timeSinceLastUpdate > timeout {
					prog.Unlock()
					cancel()
					return
				}

				// Update lastBytes only if bytes have increased (progress made)
				if currentBytes > prog.lastBytes {
					prog.lastBytes = currentBytes
					prog.lastTime = time.Now()
				}
				prog.Unlock()
			case <-progressTicker.C:
				// Report progress
				prog.Lock()
				currentBytes := prog.bytes
				prog.Unlock()
				if progressChan != nil {
					select {
					case progressChan <- currentBytes:
					default:
						// Channel full, skip this update
					}
				}
			}
		}
	}()

	// Wrap source reader to track progress
	progressReader := &progressReader{
		Reader:   src,
		progress: prog,
	}

	// Perform the copy
	var totalBytes int64
	var err error
	totalBytes, err = io.Copy(dst, progressReader)

	done <- true

	// Send final progress update
	if progressChan != nil {
		select {
		case progressChan <- totalBytes:
		default:
		}
	}

	if err != nil {
		return totalBytes, err
	}

	// Check if we were cancelled due to stall or connection loss
	select {
	case <-ctx.Done():
		prog.Lock()
		elapsed := time.Since(prog.lastTime)
		prog.Unlock()
		// Check if it was a connection issue or stall
		if connChecker != nil {
			if err := connChecker(); err != nil {
				return totalBytes, fmt.Errorf("connection lost during copy: %w", err)
			}
		}
		return totalBytes, fmt.Errorf("copy stalled: no progress for %v", elapsed)
	default:
		return totalBytes, nil
	}
}

// progressReader wraps a Reader to track progress
type progressReader struct {
	io.Reader
	progress *progressTracker
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	if n > 0 {
		pr.progress.Lock()
		pr.progress.bytes += int64(n)
		pr.progress.lastTime = time.Now()
		// Update lastBytes when we read data (progress is being made)
		pr.progress.lastBytes = pr.progress.bytes
		pr.progress.Unlock()
	}
	return n, err
}

// calculateFileHash computes SHA256 hash of a file
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
