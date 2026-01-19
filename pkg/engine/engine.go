package engine

import (
	"GusSync/pkg/state"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProgressUpdate contains current statistics for reporting
type ProgressUpdate struct {
	TotalFiles       int
	Completed        int
	Failed           int
	Skipped          int
	TimeoutSkips     int
	ConsecutiveSkips int
	TotalBytes       int64
	Rate             float64 // bytes per second
	DeltaMB          float64 // MB since last report
	WorkerStatuses   map[int]string
	ScanComplete     bool
	JobID            string
}

// ProgressReporter interface for reporting progress to CLI or GUI
type ProgressReporter interface {
	ReportProgress(update ProgressUpdate)
	ReportError(err error)
	ReportLog(level, message string)
}

// EngineConfig configuration for the backup engine
type EngineConfig struct {
	SourcePath string
	DestRoot   string
	Mode       string // "mount" or "adb"
	NumWorkers int
	Reporter   ProgressReporter
}

// Engine the core backup engine
type Engine struct {
	config       EngineConfig
	stateManager *state.StateManager
	stats        struct {
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
	}
	workerStatus struct {
		sync.Mutex
		status map[int]string
	}
}

// NewEngine creates a new backup engine
func NewEngine(config EngineConfig, sm *state.StateManager) *Engine {
	if config.NumWorkers <= 0 {
		config.NumWorkers = 1
	}
	e := &Engine{
		config:       config,
		stateManager: sm,
	}
	e.stats.startTime = time.Now()
	e.stats.lastStatsTime = time.Now()
	e.workerStatus.status = make(map[int]string)
	return e
}

// Run starts the backup process
func (e *Engine) Run(ctx context.Context) error {
	// Channels
	jobChan := make(chan FileJob, 1000)
	errorChan := make(chan error, 100)
	statsChan := make(chan CopyStats, 100)

	// Use sync.Once to ensure jobChan is closed only once
	var jobsChanOnce sync.Once
	closeJobChan := func() {
		jobsChanOnce.Do(func() {
			close(jobChan)
		})
	}

	// Select scanner and copier based on mode
	var scanner Scanner
	var copier Copier

	if e.config.Mode == "adb" {
		scanner = NewADBScanner(closeJobChan)
		copier = NewADBCopier()
	} else {
		fsScanner := NewFSScanner(closeJobChan)
		fsScanner.SetStateManager(e.stateManager)
		scanner = fsScanner
		copier = NewFSCopier()
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < e.config.NumWorkers; i++ {
		wg.Add(1)
		go e.worker(ctx, i, jobChan, errorChan, statsChan, copier, &wg)
	}

	// Start scanner
	go scanner.Scan(ctx, e.config.SourcePath, jobChan, errorChan)

	// Start reporters
	done := make(chan bool)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case s := <-statsChan:
				e.stats.Lock()
				e.stats.totalFiles++
				if s.Success {
					e.stats.completed++
					e.stats.totalBytes += s.BytesCopied
					e.stats.consecutiveSkips = 0
				} else if s.Skipped {
					e.stats.skipped++
				} else if s.IsTimeout {
					e.stats.timeoutSkips++
					e.stats.consecutiveSkips++
				} else {
					e.stats.failed++
					e.stats.consecutiveSkips = 0
				}
				e.stats.Unlock()

			case err := <-errorChan:
				if err != nil {
					// Distinguish between critical and non-critical errors
					errStr := err.Error()
					if strings.Contains(errStr, "CRITICAL") || strings.Contains(errStr, "connection lost") {
						e.config.Reporter.ReportError(err)
					} else {
						// File-level errors are reported as warnings in the log
						e.config.Reporter.ReportLog("warn", errStr)
					}
				}

			case <-ticker.C:
				e.reportProgress(false)

			case <-done:
				e.reportProgress(true)
				return
			}
		}
	}()

	// Wait for completion
	wg.Wait()
	close(statsChan)
	close(errorChan)
	done <- true

	e.stats.Lock()
	e.config.Reporter.ReportLog("info", fmt.Sprintf("Backup finished: %d completed, %d failed, %d skipped", e.stats.completed, e.stats.failed, e.stats.skipped))
	e.stats.Unlock()

	return nil
}

// VerifyResults contains results from the verification pass
type VerifyResults struct {
	Verified      int
	MissingSource int
	MissingDest   int
	Mismatches    int
}

