package services

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CopyService handles copy operations
type CopyService struct {
	ctx           context.Context
	logger        *log.Logger
	jobManager    *JobManager
	deviceService *DeviceService
	config        *ConfigService
	errorLog      *log.Logger
	errorFile     *os.File
	errorMutex    sync.Mutex
}

// NewCopyService creates a new CopyService
func NewCopyService(ctx context.Context, logger *log.Logger, jobManager *JobManager, deviceService *DeviceService) *CopyService {
	return &CopyService{
		ctx:           ctx,
		logger:        logger,
		jobManager:    jobManager,
		deviceService: deviceService,
	}
}

// SetContext sets the Wails runtime context for the service
func (s *CopyService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// SetConfig sets the config service
func (s *CopyService) SetConfig(config *ConfigService) {
	s.config = config
}

// ChooseDestination opens a directory selection dialog
func (s *CopyService) ChooseDestination() (string, error) {
	s.logger.Printf("[CopyService] ChooseDestination: Opening directory dialog")
	
	path, err := runtime.OpenDirectoryDialog(s.ctx, runtime.OpenDialogOptions{
		Title: "Choose Backup Destination",
	})
	if err != nil {
		return "", fmt.Errorf("failed to choose destination: %w", err)
	}
	
	s.logger.Printf("[CopyService] ChooseDestination: Selected path=%s", path)
	
	// Save to config if available
	if s.config != nil && path != "" {
		if err := s.config.SetDestinationPath(path); err != nil {
			s.logger.Printf("[CopyService] ChooseDestination: Failed to save destination to config: %v", err)
		}
	}
	
	return path, nil
}

// StartBackup starts a backup operation (non-blocking)
func (s *CopyService) StartBackup(sourcePath, destPath, mode string) error {
	s.logger.Printf("[CopyService] StartBackup: source=%s dest=%s mode=%s", sourcePath, destPath, mode)

	// Start job
	jobID, jobCtx, err := s.jobManager.StartJob("copy")
	if err != nil {
		return fmt.Errorf("failed to start backup job: %w", err)
	}

	// Emit initial status
	runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     "info",
		"message":   fmt.Sprintf("Starting backup job %s (mode: %s)", jobID, mode),
	})
	runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
		"id":      jobID,
		"state":   "running",
		"message": "Starting backup...",
	})

	// If destination not provided, try config
	if destPath == "" {
		if s.config != nil {
			config := s.config.GetConfig()
			destPath = config.DestinationPath
		}
		// If still empty, prompt user
		if destPath == "" {
			var err error
			destPath, err = s.ChooseDestination()
			if err != nil || destPath == "" {
				s.jobManager.CompleteJob(false)
				runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
					"id":      jobID,
					"state":   "failed",
					"message": "Destination not selected",
				})
				return fmt.Errorf("destination not selected")
			}
		}
	}

	// Validate source path
	if sourcePath == "" {
		// Try to auto-detect from device service
		devices, err := s.deviceService.GetDeviceStatus()
		if err == nil && len(devices) > 0 {
			// Find a device that matches the requested mode
			var selectedDevice *DeviceInfo
			for _, d := range devices {
				if mode == "adb" && d.Type == "adb" {
					selectedDevice = &d
					break
				} else if mode == "mount" && (d.Type == "mtp" || d.Type == "gphoto2") {
					selectedDevice = &d
					break
				}
			}
			
			// If no exact match for mode, just take the first one
			if selectedDevice == nil {
				selectedDevice = &devices[0]
			}
			
			sourcePath = selectedDevice.Path
			s.logger.Printf("[CopyService] StartBackup: Auto-detected source path=%s (Type: %s, ID: %s)", sourcePath, selectedDevice.Type, selectedDevice.ID)
			
			// If we're in mount mode but the auto-detected device is ADB, maybe we should warn?
			// For now, just use it.
		}

		// Fallback for Linux if DeviceService failed or returned nothing
		if sourcePath == "" && goruntime.GOOS == "linux" {
			uid := os.Getuid()
			gvfsBase := filepath.Join("/run/user", fmt.Sprintf("%d", uid), "gvfs")
			entries, err := os.ReadDir(gvfsBase)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() && (strings.HasPrefix(entry.Name(), "mtp") || strings.HasPrefix(entry.Name(), "gphoto2")) {
						sourcePath = filepath.Join(gvfsBase, entry.Name())
						s.logger.Printf("[CopyService] StartBackup: Fallback auto-detected source path=%s", sourcePath)
						break
					}
				}
			}
		}
		
		if sourcePath == "" {
			if mode == "adb" {
				sourcePath = "/sdcard" // Standard Android root
			} else {
				sourcePath = "/mnt/phone" // Default fallback
			}
		}
	}

	s.logger.Printf("[CopyService] StartBackup: Using source=%s dest=%s mode=%s", sourcePath, destPath, mode)

	// Emit job info with paths
	runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
		"id":          jobID,
		"state":       "running",
		"message":     "Backup in progress...",
		"sourcePath":  sourcePath,
		"destPath":    destPath,
		"mode":        mode,
	})

	// Run the backup in a goroutine
	go s.runBackup(jobCtx, jobID, sourcePath, destPath, mode)

	return nil
}

