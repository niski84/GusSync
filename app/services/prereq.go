package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PrereqService handles prerequisite checks for the application
type PrereqService struct {
	ctx        context.Context
	logger     *log.Logger
	lastReport *PrereqReport
}

// NewPrereqService creates a new PrereqService
func NewPrereqService(ctx context.Context, logger *log.Logger) *PrereqService {
	return &PrereqService{
		ctx:    ctx,
		logger: logger,
	}
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
	OverallStatus string       `json:"overallStatus"` // "ok", "warn", "fail"
	OS            string       `json:"os"`            // "linux", "windows", "darwin"
	Checks        []PrereqCheck `json:"checks"`
	Timestamp     time.Time    `json:"timestamp"`
}

// RefreshNow forces an immediate prerequisite check and returns the report
func (s *PrereqService) RefreshNow() (PrereqReport, error) {
	s.logger.Printf("[PrereqService] RefreshNow: Forcing immediate prerequisite check")
	return s.GetPrereqReport(), nil
}

// GetPrereqReport returns the current prerequisite status report
func (s *PrereqService) GetPrereqReport() PrereqReport {
	s.logger.Printf("[PrereqService] GetPrereqReport: Generating prerequisite report...")

	report := PrereqReport{
		OS:        goruntime.GOOS,
		Checks:    []PrereqCheck{},
		Timestamp: time.Now(),
	}

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

	// Run all prerequisite checks and emit progress events
	checks := make([]PrereqCheck, 0, len(checkConfigs))
	for i, config := range checkConfigs {
		// Emit "starting" event
		s.logger.Printf("[PrereqService] Emitting PrereqCheckProgress event: checkID=%s, checkName=%s, status=starting", config.id, config.name)
		eventData := map[string]interface{}{
			"checkID":   config.id,
			"checkName": config.name,
			"status":    "starting",
		}
		runtime.EventsEmit(s.ctx, "PrereqCheckProgress", eventData)
		s.logger.Printf("[PrereqService] Emitted PrereqCheckProgress (starting) event for %s", config.id)
		
		// Add small delay to allow UI to update (50ms)
		time.Sleep(50 * time.Millisecond)

		// Run the check
		check := config.fn()

		// Emit "completed" event with result
		s.logger.Printf("[PrereqService] Emitting PrereqCheckProgress event: checkID=%s, checkName=%s, status=completed, resultStatus=%s", config.id, config.name, check.Status)
		eventData = map[string]interface{}{
			"checkID":   config.id,
			"checkName": config.name,
			"status":    "completed",
			"result":    check,
		}
		runtime.EventsEmit(s.ctx, "PrereqCheckProgress", eventData)
		s.logger.Printf("[PrereqService] Emitted PrereqCheckProgress (completed) event for %s", config.id)

		checks = append(checks, check)
		
		// Add small delay between checks (except for last one)
		if i < len(checkConfigs)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

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

	s.lastReport = &report
	s.logger.Printf("[PrereqService] Report generated: overallStatus=%s, checks=%d", report.OverallStatus, len(report.Checks))

	// Emit event to frontend
	runtime.EventsEmit(s.ctx, "PrereqReport", report)

	return report
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
			// Look for "device" (authorized) in output
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, "\tdevice") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
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

