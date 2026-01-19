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

// DeviceService handles device discovery and status
type DeviceService struct {
	ctx          context.Context
	logger       *log.Logger
	lastDevices  []DeviceInfo
	isPolling    bool
	pollInterval time.Duration
}

// NewDeviceService creates a new DeviceService
func NewDeviceService(ctx context.Context, logger *log.Logger) *DeviceService {
	return &DeviceService{
		ctx:          ctx,
		logger:       logger,
		pollInterval: 2 * time.Second,
	}
}

// SetContext updates the context for the DeviceService
func (s *DeviceService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// StartPolling starts background polling for devices
func (s *DeviceService) StartPolling(ctx context.Context) {
	if s.isPolling {
		return
	}
	s.isPolling = true
	s.ctx = ctx

	// Do an immediate check before starting the ticker
	go func() {
		s.logger.Printf("[DeviceService] Performing initial device check")
		s.checkAndEmit()
		
		s.logger.Printf("[DeviceService] Starting background device polling (every %v)", s.pollInterval)
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Printf("[DeviceService] Stopping device polling")
				s.isPolling = false
				return
			case <-ticker.C:
				s.checkAndEmit()
			}
		}
	}()
}

// checkAndEmit scans for devices and emits events if status changed
func (s *DeviceService) checkAndEmit() {
	devices, err := s.GetDeviceStatus()
	if err != nil {
		return
	}

	// Check if status changed
	changed := false
	if len(devices) != len(s.lastDevices) {
		changed = true
	} else {
		// Simple ID-based comparison
		for i := range devices {
			if devices[i].ID != s.lastDevices[i].ID {
				changed = true
				break
			}
		}
	}

	if changed {
		s.logger.Printf("[DeviceService] Device status changed: %d devices found", len(devices))
		s.lastDevices = devices
		
		// Emit device:connection (boolean for simple UI)
		runtime.EventsEmit(s.ctx, "device:connection", map[string]interface{}{
			"connected": len(devices) > 0,
			"count":     len(devices),
		})

		// Emit full device list
		runtime.EventsEmit(s.ctx, "device:list", devices)
		
		// Also notify PrereqService if needed (via Event)
		// PrereqService listens for PrereqReport, but we can emit a signal
		// to refresh prerequisites if a device is connected/disconnected
		runtime.EventsEmit(s.ctx, "device:changed", devices)
	}
}

// DeviceInfo represents a discovered device
type DeviceInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "mtp", "adb", "gphoto2"
	Path      string `json:"path"`
	Connected bool   `json:"connected"`
}

// GetDeviceStatus returns the current device connection status
func (s *DeviceService) GetDeviceStatus() ([]DeviceInfo, error) {
	s.logger.Printf("[DeviceService] GetDeviceStatus: Scanning for devices (OS: %s)", goruntime.GOOS)

	devices := []DeviceInfo{}

	// Check ADB devices
	if goruntime.GOOS == "linux" || goruntime.GOOS == "windows" || goruntime.GOOS == "darwin" {
		if _, err := exec.LookPath("adb"); err == nil {
			cmd := exec.CommandContext(s.ctx, "adb", "devices")
			output, err := cmd.CombinedOutput()
			if err == nil {
				s.logger.Printf("[DeviceService] ADB devices output:\n%s", string(output))
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "List of devices") {
						continue
					}
					// Look for authorized device
					if strings.Contains(line, "device") && !strings.Contains(line, "unauthorized") && !strings.Contains(line, "offline") {
						parts := strings.Fields(line)
						if len(parts) >= 2 && parts[1] == "device" {
							s.logger.Printf("[DeviceService] Found ADB device: %s", parts[0])
							devices = append(devices, DeviceInfo{
								ID:        parts[0],
								Name:      "ADB Device (" + parts[0] + ")",
								Type:      "adb",
								Path:      "/sdcard",
								Connected: true,
							})
						}
					}
				}
			} else {
				s.logger.Printf("[DeviceService] ADB devices command failed: %v", err)
			}
		} else {
			s.logger.Printf("[DeviceService] ADB not found in path")
		}
	}

	// Check MTP/GVFS mounts (Linux only)
	if goruntime.GOOS == "linux" {
		uid := os.Getuid()
		gvfsBase := filepath.Join("/run/user", fmt.Sprintf("%d", uid))
		gvfsPath := filepath.Join(gvfsBase, "gvfs")
		s.logger.Printf("[DeviceService] Checking GVFS path: %s", gvfsPath)
		entries, err := os.ReadDir(gvfsPath)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					name := entry.Name()
					if strings.HasPrefix(name, "mtp:") || strings.HasPrefix(name, "gphoto2:") {
						deviceType := "mtp"
						if strings.HasPrefix(name, "gphoto2:") {
							deviceType = "gphoto2"
						}
						
						fullPath := filepath.Join(gvfsPath, name)
						s.logger.Printf("[DeviceService] Found %s device: %s at %s", deviceType, name, fullPath)
						devices = append(devices, DeviceInfo{
							ID:        name,
							Name:      name,
							Type:      deviceType,
							Path:      fullPath,
							Connected: true,
						})
					}
				}
			}
		} else {
			s.logger.Printf("[DeviceService] GVFS path read failed: %v", err)
		}
	}

	s.logger.Printf("[DeviceService] Scan complete: %d devices found", len(devices))
	return devices, nil
}