// runBackup executes the actual backup process
func (s *CopyService) runBackup(ctx context.Context, jobID string, sourcePath, destPath, mode string) {
	defer func() {
		if r := recover(); r != nil {
			s.logError("[CopyService] runBackup: Panic recovered in job %s: %v", jobID, r)
			runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
				"timestamp": time.Now().Format(time.RFC3339Nano),
				"level":     "error",
				"message":   fmt.Sprintf("✗ Backup job %s crashed: %v", jobID, r),
			})
			runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
				"id":      jobID,
				"message": fmt.Sprintf("Backup crashed: %v", r),
			})
			runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
				"id":      jobID,
				"state":   "failed",
				"message": fmt.Sprintf("Backup crashed: %v", r),
			})
			s.jobManager.CompleteJob(false)
		}
		s.logger.Printf("[CopyService] runBackup: Job %s finished", jobID)
	}()

	// Get the path to the CLI binary
	var execPath string
	homeDir, _ := os.UserHomeDir()
	projectRoot := filepath.Join(homeDir, "goprojects", "GusSync", "gussync")
	if _, err := os.Stat(projectRoot); err == nil {
		execPath = projectRoot
	} else {
		// Try to find in PATH
		if path, err := exec.LookPath("gussync"); err == nil {
			execPath = path
		} else {
			// Try relative to executable
			if exePath, err := os.Executable(); err == nil {
				exeDir := filepath.Dir(exePath)
				possiblePath := filepath.Join(exeDir, "gussync")
				if _, err := os.Stat(possiblePath); err == nil {
					execPath = possiblePath
				}
			}
		}
	}

	if execPath == "" {
		err := fmt.Errorf("gussync CLI binary not found - please build it first with 'go build -o gussync .'")
		s.logError("[CopyService] runBackup: %v", err)
		runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
			"id":      jobID,
			"message": err.Error(),
		})
		runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
			"id":      jobID,
			"state":   "failed",
			"message": err.Error(),
		})
		s.jobManager.CompleteJob(false)
		return
	}

	// Pass number of workers
	workers := 2
	
	s.logger.Printf("[CopyService] runBackup: Executing CLI: %s -source %s -dest %s -mode %s -workers %d", execPath, sourcePath, destPath, mode, workers)
	s.logError("[CopyService] runBackup: Starting backup job %s - source=%s dest=%s mode=%s execPath=%s workers=%d", jobID, sourcePath, destPath, mode, execPath, workers)
	
	// Emit log line with backup details
	runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     "info",
		"message":   fmt.Sprintf("Backup running: source=%s dest=%s mode=%s workers=%d", sourcePath, destPath, mode, workers),
	})

	cmd := exec.CommandContext(ctx, execPath, "-source", sourcePath, "-dest", destPath, "-mode", mode, "-workers", strconv.Itoa(workers))
	
	// Set up process group for proper signal handling on Unix
	if goruntime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	// Create pipes for stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.logError("[CopyService] runBackup: Failed to create stdout pipe: %v", err)
		runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
			"id":      jobID,
			"message": fmt.Sprintf("Failed to create stdout pipe: %v", err),
		})
		s.jobManager.CompleteJob(false)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.logError("[CopyService] runBackup: Failed to create stderr pipe: %v", err)
		runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
			"id":      jobID,
			"message": fmt.Sprintf("Failed to create stderr pipe: %v", err),
		})
		s.jobManager.CompleteJob(false)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		s.logError("[CopyService] runBackup: Failed to start command: %v", err)
		runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
			"id":      jobID,
			"message": fmt.Sprintf("Failed to start backup: %v", err),
		})
		s.jobManager.CompleteJob(false)
		return
	}

	// Store process PID for cancellation
	pid := cmd.Process.Pid
	var pgid int
	if goruntime.GOOS != "windows" {
		pgid, _ = syscall.Getpgid(pid)
	}
	s.jobManager.SetProcessPID(pid, pgid)

	// Create scanners for output
	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)

	// Stream stdout/stderr to events
	go s.streamOutput(ctx, stdoutScanner, jobID, "stdout")
	go s.streamOutput(ctx, stderrScanner, jobID, "stderr")

	// Wait for command to complete
	err = cmd.Wait()

		// Check if cancelled
	if ctx.Err() != nil {
		s.logger.Printf("[CopyService] runBackup: Job %s was cancelled", jobID)
		runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"level":     "warn",
			"message":   fmt.Sprintf("Backup job %s cancelled by user", jobID),
		})
		runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
			"id":      jobID,
			"state":   "cancelled",
			"message": "Backup cancelled by user",
		})
		s.jobManager.CompleteJob(false)
		return
	}

	if err != nil {
		s.logError("[CopyService] runBackup: Command failed: %v (exit code may be non-zero)", err)
		runtime.EventsEmit(s.ctx, "job:error", map[string]interface{}{
			"id":      jobID,
			"message": fmt.Sprintf("Backup failed: %v", err),
		})
		runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
			"id":      jobID,
			"state":   "failed",
			"message": fmt.Sprintf("Backup failed: %v", err),
		})
		s.jobManager.CompleteJob(false)
		return
	}

	// Success
	s.logger.Printf("[CopyService] runBackup: Job %s completed successfully", jobID)
	runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     "info",
		"message":   fmt.Sprintf("✓ Backup job %s completed successfully", jobID),
	})
	runtime.EventsEmit(s.ctx, "job:status", map[string]interface{}{
		"id":      jobID,
		"state":   "completed",
		"message": "Backup completed successfully",
	})
	s.jobManager.CompleteJob(true)
}

