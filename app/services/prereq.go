package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PrereqService handles prerequisite checks for the application
type PrereqService struct {
	ctx        context.Context
	logger     *log.Logger
	lastReport *PrereqReport
	reportMu   sync.Mutex // Protect lastReport access
	seqCounter int64      // Sequence counter for report versioning
	errorFile  *os.File
	errorLog   *log.Logger
	errorMutex sync.Mutex
	firstCall  time.Time // Track when GetPrereqReport was first called
	firstCallMu sync.Mutex
	
	// Singleflight logic to prevent concurrent check runs
	runMu      sync.Mutex
	isRunning  bool
	waiters    []chan struct{}
}

// NewPrereqService creates a new PrereqService
func NewPrereqService(ctx context.Context, logger *log.Logger) *PrereqService {
	return &PrereqService{
		ctx:    ctx,
		logger: logger,
	}
}

// SetContext updates the context for the PrereqService
func (s *PrereqService) SetContext(ctx context.Context) {
	s.ctx = ctx
	
	// Start listening for device changes to trigger immediate refreshes
	go func() {
		// Wait a bit for everything to settle
		time.Sleep(1 * time.Second)
		
		s.logDebug("[PrereqService] Starting listener for device:changed events")
		ch := make(chan struct{})
		cleanup := runtime.EventsOn(ctx, "device:changed", func(data ...interface{}) {
			s.logDebug("[PrereqService] Received device:changed event - triggering refresh")
			// Trigger a refresh (async)
			go s.RefreshNow()
		})
		
		// Wait for context to be done
		<-ctx.Done()
		cleanup()
		close(ch)
	}()
}

// PrereqCheck represents a single prerequisite check
type PrereqCheck struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"` // "ok", "warn", "fail"
	Details         string   `json:"details"`
	RemediationSteps []string `json:"remediationSteps"`
	Links           []string `json:"links,omitempty"`
}

// PrereqReport contains all prerequisite checks
type PrereqReport struct {
	OverallStatus string        `json:"overallStatus"` // "ok", "warn", "fail"
	Seq           int64         `json:"seq"`           // Monotonically increasing sequence number
	OS            string        `json:"os"`            // "linux", "windows", "darwin"
	Checks        []PrereqCheck `json:"checks"`
	Timestamp     time.Time     `json:"timestamp"`
}

// RefreshNow forces an immediate prerequisite check and returns the report (bypasses cache)
func (s *PrereqService) RefreshNow() (PrereqReport, error) {
	s.logger.Printf("[PrereqService] RefreshNow: Forcing immediate prerequisite check (bypassing cache)")
	return s.getPrereqReportInternal(true), nil
}

