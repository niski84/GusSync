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
)

// DeviceService handles device discovery and status
type DeviceService struct {
	ctx    context.Context
	logger *log.Logger
}

// NewDeviceService creates a new DeviceService
func NewDeviceService(ctx context.Context, logger *log.Logger) *DeviceService {
	return &DeviceService{
		ctx:    ctx,
		logger: logger,
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
	s.logger.Printf("[DeviceService] GetDeviceStatus: Scanning for devices")

	devices := []DeviceInfo{}

	// Check ADB devices
	if goruntime.GOOS == "linux" || goruntime.GOOS == "windows" || goruntime.GOOS == "darwin" {
		if _, err := exec.LookPath("adb"); err == nil {
			cmd := exec.CommandContext(s.ctx, "adb", "devices")
			output, err := cmd.CombinedOutput()
			if err == nil {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					if strings.Contains(line, "\tdevice") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							devices = append(devices, DeviceInfo{
								ID:        parts[0],
								Name:      "ADB Device",
								Type:      "adb",
								Path:      "/sdcard",
								Connected: true,
							})
						}
					}
				}
			}
		}
	}

	// Check MTP/GVFS mounts (Linux only)
	if goruntime.GOOS == "linux" {
		uid := os.Getuid()
		gvfsBase := filepath.Join("/run/user", fmt.Sprintf("%d", uid))
		gvfsPath := filepath.Join(gvfsBase, "gvfs")
		entries, err := os.ReadDir(gvfsPath)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					name := entry.Name()
					if strings.HasPrefix(name, "mtp:") {
						devices = append(devices, DeviceInfo{
							ID:        name,
							Name:      name,
							Type:      "mtp",
							Path:      filepath.Join(gvfsBase, "gvfs", name),
							Connected: true,
						})
					} else if strings.HasPrefix(name, "gphoto2:") {
						devices = append(devices, DeviceInfo{
							ID:        name,
							Name:      name,
							Type:      "gphoto2",
							Path:      filepath.Join(gvfsBase, "gvfs", name),
							Connected: true,
						})
					}
				}
			}
		}
	}

	return devices, nil
}

