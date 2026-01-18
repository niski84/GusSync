package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// ADBCommandTimeout is the timeout for ADB commands
	ADBCommandTimeout = 5 * time.Minute
	// ADBPullTimeout is the timeout for adb pull operations
	ADBPullTimeout = 30 * time.Minute
)

// ADBScanner implements Scanner for ADB-based scanning
type ADBScanner struct {
	closeJobChan func() // Function to safely close jobChan (uses sync.Once)
}

// NewADBScanner creates a new ADB scanner
func NewADBScanner(closeJobChan func()) *ADBScanner {
	return &ADBScanner{
		closeJobChan: closeJobChan,
	}
}

// Scan discovers files using adb shell find with priority paths first
func (adb *ADBScanner) Scan(ctx context.Context, root string, jobs chan<- FileJob, errors chan<- error) {
	defer func() {
		// Use the safe close function (sync.Once ensures it's only closed once)
		adb.closeJobChan()
	}()

	// Sanitize root path for Android
	// Convert local paths like /mnt/phone to /sdcard if needed
	androidRoot := sanitizeAndroidPath(root)

	// Track files we've already sent (to avoid duplicates)
	sentFiles := make(map[string]bool)
	var mu sync.Mutex

	// Helper function to find and send files from a path
	findAndSend := func(searchPath string) {
		cmd := exec.CommandContext(ctx, "adb", "shell", "find", searchPath, "-type", "f", "2>/dev/null")
		
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errors <- fmt.Errorf("failed to create stdout pipe for adb find %s: %w", searchPath, err)
			return
		}

		if err := cmd.Start(); err != nil {
			errors <- fmt.Errorf("failed to start adb find %s: %w", searchPath, err)
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				cmd.Process.Kill()
				return
			default:
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				androidPath := line
				
				// Check if we've already sent this file
				mu.Lock()
				if sentFiles[androidPath] {
					mu.Unlock()
					continue
				}
				sentFiles[androidPath] = true
				mu.Unlock()

				// Calculate relative path from root
				relPath, err := calculateRelPathFromAndroid(androidPath, androidRoot)
				if err != nil {
					errors <- fmt.Errorf("failed to calculate relative path for %s: %w", androidPath, err)
					continue
				}

				// Send job immediately (priority paths are processed first)
				select {
				case jobs <- FileJob{SourcePath: androidPath, RelPath: relPath}:
				case <-ctx.Done():
					cmd.Process.Kill()
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errors <- fmt.Errorf("error reading adb find output for %s: %w", searchPath, err)
		}

		cmd.Wait() // Ignore errors for missing directories
	}

	// First, process priority paths in order
	var wg sync.WaitGroup
	for _, priorityPath := range PriorityPaths {
		select {
		case <-ctx.Done():
			return
		default:
			// Build full Android path for priority directory
			priorityFullPath := androidRoot + "/" + priorityPath
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				findAndSend(path)
			}(priorityFullPath)
		}
	}
	
	// Wait for all priority paths to complete
	wg.Wait()

	// Then, find all remaining files (excluding already sent ones)
	// We'll use find with -path exclusion, but that's complex, so instead
	// just run a general find and skip already-sent files
	cmd := exec.CommandContext(ctx, "adb", "shell", "find", androidRoot, "-type", "f", "2>/dev/null")
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errors <- fmt.Errorf("failed to create stdout pipe for general adb find: %w", err)
		return
	}

	if err := cmd.Start(); err != nil {
		errors <- fmt.Errorf("failed to start general adb find: %w", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			return
		default:
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			androidPath := line
			
			// Skip if already sent (from priority paths)
			mu.Lock()
			if sentFiles[androidPath] {
				mu.Unlock()
				continue
			}
			sentFiles[androidPath] = true
			mu.Unlock()

			// Calculate relative path from root
			relPath, err := calculateRelPathFromAndroid(androidPath, androidRoot)
			if err != nil {
				errors <- fmt.Errorf("failed to calculate relative path for %s: %w", androidPath, err)
				continue
			}

			// Send job
			select {
			case jobs <- FileJob{SourcePath: androidPath, RelPath: relPath}:
			case <-ctx.Done():
				cmd.Process.Kill()
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		errors <- fmt.Errorf("error reading general adb find output: %w", err)
	}

	cmd.Wait() // Ignore errors
}

// sanitizeAndroidPath converts local mount paths to Android paths
// If the path looks like a local mount, try to map it to /sdcard
func sanitizeAndroidPath(path string) string {
	// If path already starts with /sdcard or /storage, use as-is
	if strings.HasPrefix(path, "/sdcard") || strings.HasPrefix(path, "/storage") {
		return path
	}

	// For MTP mounts, the root is typically the device root
	// Try to map common patterns to /sdcard
	// This is a heuristic - user should specify /sdcard for ADB mode
	if strings.Contains(path, "mtp:") || strings.Contains(path, "gvfs") {
		// For MTP mounts accessed via ADB, use /sdcard
		return "/sdcard"
	}

	// If path contains "Internal shared storage" or similar, map to /sdcard
	if strings.Contains(strings.ToLower(path), "internal") || strings.Contains(strings.ToLower(path), "shared") {
		return "/sdcard"
	}

	// Default: assume it's already an Android path or user knows what they're doing
	return path
}

// calculateRelPathFromAndroid calculates relative path from Android absolute path
func calculateRelPathFromAndroid(androidPath, androidRoot string) (string, error) {
	// Normalize paths
	androidPath = strings.TrimPrefix(androidPath, androidRoot)
	androidPath = strings.TrimPrefix(androidPath, "/")
	
	// Convert Android path separators if needed (should be / already)
	return androidPath, nil
}

// ADBCopier implements Copier for ADB-based copying
type ADBCopier struct{}

// NewADBCopier creates a new ADB copier
func NewADBCopier() *ADBCopier {
	return &ADBCopier{}
}

// Copy copies a file using adb pull
func (ac *ADBCopier) Copy(ctx context.Context, sourcePath, sourceRoot, destRoot string, progressChan chan<- int64) (int64, error) {
	// Calculate relative path from source root
	relPath, err := calculateRelPathFromAndroid(sourcePath, sourceRoot)
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

	// Create context with timeout for adb pull
	pullCtx, cancel := context.WithTimeout(ctx, ADBPullTimeout)
	defer cancel()

	// Use adb pull to copy the file
	// adb pull /sdcard/path/to/file /local/dest/path
	cmd := exec.CommandContext(pullCtx, "adb", "pull", sourcePath, destPath)

	// Start progress monitoring and connection checking in a goroutine
	progressDone := make(chan bool, 1)
	var bytesCopied int64
	
	// Connection checker for ADB: verify device is still connected
	connTicker := time.NewTicker(10 * time.Second)
	go func() {
		defer connTicker.Stop()
		for {
			select {
			case <-pullCtx.Done():
				return
			case <-progressDone:
				return
			case <-connTicker.C:
				// Check if ADB device is still connected
				checkCmd := exec.CommandContext(pullCtx, "adb", "devices")
				output, err := checkCmd.Output()
				if err != nil {
					cancel() // Connection lost
					return
				}
				// Parse output - should contain device ID
				outputStr := string(output)
				// Check if our device (from sourceRoot) is in the list
				if !strings.Contains(outputStr, "device") {
					// No devices connected or all unauthorized
					cancel() // Connection lost
					return
				}
			}
		}
	}()
	
	// Note: adb pull doesn't provide progress output in a parseable format
	// We'll monitor the destination file size instead
	go func() {
		ticker := time.NewTicker(ProgressUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-pullCtx.Done():
				progressDone <- true
				return
			case <-progressDone:
				return
			case <-ticker.C:
				// Check destination file size
				if info, err := os.Stat(destPath); err == nil {
					bytesCopied = info.Size()
					if progressChan != nil {
						select {
						case progressChan <- bytesCopied:
						default:
						}
					}
				}
			}
		}
	}()

	// Execute adb pull
	err = cmd.Run()
	// Signal progress goroutine to stop
	select {
	case progressDone <- true:
	default:
	}
	// Wait for progress goroutine to finish
	<-progressDone

	// Check if error was due to connection loss
	if err != nil {
		// Check if context was cancelled due to connection loss
		if pullCtx.Err() == context.Canceled {
			// Check if device is still connected
			checkCmd := exec.Command("adb", "devices")
			output, checkErr := checkCmd.Output()
			if checkErr != nil || !strings.Contains(string(output), "device") {
				// Clean up partial file on error
				os.Remove(destPath)
				return 0, fmt.Errorf("connection lost during adb pull: device disconnected")
			}
		}
		// Clean up partial file on error
		os.Remove(destPath)
		return 0, fmt.Errorf("adb pull failed: %w", err)
	}

	// Get final file size
	if info, err := os.Stat(destPath); err == nil {
		bytesCopied = info.Size()
		if progressChan != nil {
			select {
			case progressChan <- bytesCopied:
			default:
			}
		}
	}

	return bytesCopied, nil
}