// VerifyBackup compares source and destination hashes for all completed files
func (e *Engine) VerifyBackup(ctx context.Context) (VerifyResults, error) {
	allCompletedFiles := e.stateManager.GetAllCompletedFiles()
	
	if len(allCompletedFiles) == 0 {
		return VerifyResults{}, nil
	}
	
	// Filter completed files to only include those under the current sourcePath
	completedFiles := make(map[string]string)
	sourceBaseCleaned := filepath.Clean(e.config.SourcePath)
	for path, hash := range allCompletedFiles {
		pathCleaned := filepath.Clean(path)
		if strings.HasPrefix(pathCleaned, sourceBaseCleaned) {
			completedFiles[path] = hash
		}
	}
	
	if len(completedFiles) == 0 {
		return VerifyResults{}, nil
	}
	
	var results VerifyResults
	var mu sync.Mutex
	var verifiedCount int64
	
	verifyChan := make(chan string, 1000)
	var wg sync.WaitGroup
	
	// Start verification workers
	for i := 0; i < e.config.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var copier Copier
			if e.config.Mode == "adb" {
				copier = NewADBCopier()
			} else {
				copier = NewFSCopier()
			}

			for sourcePath := range verifyChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if e.config.Mode == "mount" {
					if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
						mu.Lock()
						results.MissingSource++
						mu.Unlock()
						continue
					}
				}
				
				relPath, err := filepath.Rel(e.config.SourcePath, sourcePath)
				if err != nil {
					relPath = filepath.Base(sourcePath)
				}
				destPath := filepath.Join(e.config.DestRoot, relPath)
				
				if _, err2 := os.Stat(destPath); os.IsNotExist(err2) {
					mu.Lock()
					results.MissingDest++
					mu.Unlock()
					continue
				}
				
				var sourceHash string
				if e.config.Mode == "mount" {
					var err2 error
					sourceHash, err2 = calculateFileHash(sourcePath)
					if err2 != nil {
						continue
					}
				}
				
				destHash, err2 := calculateFileHash(destPath)
				if err2 != nil {
					continue
				}
				
				if e.config.Mode == "adb" {
					mu.Lock()
					results.Verified++
					verifiedCount++
					mu.Unlock()
					continue
				}
				
				if sourceHash != destHash {
					mu.Lock()
					results.Mismatches++
					mu.Unlock()
					
					// Attempt re-copy
					_, err3 := copier.Copy(ctx, sourcePath, e.config.SourcePath, e.config.DestRoot, nil)
					if err3 == nil {
						newDestHash, err := calculateFileHash(destPath)
						if err == nil && sourceHash == newDestHash {
							mu.Lock()
							results.Verified++
							mu.Unlock()
						}
					}
				} else {
					mu.Lock()
					results.Verified++
					verifiedCount++
					mu.Unlock()
				}
			}
		}()
	}
	
	for sourcePath := range completedFiles {
		verifyChan <- sourcePath
	}
	close(verifyChan)
	wg.Wait()
	
	return results, nil
}

// CleanupResults contains results from the cleanup pass
type CleanupResults struct {
	Deleted        int
	AlreadyDeleted int
	Failed         int
	Skipped        int
	IOErrors       int
}