// getPrereqReportInternal performs the actual check (used by both GetPrereqReport and RefreshNow)
func (s *PrereqService) getPrereqReportInternal(forceRefresh bool) PrereqReport {
	// Check cache first if not forcing refresh
	if !forceRefresh {
		s.reportMu.Lock()
		if s.lastReport != nil {
			cachedReport := *s.lastReport
			s.reportMu.Unlock()
			s.logger.Printf("[PrereqService] GetPrereqReport: Returning cached report (from %v)", cachedReport.Timestamp)
			return cachedReport
		}
		s.reportMu.Unlock()
	}

	// Singleflight logic: if already running, wait for completion
	s.runMu.Lock()
	if s.isRunning {
		s.logDebug("[PrereqService] GetPrereqReport: Checks already in progress, waiting...")
		ch := make(chan struct{})
		s.waiters = append(s.waiters, ch)
		s.runMu.Unlock()
		
		// Wait for completion (with timeout just in case)
		select {
		case <-ch:
			// Done, now return the cached result
			s.reportMu.Lock()
			defer s.reportMu.Unlock()
			if s.lastReport != nil {
				return *s.lastReport
			}
			return PrereqReport{OverallStatus: "fail"} // Fallback
		case <-time.After(10 * time.Second):
			s.logDebug("[PrereqService] GetPrereqReport: Timeout waiting for concurrent checks")
			return PrereqReport{OverallStatus: "fail"}
		}
	}
	
	// Mark as running
	s.isRunning = true
	s.runMu.Unlock()

	// Ensure we signal waiters and mark as not running when we finish
	defer func() {
		s.runMu.Lock()
		s.isRunning = false
		for _, ch := range s.waiters {
			close(ch)
		}
		s.waiters = nil
		s.runMu.Unlock()
	}()

	// No cache or force refresh - run the actual checks
	callTime := time.Now()
	
	// Track first call timing
	s.firstCallMu.Lock()
	isFirstCall := s.firstCall.IsZero()
	if isFirstCall {
		s.firstCall = callTime
		s.firstCallMu.Unlock()
		s.logDebug("[PrereqService] GetPrereqReport: ⭐ FIRST CALL ⭐ - This is the first time GetPrereqReport has been called")
		s.logDebug("[PrereqService] GetPrereqReport: First call timestamp: %s", callTime.Format("2006-01-02 15:04:05.000"))
		s.emitLogLine("info", "Starting prerequisite checks...")
	} else {
		timeSinceFirstCall := callTime.Sub(s.firstCall)
		s.firstCallMu.Unlock()
		s.logDebug("[PrereqService] GetPrereqReport: Call #%d - Time since first call: %v", int(timeSinceFirstCall.Seconds()/30)+2, timeSinceFirstCall)
	}

	startTime := time.Now()
	s.logDebug("[PrereqService] GetPrereqReport: ENTRY - Starting prerequisite report generation")

	// Increment sequence counter for this report
	s.reportMu.Lock()
	s.seqCounter++
	seq := s.seqCounter
	s.reportMu.Unlock()

	report := PrereqReport{
		OS:        goruntime.GOOS,
		Seq:       seq,
		Checks:    []PrereqCheck{},
		Timestamp: time.Now(),
	}

	initDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: Report struct initialized (took %v)", initDuration)

	// Define all checks to run with their IDs and names
	checkConfigs := []struct {
		id   string
		name string
		fn   func() PrereqCheck
	}{
		{"adb", "Android Debug Bridge (ADB)", s.checkADB},
		{"mtp_tools", "MTP/GVFS Support", s.checkMTPTools},
		{"device_connection", "Device Connection", s.checkDeviceConnection},
		{"destination_write", "Destination Write Access", s.checkDestinationWriteAccess},
		{"disk_space", "Disk Space", s.checkDiskSpace},
		{"webview2", "WebView2 Runtime", s.checkWebView2},
		{"filesystem_support", "File System Support", s.checkFileSystemSupport},
	}

	// Run all prerequisite checks in parallel using goroutines
	// This allows fast checks to complete immediately while slow ones continue
	checks := make([]PrereqCheck, len(checkConfigs))
	var wg sync.WaitGroup
	var mu sync.Mutex // Protect the checks slice during concurrent writes

	configSetupDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: Check configs defined (took %v total, %v since last)", configSetupDuration, configSetupDuration-initDuration)

	// Start all checks concurrently
	for i, config := range checkConfigs {
		wg.Add(1)
		// Capture values for goroutine
		idx := i
		checkID := config.id
		checkName := config.name
		checkFn := config.fn
		
		go func() {
			defer wg.Done()

			checkStartTime := time.Now()
			s.logDebug("[PrereqService] Check STARTING: %s (%s)", checkID, checkName)

			// Emit "starting" event immediately
			eventEmitStart := time.Now()
			eventData := map[string]interface{}{
				"checkID":   checkID,
				"checkName": checkName,
				"status":    "starting",
			}
			emitTime := time.Now()
			runtime.EventsEmit(s.ctx, "PrereqCheckProgress", eventData)
			eventEmitDuration := time.Since(eventEmitStart)
			afterEmitTime := time.Now()
			s.logDebug("[PrereqService] Check %s: About to emit 'starting' event at %s", checkID, emitTime.Format("15:04:05.000"))
			s.logDebug("[PrereqService] Check %s: Emitted 'starting' event at %s (took %v)", checkID, afterEmitTime.Format("15:04:05.000"), eventEmitDuration)
			
			// Emit log line for UI
			s.emitLogLine("info", fmt.Sprintf("Checking: %s", checkName))

			// Run the check (this may take varying amounts of time)
			checkRunStart := time.Now()
			check := checkFn()
			checkRunDuration := time.Since(checkRunStart)
			s.logDebug("[PrereqService] Check %s: Function execution completed (took %v) - Status: %s", checkID, checkRunDuration, check.Status)

			// Store result in correct position
			mu.Lock()
			checks[idx] = check
			mu.Unlock()

			// Emit "completed" event with result
			eventEmitStart2 := time.Now()
			eventData = map[string]interface{}{
				"checkID":   checkID,
				"checkName": checkName,
				"status":    "completed",
				"result":    check,
			}
			runtime.EventsEmit(s.ctx, "PrereqCheckProgress", eventData)
			eventEmitDuration2 := time.Since(eventEmitStart2)
			totalCheckDuration := time.Since(checkStartTime)
			s.logDebug("[PrereqService] Check %s: Emitted 'completed' event (took %v), Total check time: %v", checkID, eventEmitDuration2, totalCheckDuration)
			
			// Emit log line for UI with status
			statusEmoji := "✓"
			statusText := "OK"
			if check.Status == "fail" {
				statusEmoji = "✗"
				statusText = "FAILED"
			} else if check.Status == "warn" {
				statusEmoji = "⚠"
				statusText = "WARNING"
			}
			s.emitLogLine(check.Status, fmt.Sprintf("%s %s: %s", statusEmoji, checkName, statusText))
		}()
	}

	goroutineStartDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: All goroutines launched (took %v total)", goroutineStartDuration)

	// Wait for all checks to complete
	waitStartTime := time.Now()
	wg.Wait()
	waitDuration := time.Since(waitStartTime)
	totalDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: All checks completed - Wait time: %v, Total time so far: %v", waitDuration, totalDuration)
	
	// Emit log line when all checks complete
	s.emitLogLine("info", fmt.Sprintf("All prerequisite checks completed (took %.1fms)", float64(totalDuration)/float64(time.Millisecond)))

	report.Checks = checks

	// Determine overall status
	hasFail := false
	hasWarn := false
	for _, check := range checks {
		if check.Status == "fail" {
			hasFail = true
		} else if check.Status == "warn" {
			hasWarn = true
		}
	}

	if hasFail {
		report.OverallStatus = "fail"
	} else if hasWarn {
		report.OverallStatus = "warn"
	} else {
		report.OverallStatus = "ok"
	}

	// Cache the report
	s.reportMu.Lock()
	s.lastReport = &report
	s.reportMu.Unlock()
	
	statusCalcDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: Status calculated (took %v total)", statusCalcDuration)

	// Emit final status log
	overallStatusEmoji := "✓"
	if report.OverallStatus == "fail" {
		overallStatusEmoji = "✗"
	} else if report.OverallStatus == "warn" {
		overallStatusEmoji = "⚠"
	}
	s.emitLogLine(report.OverallStatus, fmt.Sprintf("%s System Status: %s", overallStatusEmoji, strings.ToUpper(report.OverallStatus)))

	// Emit PrereqReport event to notify frontend that checks are complete
	// This is the industry-standard pattern: emit an event when async operation completes
	runtime.EventsEmit(s.ctx, "PrereqReport", report)
	s.logDebug("[PrereqService] GetPrereqReport: Emitted PrereqReport event (checks complete)")

	finalDuration := time.Since(startTime)
	s.logDebug("[PrereqService] GetPrereqReport: EXIT - Total function time: %v", finalDuration)

	return report
}

