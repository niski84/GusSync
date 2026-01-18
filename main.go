package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	stateFileName = "gus_state.md"
	jobBufferSize = 1000
	// MTPMaxWorkers is the maximum recommended workers for MTP protocol
	// MTP typically supports 1-2 concurrent transfers well, some devices support more
	// Using 4 as a safe conservative limit to avoid overwhelming the protocol
	MTPMaxWorkers = 4
)

// PriorityPaths are common Android paths that should be processed first
// These are typical locations for photos, documents, and important user data
var PriorityPaths = []string{
	"DCIM",                    // Camera photos and videos
	"Camera",                  // Camera folder (some devices)
	"Pictures",                // Pictures folder
	"Documents",               // Documents folder
	"Download",                // Downloads
	"Movies",                  // Videos
	"Music",                   // Music files
	"Videos",                  // Videos folder
	"ScreenRecordings",        // Screen recordings
	"Screenshots",             // Screenshots
	"WhatsApp/Media",          // WhatsApp media
	"Android/media",           // App media (contains WhatsApp, etc.)
	"Android/data",            // App data
}

var (
	sourcePath string
	destPath   string
	numWorkers int
	mode       string
)

func init() {
	flag.StringVar(&sourcePath, "source", "", "Source directory to backup (e.g., /mnt/phone or /sdcard for ADB)")
	flag.StringVar(&destPath, "dest", "", "Destination directory (e.g., /mnt/ssd/backup)")
	flag.IntVar(&numWorkers, "workers", 0, "Number of worker threads (default: number of CPU cores)")
	flag.StringVar(&mode, "mode", "mount", "Backup mode: 'mount' (filesystem), 'adb' (Android Debug Bridge), 'cleanup' (delete verified files from source), or 'verify' (verify existing backup)")
}