// CancelCopy cancels the current copy operation
func (s *CopyService) CancelCopy() error {
	s.logger.Printf("[CopyService] CancelCopy: Cancelling copy operation")
	return s.jobManager.CancelJob()
}

// streamOutput streams command output to UI events
func (s *CopyService) streamOutput(ctx context.Context, scanner *bufio.Scanner, jobID string, streamType string) {
	for scanner.Scan() {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return
		}

		line := scanner.Text()
		s.logger.Printf("[CopyService] streamOutput [%s]: %s", streamType, line)

		// Parse stats line: [150 files] Completed: 145 | Skipped: 3 | Failed: 2 | Speed: 12.34 MB/s | Delta: 25.67 MB
		if strings.HasPrefix(line, "[") && strings.Contains(line, "files]") {
			s.parseAndEmitStats(line, jobID)
		}

		// Parse worker status line: W0: Copying: filename (X MB/Y MB Z% speed) or W0: idle
		if match := regexp.MustCompile(`^\s*W(\d+):\s+(.+)$`).FindStringSubmatch(line); len(match) > 2 {
			workerID, _ := strconv.Atoi(match[1])
			workerStatus := strings.TrimSpace(match[2])
			s.parseAndEmitWorkerStatus(workerID, workerStatus, jobID)
		}

		// Parse worker status line: Copying: filename (X MB/Y MB Z%) or Copying: filename (X MB)
		if strings.HasPrefix(line, "Copying:") && !strings.HasPrefix(strings.TrimSpace(line), "W") {
			s.parseAndEmitFileProgress(line, jobID)
		}

		// Parse discovery progress: [DEBUG scanDir] Reading directory: /path
		if strings.Contains(line, "[DEBUG scanDir] Reading directory:") {
			parts := strings.SplitN(line, "Reading directory:", 2)
			if len(parts) == 2 {
				dirPath := strings.TrimSpace(parts[1])
				runtime.EventsEmit(s.ctx, "job:discovery", map[string]interface{}{
					"id":      jobID,
					"type":    "directory_scanning",
					"path":    dirPath,
					"message": fmt.Sprintf("Scanning directory: %s", dirPath),
				})
			}
		}

		// Parse directory stats: [DEBUG scanDir] Directory /path: X files, Y subdirectories
		if strings.Contains(line, "[DEBUG scanDir] Directory") && strings.Contains(line, "files") && strings.Contains(line, "subdirectories") {
			if match := regexp.MustCompile(`Directory\s+(.+?):\s+(\d+)\s+files,\s+(\d+)\s+subdirectories`).FindStringSubmatch(line); len(match) > 3 {
				dirPath := match[1]
				filesCount, _ := strconv.Atoi(match[2])
				dirsCount, _ := strconv.Atoi(match[3])
				
				runtime.EventsEmit(s.ctx, "job:discovery", map[string]interface{}{
					"id":         jobID,
					"type":       "directory_stats",
					"path":       dirPath,
					"filesFound": filesCount,
					"dirsFound":  dirsCount,
				})
			}
		}

		// Parse discovered files count: discoveredCount from scanner: X files
		if strings.Contains(line, "discoveredCount") && strings.Contains(line, "files") {
			if match := regexp.MustCompile(`discoveredCount.*?(\d+)\s+files`).FindStringSubmatch(line); len(match) > 1 {
				count, _ := strconv.Atoi(match[1])
				runtime.EventsEmit(s.ctx, "job:discovery", map[string]interface{}{
					"id":         jobID,
					"type":       "total_discovered",
					"filesCount": count,
				})
			}
		}

		// Parse directory discovery summary
		if strings.Contains(line, "Fully scanned:") {
			if match := regexp.MustCompile(`Fully scanned:\s+(\d+)\s+directories`).FindStringSubmatch(line); len(match) > 1 {
				completedDirs, _ := strconv.Atoi(match[1])
				runtime.EventsEmit(s.ctx, "job:discovery", map[string]interface{}{
					"id":            jobID,
					"type":          "discovery_complete",
					"completedDirs": completedDirs,
				})
			}
		}

		// Emit raw log line for general output
		runtime.EventsEmit(s.ctx, "job:log", map[string]interface{}{
			"id":      jobID,
			"stream":  streamType,
			"message": line,
		})
	}

	if err := scanner.Err(); err != nil {
		s.logError("[CopyService] streamOutput: Scanner error for %s: %v", streamType, err)
	}
}