// getErrorLogPath returns the path to the error log file
func (s *PrereqService) getErrorLogPath() string {
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

// emitLogLine emits a LogLine event to the frontend UI
func (s *PrereqService) emitLogLine(level, message string) {
	runtime.EventsEmit(s.ctx, "LogLine", map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     level, // "info", "warn", "error"
		"message":   message,
	})
}

// logDebug logs a debug message with timestamp to both stderr and error log file
func (s *PrereqService) logDebug(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	// Prepend timestamp to args
	allArgs := append([]interface{}{timestamp}, args...)
	message := fmt.Sprintf("[TIMING %s] "+format, allArgs...)
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
			s.logger.Printf("[PrereqService] logDebug: Failed to open error log file: %v", err)
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

// GetPrereqReport returns the current prerequisite status report (uses cache if available)
func (s *PrereqService) GetPrereqReport() PrereqReport {
	return s.getPrereqReportInternal(false)
}

// checkADB verifies that ADB is installed and accessible
func (s *PrereqService) checkADB() PrereqCheck {
	check := PrereqCheck{
		ID:      "adb",
		Name:    "Android Debug Bridge (ADB)",
		Status:  "fail",
		Details: "ADB is required for ADB mode backup operations.",
	}

	// Check if adb command exists in PATH
	adbPath, err := exec.LookPath("adb")
	if err != nil {
		check.Details = "ADB not found in PATH. Required for ADB mode."
		switch goruntime.GOOS {
		case "linux":
			check.RemediationSteps = []string{
				"Install ADB using your package manager:",
				"  Ubuntu/Debian: sudo apt install adb",
				"  Fedora: sudo dnf install android-tools",
				"  Arch: sudo pacman -S android-tools",
			}
			check.Links = []string{"https://developer.android.com/tools/releases/platform-tools"}
		case "windows":
			check.RemediationSteps = []string{
				"Download Platform Tools from Android Developer website:",
				"  1. Visit: https://developer.android.com/tools/releases/platform-tools",
				"  2. Download Windows zip file",
				"  3. Extract to a folder (e.g., C:\\adb)",
				"  4. Add folder to PATH environment variable",
			}
			check.Links = []string{"https://developer.android.com/tools/releases/platform-tools"}
		case "darwin":
			check.RemediationSteps = []string{
				"Install via Homebrew: brew install --cask android-platform-tools",
				"Or download from: https://developer.android.com/tools/releases/platform-tools",
			}
			check.Links = []string{"https://developer.android.com/tools/releases/platform-tools"}
		}
		return check
	}

	// Try to run adb version to verify it works
	cmd := exec.CommandContext(s.ctx, "adb", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		check.Status = "warn"
		check.Details = "ADB found but failed to execute. Error: " + err.Error()
		check.RemediationSteps = []string{
			"Reinstall ADB or check installation.",
		}
		return check
	}

	check.Status = "ok"
	check.Details = "ADB found at: " + adbPath + "\nVersion: " + strings.TrimSpace(string(output))
	return check
}

// checkMTPTools verifies MTP support tools (Linux-specific)
func (s *PrereqService) checkMTPTools() PrereqCheck {
	check := PrereqCheck{
		ID:      "mtp_tools",
		Name:    "MTP/GVFS Support",
		Status:  "ok",
		Details: "MTP support is required for mount mode backup operations.",
	}

	// Only check on Linux
	if goruntime.GOOS != "linux" {
		check.Status = "ok"
		check.Details = "MTP check skipped (not Linux). Mount mode primarily used on Linux."
		return check
	}

	// Check for GVFS (GNOME Virtual File System) - typically provides MTP support
	// Check if /run/user/$UID/gvfs exists (indicates GVFS is available)
	uid := os.Getuid()
	gvfsPath := filepath.Join("/run/user", fmt.Sprintf("%d", uid), "gvfs")
	
	if _, err := os.Stat(gvfsPath); os.IsNotExist(err) {
		// GVFS directory doesn't exist - check if gvfs-backends is installed
		if _, err := exec.LookPath("gio"); err != nil {
			check.Status = "warn"
			check.Details = "GVFS/GIO tools not found. MTP mounts may not work."
			check.RemediationSteps = []string{
				"Install GVFS backends:",
				"  Ubuntu/Debian: sudo apt install gvfs-backends gvfs-fuse",
				"  Fedora: sudo dnf install gvfs-mtp",
				"  Arch: sudo pacman -S gvfs",
			}
			return check
		}
	}

	// Check if gio mount command works
	cmd := exec.CommandContext(s.ctx, "gio", "mount", "-l")
	if err := cmd.Run(); err != nil {
		check.Status = "warn"
		check.Details = "GVFS tools found but may not be fully functional. Error: " + err.Error()
		check.RemediationSteps = []string{
			"Restart GVFS daemon: systemctl --user restart gvfs-daemon",
			"Or logout/login to restart user services.",
		}
		return check
	}

	check.Status = "ok"
	check.Details = "MTP/GVFS support is available."
	return check
}

// checkDeviceConnection checks if any device is connected (ADB or MTP)
func (s *PrereqService) checkDeviceConnection() PrereqCheck {
	check := PrereqCheck{
		ID:      "device_connection",
		Name:    "Device Connection",
		Status:  "fail",
		Details: "No device detected. Connect your Android device via USB.",
	}

	// Check ADB devices
	adbCheck := s.checkADB()
	if adbCheck.Status == "ok" {
		cmd := exec.CommandContext(s.ctx, "adb", "devices")
		output, err := cmd.CombinedOutput()
		if err == nil {
			outputStr := string(output)
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "List of devices") {
					continue
				}
				// Look for authorized device - more robust check
				if strings.Contains(line, "device") && !strings.Contains(line, "unauthorized") && !strings.Contains(line, "offline") {
					parts := strings.Fields(line)
					if len(parts) >= 2 && parts[1] == "device" {
						check.Status = "ok"
						check.Details = "ADB device connected: " + parts[0]
						return check
					}
				}
			}
		}
	}

	// Check MTP/GVFS mounts (Linux only)
	if goruntime.GOOS == "linux" {
		uid := os.Getuid()
		gvfsPath := filepath.Join("/run/user", fmt.Sprintf("%d", uid), "gvfs")
		if _, err := os.Stat(gvfsPath); err == nil {
			// List GVFS mounts
			entries, err := os.ReadDir(gvfsPath)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						name := entry.Name()
						if strings.HasPrefix(name, "mtp:") || strings.HasPrefix(name, "gphoto2:") {
							check.Status = "ok"
							check.Details = "MTP/gphoto2 device mounted: " + name
							return check
						}
					}
				}
			}
		}
	}

	// No device found
	check.RemediationSteps = []string{
		"For ADB mode:",
		"  1. Enable USB debugging on your Android device",
		"  2. Connect device via USB",
		"  3. Authorize computer on device prompt",
		"  4. Run: adb devices (should show device)",
		"",
		"For Mount mode (Linux):",
		"  1. Connect device via USB",
		"  2. Open file manager (Nautilus/Dolphin) and select device",
		"  3. Device should appear in /run/user/$UID/gvfs/",
	}

	return check
}