// RunCleanup deletes source files that are verified in the destination
func (e *Engine) RunCleanup(ctx context.Context) (CleanupResults, error) {
	completedFiles := e.stateManager.GetAllCompletedFiles()
	
	if e.config.Reporter != nil {
		e.config.Reporter.ReportLog("info", fmt.Sprintf("Cleanup: Found %d completed files in state", len(completedFiles)))
	}
	
	if len(completedFiles) == 0 {
		if e.config.Reporter != nil {
			e.config.Reporter.ReportLog("info", "Cleanup: No completed files to process")
		}
		return CleanupResults{}, nil
	}

	var results CleanupResults
	filesToProcess := make([]struct{ path, hash string }, 0)

	for path, hash := range completedFiles {
		if e.stateManager.IsDeleted(path) {
			results.AlreadyDeleted++
			continue
		}
		if !e.stateManager.ShouldRetryCleanup(path) {
			results.Skipped++
			continue
		}
		filesToProcess = append(filesToProcess, struct{ path, hash string }{path, hash})
	}

	totalToProcess := len(filesToProcess)
	if e.config.Reporter != nil {
		e.config.Reporter.ReportLog("info", fmt.Sprintf("Cleanup: Processing %d files (skipped %d already deleted, %d failed too many times)", 
			totalToProcess, results.AlreadyDeleted, results.Skipped))
	}

	lastReport := time.Now()
	processed := 0

	for _, file := range filesToProcess {
		select {
		case <-ctx.Done():
			return results, context.Canceled
		default:
		}

		sourcePath := file.path
		expectedHash := file.hash
		processed++

		// Report progress periodically
		if e.config.Reporter != nil && time.Since(lastReport) > 2*time.Second {
			e.config.Reporter.ReportProgress(ProgressUpdate{
				TotalFiles: totalToProcess,
				Completed:  results.Deleted,
				Failed:     results.Failed,
				Skipped:    results.Skipped,
			})
			lastReport = time.Now()
		}

		// Stat check
		info, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				results.Skipped++
				continue
			}
			results.IOErrors++
			continue
		}

		if info.IsDir() {
			results.Skipped++
			continue
		}

		// Determine destination path
		relPath, _ := filepath.Rel(e.config.SourcePath, sourcePath)
		destPath := filepath.Join(e.config.DestRoot, relPath)

		// Check destination
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Restore if missing (as in original logic)
			copyResult := RobustCopy(sourcePath, e.config.SourcePath, e.config.DestRoot, nil)
			if !copyResult.Success {
				e.stateManager.RecordCleanupFailure(sourcePath)
				results.Failed++
				continue
			}
		}

		// Verify hashes
		destHash, err1 := calculateFileHash(destPath)
		sourceHash, err2 := calculateFileHash(sourcePath)

		if err1 == nil && err2 == nil && sourceHash == expectedHash && destHash == expectedHash {
			if err := os.Remove(sourcePath); err == nil {
				e.stateManager.MarkDeleted(sourcePath, expectedHash)
				results.Deleted++
				if e.config.Reporter != nil && results.Deleted%10 == 0 {
					e.config.Reporter.ReportLog("info", fmt.Sprintf("Deleted %d files so far...", results.Deleted))
				}
			} else {
				e.stateManager.RecordCleanupFailure(sourcePath)
				results.Failed++
			}
		} else {
			e.stateManager.RecordCleanupFailure(sourcePath)
			results.Failed++
		}
	}

	// Final report
	if e.config.Reporter != nil {
		e.config.Reporter.ReportProgress(ProgressUpdate{
			TotalFiles:   totalToProcess,
			Completed:    results.Deleted,
			Failed:       results.Failed,
			Skipped:      results.Skipped,
			ScanComplete: true,
		})
		e.config.Reporter.ReportLog("info", fmt.Sprintf("Cleanup complete: %d deleted, %d failed, %d skipped", 
			results.Deleted, results.Failed, results.Skipped))
	}

	return results, nil
}

// ErrorSummary contains a summary of errors found in the log
type ErrorSummary struct {
	TotalErrors       int
	CriticalErrors    int
	DirectoryTimeouts int
	DirectoryErrors   int
	HashMismatches    int
	CopyErrors        int
	OtherErrors       int
	TimeoutDirs       []string
	ErrorDirs         []string
}

// SummarizeErrorLog reads and summarizes the error log file
func SummarizeErrorLog(errorLogFile string) (ErrorSummary, error) {
	file, err := os.Open(errorLogFile)
	if os.IsNotExist(err) {
		return ErrorSummary{}, nil
	}
	if err != nil {
		return ErrorSummary{}, err
	}
	defer file.Close()

	var summary ErrorSummary
	timeoutDirsMap := make(map[string]bool)
	errorDirsMap := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.Contains(line, "CRITICAL:") {
			summary.CriticalErrors++
		} else if strings.Contains(line, "directory read timeout") {
			summary.DirectoryTimeouts++
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
					timeoutDirsMap[dir] = true
				}
			}
		} else if strings.Contains(line, "error reading") && strings.Contains(line, ":") {
			summary.DirectoryErrors++
			if idx := strings.Index(line, "error reading "); idx >= 0 {
				pathStart := idx + len("error reading ")
				pathEnd := strings.Index(line[pathStart:], ":")
				if pathEnd >= 0 {
					dir := strings.TrimSpace(line[pathStart : pathStart+pathEnd])
					if dir != "" {
						errorDirsMap[dir] = true
					}
				}
			}
		} else if strings.Contains(line, "hash mismatch") {
			summary.HashMismatches++
		} else if strings.Contains(line, "copy failed") || strings.Contains(line, "copy timed out") || strings.Contains(line, "stalled") {
			summary.CopyErrors++
		} else if strings.Contains(line, "[ERROR]") || strings.Contains(line, "failed to") {
			summary.OtherErrors++
		}
	}

	for dir := range timeoutDirsMap {
		summary.TimeoutDirs = append(summary.TimeoutDirs, dir)
	}
	for dir := range errorDirsMap {
		summary.ErrorDirs = append(summary.ErrorDirs, dir)
	}

	summary.TotalErrors = summary.CriticalErrors + summary.DirectoryTimeouts + summary.DirectoryErrors + summary.HashMismatches + summary.CopyErrors + summary.OtherErrors

	return summary, scanner.Err()
}