func main() {
	flag.Parse()

	if sourcePath == "" || destPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -source <src> -dest <dst>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate mode
	if mode != "mount" && mode != "adb" && mode != "cleanup" && mode != "verify" {
		fmt.Fprintf(os.Stderr, "Error: invalid mode '%s'. Must be 'mount', 'adb', 'cleanup', or 'verify'\n", mode)
		os.Exit(1)
	}

	// Handle cleanup mode separately
	if mode == "cleanup" {
		if err := runCleanupMode(sourcePath, destPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cleanup failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle verify mode separately
	if mode == "verify" {
		if err := runVerifyMode(sourcePath, destPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: verification failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Validate source (only for mount mode)
	if mode == "mount" {
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: source path does not exist: %s\n", sourcePath)
			os.Exit(1)
		}
	} else {
		// For ADB mode, verify adb is available
		if _, err := exec.LookPath("adb"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: adb command not found. Please install Android Debug Bridge.\n")
			os.Exit(1)
		}
	}

	// Update destination path to include mode
	destPath = filepath.Join(destPath, mode)

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create destination directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize state manager
	stateFile := filepath.Join(destPath, stateFileName)
	stateManager, err := NewStateManager(stateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create state manager: %v\n", err)
		os.Exit(1)
	}
	defer stateManager.Close()

	// Open error log file
	errorLogFile := filepath.Join(destPath, "gus_errors.log")
	errorLog, err := os.OpenFile(errorLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create error log file: %v\n", err)
		os.Exit(1)
	}
	defer errorLog.Close()
	
	fmt.Printf("Error log: %s\n", errorLogFile)

	// Set default workers if not specified
	// For testing/protocol stability, default to 1 worker
	if numWorkers <= 0 {
		numWorkers = 1 // Default to 1 worker for protocol stability
	}

	fmt.Printf("GusSync - Starting backup\n")
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Source: %s\n", sourcePath)
	fmt.Printf("Dest: %s\n", destPath)
	fmt.Printf("Workers: %d\n", numWorkers)
	fmt.Printf("Already completed: %d files\n\n", stateManager.GetStats())

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channels
	jobChan := make(chan FileJob, jobBufferSize)
	errorChan := make(chan error, 100)
	statsChan := make(chan CopyStats, 100)
	
	// Track discovered files count (separate from processed files)
	var discoveredFiles struct {
		sync.Mutex
		count int64
	}
	
	// Intercept jobs to count discovered files
	jobsChan := make(chan FileJob, jobBufferSize)
	go func() {
		for job := range jobsChan {
			discoveredFiles.Lock()
			discoveredFiles.count++
			discoveredFiles.Unlock()
			// Forward to actual job channel
			jobChan <- job
		}
		close(jobChan)
	}()
	
	// Use sync.Once to ensure jobsChan is only closed once
	var jobsChanOnce sync.Once
	closeJobChan := func() {
		jobsChanOnce.Do(func() {
			close(jobsChan) // Close the scanner's channel, interceptor will close jobChan
		})
	}

	// Select scanner and copier based on mode
	var scanner Scanner
	var copier Copier
	
	if mode == "adb" {
		scanner = NewADBScanner(closeJobChan)
		copier = NewADBCopier()
	} else {
		fsScanner := NewFSScanner(closeJobChan)
		fsScanner.SetStateManager(stateManager)
		scanner = fsScanner
		copier = NewFSCopier()
	}

	// Statistics
	var stats struct {
		sync.Mutex
		totalFiles       int
		completed        int
		failed           int
		skipped          int
		timeoutSkips     int       // Files skipped due to timeout/stall
		consecutiveSkips int       // Consecutive timeout skips
		totalBytes       int64
		lastTotalBytes   int64
		lastStatsTime    time.Time
		startTime        time.Time
	}

	stats.startTime = time.Now()
	stats.lastStatsTime = time.Now()

	// Worker status tracking
	workerStatus := &struct {
		sync.Mutex
		status map[int]string // worker ID -> current status string
	}{status: make(map[int]string)}

	// Worker pool - start workers FIRST so they're ready to consume jobs
	var wg sync.WaitGroup

	// Start workers BEFORE scanner so they're ready when files are discovered
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(ctx, i, jobChan, errorChan, statsChan, stateManager, sourcePath, destPath, mode, copier, workerStatus, &wg)
	}

	// Start scanner AFTER workers are ready (workers will block on jobChan until files arrive)
	go scanner.Scan(ctx, sourcePath, jobsChan, errorChan)

	// Track if we've seen a critical connection error
	connectionDead := make(chan bool, 1)
	
	// Start error handler - write to log file and check for critical errors
	var errorWg sync.WaitGroup
	errorWg.Add(1)
	go func() {
		defer errorWg.Done()
		for err := range errorChan {
			if err != nil {
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				logLine := fmt.Sprintf("[%s] %v\n", timestamp, err)
				errorLog.WriteString(logLine)
				errorLog.Sync() // Flush immediately for tail -f
				
				// Check if this is a critical connection error
				if strings.Contains(err.Error(), "CRITICAL:") {
					fmt.Fprintf(os.Stderr, "\n\n🔴 CRITICAL: Connection dropped - %v\n", err)
					fmt.Fprintf(os.Stderr, "Exiting due to connection failure. Progress has been saved.\n")
					fmt.Fprintf(os.Stderr, "Reconnect the device and run again to resume.\n\n")
					connectionDead <- true
					cancel() // Cancel scanner context
					return
				}
			}
		}
	}()

	// Start stats printer
	var statsWg sync.WaitGroup
	statsWg.Add(1)
	done := make(chan bool)
	go func() {
		defer statsWg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case s := <-statsChan:
				stats.Lock()
				stats.totalFiles++
				if s.Success {
					stats.completed++
					stats.totalBytes += s.BytesCopied
					// Reset consecutive skips on success
					if stats.consecutiveSkips > 0 {
						stats.consecutiveSkips = 0
					}
				} else if s.Skipped {
					stats.skipped++
				} else if s.IsTimeout {
					// Timeout/stall - increment counters
					stats.timeoutSkips++
					stats.consecutiveSkips++
				} else {
					stats.failed++
					// Reset consecutive skips on non-timeout failure
					if stats.consecutiveSkips > 0 {
						stats.consecutiveSkips = 0
					}
				}
				stats.Unlock()

			case <-ticker.C:
				printStats(&stats, stateManager, workerStatus, numWorkers)

			case <-done:
				return
			}
		}
	}()

	// Wait for walker to finish and workers to complete
	go func() {
		wg.Wait()
		close(errorChan)
		done <- true
	}()

	// Handle shutdown or connection death
	shutdownRequested := false
	select {
	case <-sigChan:
		fmt.Println("\n\nShutdown signal received. Finishing current operations...")
		shutdownRequested = true
		// Cancel context to stop walker
		cancel()
		// Close jobChan to signal workers to stop accepting new jobs
		// Workers will finish their current operation and exit
		// Use safe close function (sync.Once ensures it's only closed once)
		closeJobChan()
	case <-connectionDead:
		// Connection dropped - treat as shutdown
		fmt.Println("\n\nConnection dropped - exiting...")
		shutdownRequested = true
		cancel()
		closeJobChan()
	case <-done:
		// Scanner has finished, jobChan is already closed by scanner
		// Just continue to wait for workers
	}

	// Wait for all workers with timeout if shutdown was requested
	if shutdownRequested {
		// Use a channel to wait for workers with timeout
		workersDone := make(chan bool, 1)
		go func() {
			wg.Wait()
			workersDone <- true
		}()

		// Wait up to 10 seconds for workers to finish
		timeout := 10 * time.Second
		select {
		case <-workersDone:
			// Workers finished gracefully
			errorWg.Wait()
		case <-time.After(timeout):
			fmt.Fprintf(os.Stderr, "\n⚠️  Workers did not finish within %v, forcing exit...\n", timeout)
			stateManager.Flush()
			// Force exit immediately - don't wait for anything else
			fmt.Fprintf(os.Stderr, "Force exiting now...\n")
			os.Exit(130) // Exit code 130 typically means killed by SIGINT
		}
		
		// Shutdown was requested - don't run verification pass
		// Final stats
		printStats(&stats, stateManager, workerStatus, numWorkers)
		stateManager.Flush()
		
		fmt.Println("\nBackup interrupted. Progress saved. Run again to resume.")
		return // Exit early - don't run verification pass
	} else {
		// Normal completion - wait for workers without timeout
		wg.Wait()
		errorWg.Wait()
	}

	// Stop stats printer before verification
	close(done)
	statsWg.Wait()
	
	// Final stats
	printStats(&stats, stateManager, workerStatus, numWorkers)
	stateManager.Flush()
	
	// Check if we discovered all files (count actual files in source for comparison)
	discoveredFiles.Lock()
	discoveredCount := discoveredFiles.count
	discoveredFiles.Unlock()
	
	fmt.Printf("\nFiles discovered: %d\n", discoveredCount)
	
	// For mount mode, do a quick actual file count to verify discovery completeness
	if mode == "mount" {
		actualCount := countFilesInSource(sourcePath)
		if actualCount > 0 && discoveredCount < int64(actualCount) {
			missingPct := float64(actualCount-int(discoveredCount)) / float64(actualCount) * 100
			fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: Only discovered %d of %d files (%.1f%% missing)!\n", 
				discoveredCount, actualCount, missingPct)
			fmt.Fprintf(os.Stderr, "   Some directories may have timed out or failed during scanning.\n")
			fmt.Fprintf(os.Stderr, "   Check the error log for directory read timeouts.\n")
			fmt.Fprintf(os.Stderr, "   You may need to run again or increase directory timeout.\n\n")
		} else if actualCount > 0 {
			fmt.Printf("File count verified: %d files found in source (matches discovered count)\n", actualCount)
		}
	}

	fmt.Println("\nBackup complete!")
	
	// Summarize error log if it exists
	if errorLogFile != "" {
		summarizeErrorLog(errorLogFile)
	}
}