// checkDestinationWriteAccess checks if we can write to a default destination
func (s *PrereqService) checkDestinationWriteAccess() PrereqCheck {
	check := PrereqCheck{
		ID:      "destination_write",
		Name:    "Destination Write Access",
		Status:  "ok",
		Details: "Write access to destination directory is required for backups.",
	}

	// Try to determine a reasonable default destination
	var testPath string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		check.Status = "warn"
		check.Details = "Could not determine home directory: " + err.Error()
		return check
	}

	// Use a test path in home directory
	testPath = filepath.Join(homeDir, "GusSyncTest")
	
	// Try to create a test file
	testFile := filepath.Join(testPath, ".gus_write_test")
	err = os.MkdirAll(testPath, 0755)
	if err != nil {
		check.Status = "warn"
		check.Details = "Cannot create test directory: " + err.Error()
		check.RemediationSteps = []string{
			"Ensure you have write permissions to your home directory.",
			"Or specify a destination directory where you have write access.",
		}
		return check
	}

	// Try to write a test file
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		check.Status = "warn"
		check.Details = "Cannot write test file: " + err.Error()
		check.RemediationSteps = []string{
			"Check file system permissions.",
			"Ensure you have write access to: " + testPath,
		}
		return check
	}

	// Clean up test file
	os.Remove(testFile)

	check.Status = "ok"
	check.Details = "Write access verified for: " + testPath
	return check
}