// parseAndEmitStats parses stats line and emits structured progress event
func (s *CopyService) parseAndEmitStats(line string, jobID string) {
	// Example: [150 files] Completed: 145 | Skipped: 3 | Failed: 2 | Speed: 12.34 MB/s | Delta: 25.67 MB
	stats := make(map[string]interface{})
	stats["id"] = jobID

	// Extract total files: [150 files]
	if match := regexp.MustCompile(`\[(\d+)\s+files\]`).FindStringSubmatch(line); len(match) > 1 {
		stats["totalFiles"] = parseFloatOrInt(match[1])
	}

	// Extract Completed: X
	if match := regexp.MustCompile(`Completed:\s+(\d+)`).FindStringSubmatch(line); len(match) > 1 {
		stats["filesCompleted"] = parseFloatOrInt(match[1])
	}

	// Extract Skipped: X
	if match := regexp.MustCompile(`Skipped:\s+(\d+)`).FindStringSubmatch(line); len(match) > 1 {
		stats["filesSkipped"] = parseFloatOrInt(match[1])
	}

	// Extract Failed: X
	if match := regexp.MustCompile(`Failed:\s+(\d+)`).FindStringSubmatch(line); len(match) > 1 {
		stats["filesFailed"] = parseFloatOrInt(match[1])
	}

	// Extract Speed: X.XX MB/s or X.XX KB/s
	if match := regexp.MustCompile(`Speed:\s+([\d.]+)\s+(MB/s|KB/s|B/s)`).FindStringSubmatch(line); len(match) > 2 {
		speedValue := parseFloat(match[1])
		unit := match[2]
		stats["speed"] = speedValue
		stats["speedUnit"] = unit
		// Convert to MB/s for consistent progress calculation
		if unit == "KB/s" {
			stats["speedMBps"] = speedValue / 1024.0
		} else if unit == "MB/s" {
			stats["speedMBps"] = speedValue
		} else {
			stats["speedMBps"] = speedValue / (1024.0 * 1024.0)
		}
	}

	// Extract Delta: X.XX MB (if present) - this is incremental MB in last interval
	if match := regexp.MustCompile(`Delta:\s+([\d.]+)\s+MB`).FindStringSubmatch(line); len(match) > 1 {
		stats["deltaMB"] = parseFloat(match[1])
	}

	// Track cumulative MB copied (we'll accumulate deltaMB across events)
	// Note: This assumes we're tracking state, but for now we'll rely on DeltaMB for UI updates
	// The UI can accumulate DeltaMB to get total MB copied

	// Calculate progress percentage (completed / total) - file-based fallback
	if totalFiles, ok := stats["totalFiles"].(float64); ok && totalFiles > 0 {
		if completed, ok := stats["filesCompleted"].(float64); ok {
			// Use float for more precision in progress bar
			stats["progressFiles"] = (completed / totalFiles) * 100.0
		}
	}
	
	// Progress based on MB will be calculated in UI when we have total MB discovered

	// Emit structured progress event
	runtime.EventsEmit(s.ctx, "job:progress", stats)
}