// countFilesInSource does a quick count of files in the source directory
// This is used to verify that discovery found all files
func countFilesInSource(root string) int {
	count := 0
	// Use a simple walk with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	done := make(chan bool, 1)
	go func() {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors, just count what we can
			}
			if !d.IsDir() {
				count++
			}
			select {
			case <-ctx.Done():
				return context.Canceled
			default:
				return nil
			}
		})
		done <- true
	}()
	
	select {
	case <-done:
		return count
	case <-ctx.Done():
		// Timeout - return what we counted so far
		return count
	}
}

// CopyStats represents statistics for a copy operation
type CopyStats struct {
	Success     bool
	Skipped     bool
	IsTimeout   bool
	BytesCopied int64
}

// worker processes jobs from the channel
func worker(ctx context.Context, id int, jobChan <-chan FileJob, errorChan chan<- error, statsChan chan<- CopyStats,
	stateManager *StateManager, sourceRoot, destRoot string, mode string, copier Copier, workerStatus *struct {
		sync.Mutex
		status map[int]string
	}, wg *sync.WaitGroup) {
	defer wg.Done()

	defer func() {
		if r := recover(); r != nil {
			errorChan <- fmt.Errorf("worker %d panic: %v", id, r)
		}
	}()

	// Track files that have failed in this run (to avoid multiple failures per run)
	failedThisRun := make(map[string]bool)

		for {
			select {
			case <-ctx.Done():
				// Context cancelled - exit immediately
				return
			case job, ok := <-jobChan:
				if !ok {
					// Channel closed - no more jobs
					return
				}
				
				sourcePath := job.SourcePath
				relPath := job.RelPath
				
				// Check if already done FIRST (before any other operations)
				// This makes resuming much faster - skip immediately without any work
				// BUT: Also verify destination file exists - if marked done but destination missing, recopy it
				if stateManager.IsDone(sourcePath) {
					// Verify destination file actually exists before skipping
					destPathFull := filepath.Join(destRoot, relPath)
					if _, err := os.Stat(destPathFull); err == nil {
						// Destination exists - safe to skip
						statsChan <- CopyStats{Skipped: true}
						continue
					}
					// Destination missing - even though marked done, we need to recopy
					// Log this anomaly and continue with copy
					errorChan <- fmt.Errorf("file marked as done but destination missing: %s -> %s (will recopy)", sourcePath, destPathFull)
				}

				// Check if we should retry (hasn't failed 10 times yet)
				if !stateManager.ShouldRetry(sourcePath) {
					// Silently skip - too many failures
					statsChan <- CopyStats{Skipped: true}
					continue
				}

				// Only get file info and show status if file needs processing
				var fileSize int64
				var fileName string
				if mode == "mount" {
					fileInfo, err := os.Stat(sourcePath)
					if err != nil {
						statsChan <- CopyStats{Success: false}
						errorChan <- fmt.Errorf("failed to stat file %s: %w", sourcePath, err)
						continue
					}
					fileSize = fileInfo.Size()
					fileName = filepath.Base(sourcePath)
				} else {
					// For ADB mode, we don't have file size upfront
					// Will update during copy progress
					fileName = filepath.Base(sourcePath)
					fileSize = 0 // Unknown until copy starts
				}

				// Create progress channel for this copy operation
				progressChan := make(chan int64, 10)
				var bytesCopied int64
				var lastBytes int64
				var lastProgressTime time.Time
				lastProgressTime = time.Now()

				// Monitor progress in a goroutine
				progressDone := make(chan bool)
				go func() {
					defer close(progressDone)
					for {
						select {
						case <-ctx.Done():
							return
						case bytes, ok := <-progressChan:
							if !ok {
								return
							}
							bytesCopied = bytes
							
							// Calculate speed (KB/s or MB/s)
							now := time.Now()
							elapsed := now.Sub(lastProgressTime).Seconds()
							var speedStr string
							if elapsed > 0 && bytes > lastBytes {
								bytesPerSec := float64(bytes-lastBytes) / elapsed
								if bytesPerSec >= 1024*1024 {
									// Show as MB/s
									speedStr = fmt.Sprintf(" %.1f MB/s", bytesPerSec/(1024*1024))
								} else if bytesPerSec >= 1024 {
									// Show as KB/s
									speedStr = fmt.Sprintf(" %.1f KB/s", bytesPerSec/1024)
								} else {
									// Show as B/s
									speedStr = fmt.Sprintf(" %.0f B/s", bytesPerSec)
								}
							} else {
								speedStr = " 0 B/s"
							}
							
							lastBytes = bytes
							lastProgressTime = now
							
							// Calculate progress percentage
							var percent float64
							if fileSize > 0 {
								percent = float64(bytesCopied) / float64(fileSize) * 100
							}
							
							// Update worker status with progress and speed
							workerStatus.Lock()
							if fileSize > 0 {
								workerStatus.status[id] = fmt.Sprintf("Copying: %s (%s/%s %.1f%%%s)", 
									fileName, formatSize(bytesCopied), formatSize(fileSize), percent, speedStr)
							} else {
								workerStatus.status[id] = fmt.Sprintf("Copying: %s (%s%s)", 
									fileName, formatSize(bytesCopied), speedStr)
							}
							workerStatus.Unlock()
						}
					}
				}()

				// Initial status
				workerStatus.Lock()
				workerStatus.status[id] = fmt.Sprintf("Starting: %s (%s)", fileName, formatSize(fileSize))
				workerStatus.Unlock()

				// Copy the file using the copier interface
				bytesCopied, err := copier.Copy(ctx, sourcePath, sourceRoot, destRoot, progressChan)
				close(progressChan)
				<-progressDone // Wait for progress monitor to finish

				// Clear or update worker status after copy
				workerStatus.Lock()
				if err == nil {
					workerStatus.status[id] = ""
				} else {
					// Keep status showing failure for a moment
					workerStatus.status[id] = fmt.Sprintf("Failed: %s", fileName)
				}
				workerStatus.Unlock()

				// Check if this was a timeout/stall error
				isTimeoutError := err != nil && strings.Contains(err.Error(), "stalled")
				
				if err == nil {
					// Copy succeeded - calculate hash for verification
					// For mount mode, we can hash both files
					// For ADB mode, we hash destination (source is on device)
					var sourceHash, destHash string
					
					destPathFull := filepath.Join(destPath, relPath)
					
					if mode == "mount" {
						sourceHash, err = calculateFileHash(sourcePath)
						if err != nil {
							errorChan <- fmt.Errorf("failed to hash source %s: %w", sourcePath, err)
							continue
						}
					}
					
					destHash, err = calculateFileHash(destPathFull)
					if err != nil {
						errorChan <- fmt.Errorf("failed to hash dest %s: %w", destPathFull, err)
						continue
					}
					
					// For ADB mode, we only verify destination exists and has content
					// For mount mode, verify hashes match
					if mode == "mount" && sourceHash != destHash {
						errorChan <- fmt.Errorf("hash mismatch for %s: source=%s, dest=%s", sourcePath, sourceHash, destHash)
						// Treat as failure
						if !failedThisRun[sourcePath] {
							if err := stateManager.RecordFailure(sourcePath); err != nil {
								errorChan <- fmt.Errorf("failed to record failure: %w", err)
							}
							failedThisRun[sourcePath] = true
						}
						statsChan <- CopyStats{Success: false}
						continue
					}
					
					// Mark that we've had at least one success (enables failure tracking)
					stateManager.MarkSuccess()

					// Mark as done (use destHash for ADB, sourceHash for mount)
					hashToStore := destHash
					if mode == "mount" {
						hashToStore = sourceHash
					}
					if err := stateManager.MarkDone(sourcePath, hashToStore); err != nil {
						errorChan <- fmt.Errorf("failed to mark done: %w", err)
					}

					// Remove from failed map if it was there
					delete(failedThisRun, sourcePath)

					// Flush periodically (every 100 files or so)
					if id == 0 { // Only one worker flushes to avoid contention
						_ = stateManager.Flush()
					}

					statsChan <- CopyStats{
						Success:     true,
						BytesCopied: bytesCopied,
					}
				} else if isTimeoutError {
					// This is a timeout/stall - count it and move to next file
					// Don't record as failure - just skip it and move on
					statsChan <- CopyStats{
						Success: false,
						IsTimeout: true,
					}
					// Note: consecutive skip count will be updated in stats handler
					errorChan <- fmt.Errorf("copy timed out (stalled) for %s: %v", sourcePath, err)
				} else {
					// Other error - record as failure
					// Only record failure once per run, and only if we've had a success
					if !failedThisRun[sourcePath] {
						if err2 := stateManager.RecordFailure(sourcePath); err2 != nil {
							errorChan <- fmt.Errorf("failed to record failure: %w", err2)
						}
						failedThisRun[sourcePath] = true
					}

					statsChan <- CopyStats{
						Success: false,
					}
					errorChan <- fmt.Errorf("copy failed for %s: %v", sourcePath, err)
				}
			}
		}
}