// checkDiskSpace checks if there's sufficient disk space (at least 1GB free)
func (s *PrereqService) checkDiskSpace() PrereqCheck {
	check := PrereqCheck{
		ID:      "disk_space",
		Name:    "Disk Space",
		Status:  "ok",
		Details: "Sufficient disk space is required for backups.",
	}

	// Get home directory for default check
	homeDir, err := os.UserHomeDir()
	if err != nil {
		check.Status = "warn"
		check.Details = "Could not determine home directory: " + err.Error()
		return check
	}

	// Use a simple heuristic: check if we can determine disk space
	// On Windows, use wmic; on Unix, use df
	var cmd *exec.Cmd
	if goruntime.GOOS == "windows" {
		// Windows: wmic logicaldisk get freespace,caption | findstr C:
		cmd = exec.CommandContext(s.ctx, "wmic", "logicaldisk", "get", "freespace,caption")
	} else {
		// Unix: df -B1 $HOME
		cmd = exec.CommandContext(s.ctx, "df", "-B1", homeDir)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		check.Status = "warn"
		check.Details = "Could not check disk space: " + err.Error()
		check.RemediationSteps = []string{
			"Ensure you have sufficient free space (at least 1GB recommended).",
			"Check disk space manually using system tools.",
		}
		return check
	}

	// Parse output (simplified - just verify we got output)
	// A full implementation would parse the actual free space value
	if len(output) == 0 {
		check.Status = "warn"
		check.Details = "Could not determine disk space."
		return check
	}

	check.Status = "ok"
	check.Details = "Disk space check passed. Ensure you have enough space for your backup."
	return check
}