// parseAndEmitFileProgress parses worker status line and emits file progress event
func (s *CopyService) parseAndEmitFileProgress(line string, jobID string) {
	// Example: Copying: filename (X MB/Y MB Z%) or Copying: filename (X MB)
	fileData := make(map[string]interface{})
	fileData["id"] = jobID

	// Extract filename
	if match := regexp.MustCompile(`Copying:\s+(.+?)\s+\(`).FindStringSubmatch(line); len(match) > 1 {
		fileData["currentFile"] = strings.TrimSpace(match[1])
	}

	// Extract progress: (X MB/Y MB Z%)
	if match := regexp.MustCompile(`\(([\d.]+)\s+MB/([\d.]+)\s+MB\s+([\d.]+)%\)`).FindStringSubmatch(line); len(match) > 3 {
		fileData["fileBytesCopied"] = parseFloat(match[1]) * 1024 * 1024 // Convert MB to bytes
		fileData["fileBytesTotal"] = parseFloat(match[2]) * 1024 * 1024
		fileData["fileProgress"] = parseFloat(match[3])
	} else if match := regexp.MustCompile(`\(([\d.]+)\s+MB\)`).FindStringSubmatch(line); len(match) > 1 {
		// No total size available
		fileData["fileBytesCopied"] = parseFloat(match[1]) * 1024 * 1024
		fileData["fileProgress"] = 0
	}

	// Emit file progress event
	runtime.EventsEmit(s.ctx, "job:file", fileData)
}

