package services

import (
	"context"
	"log"
	"sync"
	"time"
)

// LogService handles log aggregation and export
type LogService struct {
	ctx    context.Context
	logger *log.Logger
	logs   []LogEntry
	mu     sync.Mutex
}

// NewLogService creates a new LogService
func NewLogService(ctx context.Context, logger *log.Logger) *LogService {
	return &LogService{
		ctx:    ctx,
		logger: logger,
		logs:   []LogEntry{},
	}
}

// SetContext updates the service context
func (s *LogService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // "info", "warn", "error"
	Message   string    `json:"message"`
}

// GetRecentLogs returns recent log entries
func (s *LogService) GetRecentLogs(limit int) ([]LogEntry, error) {
	s.logger.Printf("[LogService] GetRecentLogs: limit=%d", limit)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Return last 'limit' entries
	start := 0
	if len(s.logs) > limit {
		start = len(s.logs) - limit
	}

	return s.logs[start:], nil
}

// ExportDiagnostics creates a diagnostics bundle (stub for now)
func (s *LogService) ExportDiagnostics(outputPath string) error {
	s.logger.Printf("[LogService] ExportDiagnostics: outputPath=%s", outputPath)

	// TODO: Implement diagnostics export
	return nil
}