// formatSize formats bytes as human-readable size
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printStats prints current statistics with worker activity
func printStats(stats *struct {
	sync.Mutex
	totalFiles       int
	completed        int
	failed           int
	skipped          int
	timeoutSkips     int
	consecutiveSkips int
	totalBytes       int64
	lastTotalBytes   int64
	lastStatsTime    time.Time
	startTime        time.Time
}, stateManager *StateManager, workerStatus *struct {
	sync.Mutex
	status map[int]string
}, numWorkers int) {
	stats.Lock()
	elapsed := time.Since(stats.startTime)
	deltaTime := time.Since(stats.lastStatsTime)
	
	var rate float64
	var deltaMB float64
	if elapsed.Seconds() > 0 {
		rate = float64(stats.totalBytes) / elapsed.Seconds()
	}
	if deltaTime.Seconds() > 0 {
		deltaBytes := stats.totalBytes - stats.lastTotalBytes
		deltaMB = float64(deltaBytes) / (1024 * 1024)
	}
	
	stats.lastTotalBytes = stats.totalBytes
	stats.lastStatsTime = time.Now()
	stats.Unlock()

	completed := stateManager.GetStats()
	
	// Get worker statuses
	workerStatus.Lock()
	workerStatuses := make([]string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		if status, ok := workerStatus.status[i]; ok && status != "" {
			workerStatuses[i] = status
		} else {
			workerStatuses[i] = "idle"
		}
	}
	workerStatus.Unlock()
	
	stats.Lock()
	timeoutSkips := stats.timeoutSkips
	consecutiveSkips := stats.consecutiveSkips
	stats.Unlock()
	
	// Print summary line
	var statusLine string
	if deltaMB > 0 {
		statusLine = fmt.Sprintf("\r[%d files] Completed: %d | Skipped: %d | Failed: %d | Timeouts: %d (consecutive: %d) | Speed: %.2f MB/s | Delta: %.2f MB",
			stats.totalFiles, completed, stats.skipped, stats.failed, timeoutSkips, consecutiveSkips, rate/(1024*1024), deltaMB)
	} else {
		statusLine = fmt.Sprintf("\r[%d files] Completed: %d | Skipped: %d | Failed: %d | Timeouts: %d (consecutive: %d) | Speed: %.2f MB/s",
			stats.totalFiles, completed, stats.skipped, stats.failed, timeoutSkips, consecutiveSkips, rate/(1024*1024))
	}
	
	// Print worker activity
	fmt.Print(statusLine + "\n")
	for i, status := range workerStatuses {
		if i < numWorkers {
			fmt.Printf("  W%d: %s\n", i, status)
		}
	}
}