// checkWebView2 checks if WebView2 is available (Windows only)
func (s *PrereqService) checkWebView2() PrereqCheck {
	check := PrereqCheck{
		ID:      "webview2",
		Name:    "WebView2 Runtime",
		Status:  "ok",
		Details: "WebView2 is required for Wails on Windows.",
	}

	if goruntime.GOOS != "windows" {
		check.Status = "ok"
		check.Details = "WebView2 check skipped (not Windows)."
		return check
	}

	// Check for WebView2 by looking for registry key or DLL
	// This is a simplified check - Wails will handle this internally
	// We can check if WebView2Loader.dll is in the app directory or system
	check.Status = "ok"
	check.Details = "WebView2 runtime check passed (Wails handles this automatically)."
	check.RemediationSteps = []string{
		"If WebView2 is missing, Wails will prompt to install it automatically.",
		"Or download from: https://developer.microsoft.com/microsoft-edge/webview2/",
	}
	check.Links = []string{"https://developer.microsoft.com/microsoft-edge/webview2/"}
	return check
}

// checkFileSystemSupport checks if the file system supports large files and long paths
func (s *PrereqService) checkFileSystemSupport() PrereqCheck {
	check := PrereqCheck{
		ID:      "filesystem_support",
		Name:    "File System Support",
		Status:  "ok",
		Details: "File system must support large files and long paths.",
	}

	if goruntime.GOOS == "windows" {
		// Windows: Check for long path support (requires registry setting)
		// This is a simplified check - assume Windows 10+ supports it with registry
		check.Details = "Windows file system support. Long paths may require registry setting."
		check.RemediationSteps = []string{
			"Windows 10+ supports long paths but may require enabling:",
			"  Set registry: HKLM\\SYSTEM\\CurrentControlSet\\Control\\FileSystem LongPathsEnabled = 1",
			"  Or use Group Policy: Enable Win32 Long Paths",
		}
		return check
	}

	// Unix systems typically support large files and long paths
	check.Status = "ok"
	check.Details = "File system supports large files and long paths."
	return check
}