// parseAndEmitWorkerStatus parses worker status line and emits worker status event
func (s *CopyService) parseAndEmitWorkerStatus(workerID int, status string, jobID string) {
	workerData := make(map[string]interface{})
	workerData["id"] = jobID
	workerData["workerID"] = workerID

	// Log the raw status for debugging
	s.logger.Printf("[CopyService] parseAndEmitWorkerStatus: W%d status=%s", workerID, status)

	// Parse status types
	if status == "idle" || status == "" {
		workerData["status"] = "idle"
		workerData["fileName"] = ""
		workerData["progress"] = 0
		workerData["speed"] = ""
		workerData["message"] = "Waiting for tasks..."
	} else if strings.HasPrefix(status, "Copying:") {
		workerData["status"] = "copying"
		
		// 1. Try to extract: Copying: filename (X unit/Y unit Z% speed)
		// Updated regex to handle any unit (B, KB, MB, GB, etc.) and varied spacing
		fullRegex := regexp.MustCompile(`Copying:\s+(.+)\s+\(([\d.]+)\s*([KMGTPE]?B)?\s*/\s*([\d.]+)\s*([KMGTPE]?B)?\s+([\d.]+)%\s*(.+?)?\)`)
		if match := fullRegex.FindStringSubmatch(status); len(match) > 6 {
			fileName := strings.TrimSpace(match[1])
			bytesCopied := parseSizeToBytes(match[2], match[3])
			bytesTotal := parseSizeToBytes(match[4], match[5])
			percent := parseFloat(match[6])
			speed := ""
			if len(match) > 7 {
				speed = strings.TrimSpace(match[7])
			}
			
			workerData["fileName"] = fileName
			workerData["bytesCopied"] = bytesCopied
			workerData["bytesTotal"] = bytesTotal
			workerData["progress"] = percent
			workerData["speed"] = speed
			workerData["fileSize"] = formatSizeBytes(bytesTotal)
		} else {
			// 2. Try simpler version: Copying: filename (X unit speed)
			simpleRegex := regexp.MustCompile(`Copying:\s+(.+)\s+\(([\d.]+)\s*([KMGTPE]?B)?\s*(.+?)?\)`)
			if match := simpleRegex.FindStringSubmatch(status); len(match) > 3 {
				fileName := strings.TrimSpace(match[1])
				bytesCopied := parseSizeToBytes(match[2], match[3])
				speed := strings.TrimSpace(match[4])
				
				workerData["fileName"] = fileName
				workerData["bytesCopied"] = bytesCopied
				workerData["bytesTotal"] = 0
				workerData["progress"] = 0
				workerData["speed"] = speed
			} else {
				// Fallback: just filename
				workerData["fileName"] = strings.TrimSpace(strings.TrimPrefix(status, "Copying:"))
				workerData["message"] = "Active"
			}
		}
	} else if strings.HasPrefix(status, "Starting:") {
		workerData["status"] = "starting"
		// Regex for: Starting: filename (size unit)
		startingRegex := regexp.MustCompile(`Starting:\s+(.+?)\s+\((.+?)\)`)
		if match := startingRegex.FindStringSubmatch(status); len(match) > 2 {
			workerData["fileName"] = strings.TrimSpace(match[1])
			workerData["fileSize"] = strings.TrimSpace(match[2])
		} else {
			workerData["fileName"] = strings.TrimSpace(strings.TrimPrefix(status, "Starting:"))
		}
	} else if strings.HasPrefix(status, "Failed:") {
		workerData["status"] = "failed"
		workerData["fileName"] = strings.TrimSpace(strings.TrimPrefix(status, "Failed:"))
	} else {
		// Fallback for any other status (e.g. "Hashing...", "Verifying...")
		workerData["status"] = "active"
		workerData["message"] = status
	}

	// Emit worker status event
	runtime.EventsEmit(s.ctx, "job:worker", workerData)
}

// parseSizeToBytes converts value + unit (e.g. "1.5", "MB") to bytes
func parseSizeToBytes(valStr, unit string) float64 {
	val := parseFloat(valStr)
	unit = strings.ToUpper(strings.TrimSpace(unit))
	
	switch unit {
	case "B":
		return val
	case "KB":
		return val * 1024
	case "MB":
		return val * 1024 * 1024
	case "GB":
		return val * 1024 * 1024 * 1024
	case "TB":
		return val * 1024 * 1024 * 1024 * 1024
	default:
		// Default to MB if unit is missing but common in our CLI
		return val * 1024 * 1024
	}
}

// formatSizeBytes formats float64 bytes as human-readable size
func formatSizeBytes(bytes float64) string {
	if bytes <= 0 {
		return ""
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%.0f B", bytes)
	}
	div, exp := float64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", bytes/div, "KMGTPE"[exp])
}

// Helper functions
func parseFloatOrInt(s string) float64 {
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}
	return 0
}

func parseFloat(s string) float64 {
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}
	return 0
}

// getErrorLogPath returns the path to the error log file
func (s *CopyService) getErrorLogPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	logDir := filepath.Join(homeDir, ".gussync", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return ""
	}
	return filepath.Join(logDir, "errors.log")
}

// logError logs an error to both stderr and the error log file
func (s *CopyService) logError(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	s.logger.Printf("%s", message)

	// Also log to dedicated error file
	s.errorMutex.Lock()
	defer s.errorMutex.Unlock()

	if s.errorFile == nil {
		logPath := s.getErrorLogPath()
		if logPath == "" {
			return
		}
		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			s.logger.Printf("[CopyService] logError: Failed to open error log file: %v", err)
			return
		}
		s.errorFile = file
		s.errorLog = log.New(file, "", log.LstdFlags)
	}

	if s.errorLog != nil {
		s.errorLog.Printf("%s", message)
		s.errorFile.Sync() // Flush immediately
	}
}