// VerifyResults contains results from the verification pass
type VerifyResults struct {
	verified      int
	missingSource int
	missingDest   int
	mismatches    int
}

// verifyBackup compares source and destination hashes for all completed files
func verifyBackup(sourceBase, destBase string, stateManager *StateManager, numWorkers int, mode string, copier Copier) VerifyResults {
	allCompletedFiles := stateManager.GetAllCompletedFiles()
	
	if len(allCompletedFiles) == 0 {
		fmt.Println("No files to verify.")
		return VerifyResults{}
	}
	
	// Filter completed files to only include those under the current sourceBase
	// This handles cases where the state file contains paths from previous runs with different mount points
	completedFiles := make(map[string]string)
	sourceBaseCleaned := filepath.Clean(sourceBase)
	for path, hash := range allCompletedFiles {
		pathCleaned := filepath.Clean(path)
		// Check if this path is under the current sourceBase
		if strings.HasPrefix(pathCleaned, sourceBaseCleaned) {
			completedFiles[path] = hash
		}
	}
	
	if len(completedFiles) == 0 {
		fmt.Printf("No files to verify (all %d completed files are from a different source path).\n", len(allCompletedFiles))
		return VerifyResults{}
	}
	
	totalFiles := len(completedFiles)
	fmt.Printf("Verifying %d files (filtered from %d total in state)...\n", totalFiles, len(allCompletedFiles))
	
	var results VerifyResults
	var mu sync.Mutex
	var verifiedCount int64
	
	// Create job channel for verification
	verifyChan := make(chan string, 1000)
	var wg sync.WaitGroup
	
	// Progress ticker for verification
	verifyTicker := time.NewTicker(5 * time.Second)
	verifyDone := make(chan bool)
	go func() {
		for {
			select {
			case <-verifyTicker.C:
				mu.Lock()
				currentVerified := verifiedCount
				mu.Unlock()
				fmt.Printf("\rVerifying... %d/%d files (%.1f%%)", currentVerified, totalFiles, float64(currentVerified)/float64(totalFiles)*100)
			case <-verifyDone:
				return
			}
		}
	}()
	
	// Start verification workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sourcePath := range verifyChan {
				// Check if source file still exists (only for mount mode)
				if mode == "mount" {
					if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
						mu.Lock()
						results.missingSource++
						mu.Unlock()
						fmt.Fprintf(os.Stderr, "⚠️  Source file missing: %s\n", sourcePath)
						continue
					}
				}
				// For ADB mode, we skip source existence check (would require adb shell stat)
				
				// Build destination path preserving directory structure
				relPath, err := filepath.Rel(sourceBase, sourcePath)
				if err != nil {
					// Fallback to base name if relative path calculation fails
					relPath = filepath.Base(sourcePath)
				}
				destPath := filepath.Join(destBase, relPath)
				
				// Check if destination file exists
				if _, err2 := os.Stat(destPath); os.IsNotExist(err2) {
					mu.Lock()
					results.missingDest++
					mu.Unlock()
					fmt.Fprintf(os.Stderr, "⚠️  Destination file missing: %s\n", destPath)
					continue
				}
				
				// Calculate hashes
				var sourceHash string
				
				if mode == "mount" {
					// For mount mode, hash both source and destination
					var err2 error
					sourceHash, err2 = calculateFileHash(sourcePath)
					if err2 != nil {
						fmt.Fprintf(os.Stderr, "⚠️  Failed to hash source: %s - %v\n", sourcePath, err2)
						continue
					}
				}
				
				destHash, err2 := calculateFileHash(destPath)
				if err2 != nil {
					fmt.Fprintf(os.Stderr, "⚠️  Failed to hash destination: %s - %v\n", destPath, err2)
					continue
				}
				
				// For ADB mode, we only verify destination exists and has content
				// For mount mode, compare source and destination hashes
				if mode == "adb" {
					// ADB mode: just verify destination exists and has content
					// The hash stored in state should match destination
					mu.Lock()
					results.verified++
					verifiedCount++
					mu.Unlock()
					continue
				}
				
				// Compare source and destination hashes (they should match for mount mode)
				if sourceHash != destHash {
					fmt.Fprintf(os.Stderr, "⚠️  Hash mismatch detected: %s\n", sourcePath)
					fmt.Fprintf(os.Stderr, "   Source hash: %s\n", sourceHash)
					fmt.Fprintf(os.Stderr, "   Dest hash:   %s\n", destHash)
					fmt.Fprintf(os.Stderr, "   Attempting to re-copy...\n")
					
					// Attempt to re-copy the file (will overwrite destination)
					// Use copier interface for re-copy
					ctx := context.Background()
					_, err3 := copier.Copy(ctx, sourcePath, sourceBase, destBase, nil)
					success := err3 == nil
					
					if success {
						// Re-copy succeeded, verify the hashes match now
						newDestHash, err := calculateFileHash(destPath)
						if err != nil {
							fmt.Fprintf(os.Stderr, "   ⚠️  Failed to verify re-copy hash: %v\n", err)
							mu.Lock()
							results.mismatches++
							mu.Unlock()
						} else if sourceHash == newDestHash {
							fmt.Fprintf(os.Stderr, "   ✅ Re-copy successful, hashes now match\n")
							mu.Lock()
							results.verified++
							mu.Unlock()
						} else {
							fmt.Fprintf(os.Stderr, "   ⚠️  Re-copy completed but hashes still don't match\n")
							fmt.Fprintf(os.Stderr, "   Source: %s, Dest: %s\n", sourceHash, newDestHash)
							mu.Lock()
							results.mismatches++
							mu.Unlock()
						}
					} else {
						fmt.Fprintf(os.Stderr, "   ⚠️  Re-copy failed: %v\n", err3)
						mu.Lock()
						results.mismatches++
						mu.Unlock()
					}
				} else {
					mu.Lock()
					results.verified++
					verifiedCount++
					mu.Unlock()
				}
			}
		}()
	}
	
	// Send all files to verification channel
	for sourcePath := range completedFiles {
		verifyChan <- sourcePath
	}
	close(verifyChan)
	
	// Wait for all workers
	wg.Wait()
	
	// Stop progress ticker
	verifyTicker.Stop()
	verifyDone <- true
	close(verifyDone)
	
	// Print final verification progress
	mu.Lock()
	finalVerified := verifiedCount
	mu.Unlock()
	fmt.Printf("\rVerifying... %d/%d files (100.0%%) - Complete!\n", finalVerified, totalFiles)
	
	return results
}

