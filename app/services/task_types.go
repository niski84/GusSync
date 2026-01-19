package services

import (
	"time"
)

type TaskState string

const (
	TaskQueued    TaskState = "queued"
	TaskRunning   TaskState = "running"
	TaskSucceeded TaskState = "succeeded"
	TaskFailed    TaskState = "failed"
	TaskCanceled  TaskState = "canceled"
)

type TaskProgress struct {
	Phase   string  `json:"phase"`
	Current int64   `json:"current"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
	Rate    float64 `json:"rate"`
}

type TaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

type TaskArtifact struct {
	LogPath     string `json:"logPath"`
	OpenLogHint string `json:"openLogHint"`
}

type TaskSnapshot struct {
	TaskID    string            `json:"taskId"`
	Type      string            `json:"type"`
	State     TaskState         `json:"state"`
	Params    map[string]string `json:"params,omitempty"`
	Progress  TaskProgress      `json:"progress"`
	Message   string            `json:"message"`
	Workers   map[int]string    `json:"workers,omitempty"`
	Error     *TaskError        `json:"error,omitempty"`
	Artifact  TaskArtifact      `json:"artifact"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type TaskUpdateEvent struct {
	TaskID   string            `json:"taskId"`
	Type     string            `json:"type"`
	State    TaskState         `json:"state"`
	Progress TaskProgress      `json:"progress"`
	Message  string            `json:"message"`
	LogLine  string            `json:"logLine,omitempty"`
	Workers  map[int]string    `json:"workers,omitempty"`
	Error    *TaskError        `json:"error,omitempty"`
	Artifact TaskArtifact      `json:"artifact"`
}

