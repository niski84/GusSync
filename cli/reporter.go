package main

import (
	"GusSync/pkg/engine"
	"fmt"
	"os"
	"sort"
)

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