// summarizeErrorLog reads and summarizes the error log file
func summarizeErrorLog(errorLogFile string) {
	file, err := os.Open(errorLogFile)
	if os.IsNotExist(err) {
		return // No error log yet
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read error log: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	
	var directoryTimeouts int
	var directoryErrors int
	var copyErrors int
	var criticalErrors int
	var hashMismatches int
	var otherErrors int
	
	timeoutDirs := make(map[string]bool)
	errorDirs := make(map[string]bool)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Count different error types
		if strings.Contains(line, "CRITICAL:") {
			criticalErrors++
		} else if strings.Contains(line, "directory read timeout") {
			directoryTimeouts++
			// Extract directory path from error
			// Pattern: "directory read timeout: /path/to/dir" or "directory read timeout: /path/to/dir (continuing..."
			if idx := strings.Index(line, "directory read timeout: "); idx >= 0 {
				pathStart := idx + len("directory read timeout: ")
				pathEnd := strings.Index(line[pathStart:], " (")
				if pathEnd < 0 {
					pathEnd = len(line)
				} else {
					pathEnd += pathStart
				}
				dir := strings.TrimSpace(line[pathStart:pathEnd])
				if dir != "" {
					timeoutDirs[dir] = true
				}
			}
		} else if strings.Contains(line, "error reading") && strings.Contains(line, ":") {
			directoryErrors++
			// Extract directory path from error
			// Pattern: "error reading /path/to/dir: ..."
			if idx := strings.Index(line, "error reading "); idx >= 0 {
				pathStart := idx + len("error reading ")
				pathEnd := strings.Index(line[pathStart:], ":")
				if pathEnd >= 0 {
					dir := strings.TrimSpace(line[pathStart : pathStart+pathEnd])
					if dir != "" {
						errorDirs[dir] = true
					}
				}
			}
		} else if strings.Contains(line, "hash mismatch") {
			hashMismatches++
		} else if strings.Contains(line, "copy failed") || strings.Contains(line, "copy timed out") || strings.Contains(line, "stalled") {
			copyErrors++
		} else if strings.Contains(line, "[ERROR]") || strings.Contains(line, "failed to") {
			otherErrors++
		}
	}
	
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error reading error log: %v\n", err)
		return
	}
	
	// Print summary only if there are errors
	totalErrors := directoryTimeouts + directoryErrors + copyErrors + criticalErrors + hashMismatches + otherErrors
	
	if totalErrors == 0 {
		return // No errors, don't print anything
	}
	
	fmt.Printf("\nError Log Summary:\n")
	fmt.Printf("  Total errors: %d\n", totalErrors)
	
	if criticalErrors > 0 {
		fmt.Printf("  🔴 Critical errors (connection lost): %d\n", criticalErrors)
	}
	if directoryTimeouts > 0 {
		fmt.Printf("  ⏱️  Directory read timeouts: %d (affecting %d unique directories)\n", directoryTimeouts, len(timeoutDirs))
		if len(timeoutDirs) > 0 && len(timeoutDirs) <= 10 {
			fmt.Printf("     Timed-out directories:\n")
			for dir := range timeoutDirs {
				// Truncate long paths for display
				displayDir := dir
				if len(displayDir) > 70 {
					displayDir = "..." + displayDir[len(displayDir)-67:]
				}
				fmt.Printf("       - %s\n", displayDir)
			}
		} else if len(timeoutDirs) > 10 {
			fmt.Printf("     (showing first 10 of %d timed-out directories)\n", len(timeoutDirs))
			count := 0
			for dir := range timeoutDirs {
				if count >= 10 {
					break
				}
				displayDir := dir
				if len(displayDir) > 70 {
					displayDir = "..." + displayDir[len(displayDir)-67:]
				}
				fmt.Printf("       - %s\n", displayDir)
				count++
			}
		}
	}
	if directoryErrors > 0 {
		fmt.Printf("  ⚠️  Directory read errors: %d (affecting %d unique directories)\n", directoryErrors, len(errorDirs))
		if len(errorDirs) > 0 && len(errorDirs) <= 10 {
			fmt.Printf("     Error directories:\n")
			for dir := range errorDirs {
				displayDir := dir
				if len(displayDir) > 70 {
					displayDir = "..." + displayDir[len(displayDir)-67:]
				}
				fmt.Printf("       - %s\n", displayDir)
			}
		} else if len(errorDirs) > 10 {
			fmt.Printf("     (showing first 10 of %d error directories)\n", len(errorDirs))
			count := 0
			for dir := range errorDirs {
				if count >= 10 {
					break
				}
				displayDir := dir
				if len(displayDir) > 70 {
					displayDir = "..." + displayDir[len(displayDir)-67:]
				}
				fmt.Printf("       - %s\n", displayDir)
				count++
			}
		}
	}
	if hashMismatches > 0 {
		fmt.Printf("  🔍 Hash mismatches: %d\n", hashMismatches)
	}
	if copyErrors > 0 {
		fmt.Printf("  📁 File copy errors: %d\n", copyErrors)
	}
	if otherErrors > 0 {
		fmt.Printf("  ⚠️  Other errors: %d\n", otherErrors)
	}
	
	// If there are timeouts/errors, suggest action
	if directoryTimeouts > 0 || directoryErrors > 0 {
		fmt.Printf("\n  💡 Suggestion: Directories with timeouts/errors will be retried on next run.\n")
		fmt.Printf("     Consider increasing directory timeout if timeouts persist.\n")
	}
}