func (e *Engine) reportProgress(final bool) {
	e.stats.Lock()
	defer e.stats.Unlock()

	now := time.Now()
	deltaTime := now.Sub(e.stats.lastStatsTime)
	
	var rate float64
	var deltaMB float64
	
	if deltaTime.Seconds() > 0 {
		deltaBytes := e.stats.totalBytes - e.stats.lastTotalBytes
		deltaMB = float64(deltaBytes) / (1024 * 1024)
		rate = float64(deltaBytes) / deltaTime.Seconds()
	}
	
	e.stats.lastTotalBytes = e.stats.totalBytes
	e.stats.lastStatsTime = now

	e.workerStatus.Lock()
	workerStatuses := make(map[int]string)
	for i, s := range e.workerStatus.status {
		workerStatuses[i] = s
	}
	e.workerStatus.Unlock()

	update := ProgressUpdate{
		TotalFiles:       e.stats.totalFiles,
		Completed:        e.stats.completed,
		Failed:           e.stats.failed,
		Skipped:          e.stats.skipped,
		TimeoutSkips:     e.stats.timeoutSkips,
		ConsecutiveSkips: e.stats.consecutiveSkips,
		TotalBytes:       e.stats.totalBytes,
		Rate:             rate,
		DeltaMB:          deltaMB,
		WorkerStatuses:   workerStatuses,
		ScanComplete:     final,
	}

	e.config.Reporter.ReportProgress(update)
}

func (e *Engine) worker(ctx context.Context, id int, jobChan <-chan FileJob, errorChan chan<- error, statsChan chan<- CopyStats, copier Copier, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobChan:
			if !ok {
				return
			}

			sourcePath := job.SourcePath
			relPath := job.RelPath

			// Check if already done
			if e.stateManager.IsDoneForSource(sourcePath, e.config.SourcePath) {
				statsChan <- CopyStats{Skipped: true}
				continue
			}

			if !e.stateManager.ShouldRetry(sourcePath) {
				statsChan <- CopyStats{Skipped: true}
				continue
			}

			// Report starting
			e.workerStatus.Lock()
			e.workerStatus.status[id] = fmt.Sprintf("Starting: %s", filepath.Base(sourcePath))
			e.workerStatus.Unlock()

			// Create progress channel for this copy
			progressChan := make(chan int64, 10)
			
			// Monitor progress in goroutine
			go func() {
				for bytes := range progressChan {
					e.workerStatus.Lock()
					e.workerStatus.status[id] = fmt.Sprintf("Copying: %s (%s)", filepath.Base(sourcePath), formatSize(bytes))
					e.workerStatus.Unlock()
				}
			}()

			// Copy
			bytesCopied, err := copier.Copy(ctx, sourcePath, e.config.SourcePath, e.config.DestRoot, progressChan)
			close(progressChan)

			if err == nil {
				// Mark done
				hash, _ := calculateFileHash(filepath.Join(e.config.DestRoot, relPath)) // Simplified
				normalizedPath, _ := normalizePhonePath(sourcePath, e.config.SourcePath)
				e.stateManager.MarkDone(sourcePath, hash, normalizedPath)
				e.stateManager.MarkSuccess()
				
				statsChan <- CopyStats{Success: true, BytesCopied: bytesCopied}
				
				e.workerStatus.Lock()
				e.workerStatus.status[id] = "idle"
				e.workerStatus.Unlock()
			} else {
				e.stateManager.RecordFailure(sourcePath)
				isTimeout := strings.Contains(err.Error(), "stalled")
				statsChan <- CopyStats{Success: false, IsTimeout: isTimeout}
				
				e.workerStatus.Lock()
				e.workerStatus.status[id] = fmt.Sprintf("Failed: %s", filepath.Base(sourcePath))
				e.workerStatus.Unlock()
				errorChan <- err
			}
		}
	}
}

