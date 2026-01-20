package main

import (
	"GusSync/pkg/engine"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// ConsoleReporter outputs human-readable progress to the terminal
type ConsoleReporter struct {
	numWorkers int
}

func NewConsoleReporter(numWorkers int) *ConsoleReporter {
	return &ConsoleReporter{numWorkers: numWorkers}
}

func (r *ConsoleReporter) ReportProgress(update engine.ProgressUpdate) {
	// Print summary line
	var statusLine string
	if update.DeltaMB > 0 {
		statusLine = fmt.Sprintf("\r[%d files] Completed: %d | Skipped: %d | Failed: %d | Timeouts: %d (consecutive: %d) | Speed: %.2f MB/s | Delta: %.2f MB",
			update.TotalFiles, update.Completed, update.Skipped, update.Failed, update.TimeoutSkips, update.ConsecutiveSkips, update.Rate/(1024*1024), update.DeltaMB)
	} else {
		statusLine = fmt.Sprintf("\r[%d files] Completed: %d | Skipped: %d | Failed: %d | Timeouts: %d (consecutive: %d) | Speed: %.2f MB/s",
			update.TotalFiles, update.Completed, update.Skipped, update.Failed, update.TimeoutSkips, update.ConsecutiveSkips, update.Rate/(1024*1024))
	}

	fmt.Print(statusLine + "\n")

	// Print worker activity
	// We need to sort worker IDs for consistent output
	ids := make([]int, 0, len(update.WorkerStatuses))
	for id := range update.WorkerStatuses {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		if id < r.numWorkers {
			fmt.Printf("  W%d: %s\n", id, update.WorkerStatuses[id])
		}
	}
}

func (r *ConsoleReporter) ReportError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

func (r *ConsoleReporter) ReportLog(level, message string) {
	fmt.Printf("[%s] %s\n", level, message)
}

// JSONEvent is the structured event format for machine-readable output
type JSONEvent struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// JSONProgressData contains progress information in structured form
type JSONProgressData struct {
	TotalFiles       int            `json:"totalFiles"`
	Completed        int            `json:"completed"`
	Failed           int            `json:"failed"`
	Skipped          int            `json:"skipped"`
	TimeoutSkips     int            `json:"timeoutSkips"`
	ConsecutiveSkips int            `json:"consecutiveSkips"`
	TotalBytes       int64          `json:"totalBytes"`
	RateBytesPerSec  float64        `json:"rateBytesPerSec"`
	RateMBPerSec     float64        `json:"rateMBPerSec"`
	DeltaMB          float64        `json:"deltaMB"`
	ScanComplete     bool           `json:"scanComplete"`
	Workers          map[int]string `json:"workers,omitempty"`
}

// JSONLogData contains log information in structured form
type JSONLogData struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// JSONErrorData contains error information in structured form
type JSONErrorData struct {
	Message string `json:"message"`
}

// JSONReporter outputs machine-readable JSON lines for scripting/automation
type JSONReporter struct {
	encoder *json.Encoder
}

func NewJSONReporter() *JSONReporter {
	return &JSONReporter{
		encoder: json.NewEncoder(os.Stdout),
	}
}

func (r *JSONReporter) emit(eventType string, data interface{}) {
	event := JSONEvent{
		Type:      eventType,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Data:      data,
	}
	r.encoder.Encode(event)
}

func (r *JSONReporter) ReportProgress(update engine.ProgressUpdate) {
	data := JSONProgressData{
		TotalFiles:       update.TotalFiles,
		Completed:        update.Completed,
		Failed:           update.Failed,
		Skipped:          update.Skipped,
		TimeoutSkips:     update.TimeoutSkips,
		ConsecutiveSkips: update.ConsecutiveSkips,
		TotalBytes:       update.TotalBytes,
		RateBytesPerSec:  update.Rate,
		RateMBPerSec:     update.Rate / (1024 * 1024),
		DeltaMB:          update.DeltaMB,
		ScanComplete:     update.ScanComplete,
		Workers:          update.WorkerStatuses,
	}
	r.emit("progress", data)
}

func (r *JSONReporter) ReportError(err error) {
	r.emit("error", JSONErrorData{Message: err.Error()})
}

func (r *JSONReporter) ReportLog(level, message string) {
	r.emit("log", JSONLogData{Level: level, Message: message})
}

// VerifyResultsJSON is the structured output for verify results
type VerifyResultsJSON struct {
	Verified      int `json:"verified"`
	MissingSource int `json:"missingSource"`
	MissingDest   int `json:"missingDest"`
	Mismatches    int `json:"mismatches"`
}

// CleanupResultsJSON is the structured output for cleanup results
type CleanupResultsJSON struct {
	Deleted        int `json:"deleted"`
	AlreadyDeleted int `json:"alreadyDeleted"`
	Failed         int `json:"failed"`
	Skipped        int `json:"skipped"`
	IOErrors       int `json:"ioErrors"`
}

// ErrorSummaryJSON is the structured output for error log summary
type ErrorSummaryJSON struct {
	TotalErrors       int      `json:"totalErrors"`
	CriticalErrors    int      `json:"criticalErrors"`
	DirectoryTimeouts int      `json:"directoryTimeouts"`
	DirectoryErrors   int      `json:"directoryErrors"`
	HashMismatches    int      `json:"hashMismatches"`
	CopyErrors        int      `json:"copyErrors"`
	OtherErrors       int      `json:"otherErrors"`
	TimeoutDirs       []string `json:"timeoutDirs,omitempty"`
	ErrorDirs         []string `json:"errorDirs,omitempty"`
}

// EmitVerifyResults emits verify results as JSON
func (r *JSONReporter) EmitVerifyResults(results engine.VerifyResults) {
	r.emit("verify_complete", VerifyResultsJSON{
		Verified:      results.Verified,
		MissingSource: results.MissingSource,
		MissingDest:   results.MissingDest,
		Mismatches:    results.Mismatches,
	})
}

// EmitCleanupResults emits cleanup results as JSON
func (r *JSONReporter) EmitCleanupResults(results engine.CleanupResults) {
	r.emit("cleanup_complete", CleanupResultsJSON{
		Deleted:        results.Deleted,
		AlreadyDeleted: results.AlreadyDeleted,
		Failed:         results.Failed,
		Skipped:        results.Skipped,
		IOErrors:       results.IOErrors,
	})
}

// EmitErrorSummary emits error log summary as JSON
func (r *JSONReporter) EmitErrorSummary(summary engine.ErrorSummary) {
	r.emit("error_summary", ErrorSummaryJSON{
		TotalErrors:       summary.TotalErrors,
		CriticalErrors:    summary.CriticalErrors,
		DirectoryTimeouts: summary.DirectoryTimeouts,
		DirectoryErrors:   summary.DirectoryErrors,
		HashMismatches:    summary.HashMismatches,
		CopyErrors:        summary.CopyErrors,
		OtherErrors:       summary.OtherErrors,
		TimeoutDirs:       summary.TimeoutDirs,
		ErrorDirs:         summary.ErrorDirs,
	})
}

// EmitComplete emits a completion event
func (r *JSONReporter) EmitComplete(success bool, message string) {
	r.emit("complete", map[string]interface{}{
		"success": success,
		"message": message,
	})
}
