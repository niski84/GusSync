// Package api provides an HTTP API adapter for GusSync.
// This adapter exposes REST endpoints and SSE event streaming for remote control.
package api

import "GusSync/internal/core"

// APIResponse wraps all API responses with a consistent structure
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JobListResponse contains a list of jobs
type JobListResponse struct {
	Jobs      []*core.JobSnapshot `json:"jobs"`
	ActiveJob string              `json:"activeJob,omitempty"`
}

// StartCopyRequest is the request body for starting a copy operation
type StartCopyRequest struct {
	SourcePath      string `json:"sourcePath,omitempty"`
	DestinationPath string `json:"destinationPath,omitempty"`
	WorkerCount     int    `json:"workerCount,omitempty"`
}

// DeviceInfo represents device information
type DeviceInfo struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	MountPath   string `json:"mountPath,omitempty"`
	Protocol    string `json:"protocol"` // "mtp" or "adb"
}

// DevicesResponse contains device status
type DevicesResponse struct {
	Devices   []DeviceInfo `json:"devices"`
	Connected bool         `json:"connected"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

