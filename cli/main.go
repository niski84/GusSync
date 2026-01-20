package main

import (
	"GusSync/pkg/engine"
	"GusSync/pkg/state"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const (
	stateFileName = "gus_state.md"
)

var (
	sourcePath string
	destPath   string
	numWorkers int
	mode       string
	jsonOutput bool
)

func init() {
	flag.StringVar(&sourcePath, "source", "", "Source directory to backup")
	flag.StringVar(&destPath, "dest", "", "Destination directory")
	flag.IntVar(&numWorkers, "workers", 2, "Number of worker threads")
	flag.StringVar(&mode, "mode", "mount", "Backup mode: 'mount', 'adb', 'cleanup', or 'verify'")
	flag.BoolVar(&jsonOutput, "json", false, "Output machine-readable JSON (one event per line)")
}

func main() {
	flag.Parse()

	if sourcePath == "" || destPath == "" {
		if jsonOutput {
			emitJSONError("source and dest are required")
		} else {
			fmt.Fprintf(os.Stderr, "Usage: %s -source <src> -dest <dst> [-json]\n", os.Args[0])
			flag.PrintDefaults()
		}
		os.Exit(1)
	}

	// Validate mode
	if mode != "mount" && mode != "adb" && mode != "cleanup" && mode != "verify" {
		if jsonOutput {
			emitJSONError(fmt.Sprintf("invalid mode '%s'", mode))
		} else {
			fmt.Fprintf(os.Stderr, "Error: invalid mode '%s'\n", mode)
		}
		os.Exit(1)
	}

	// Update destination path to include mode
	fullDestPath := filepath.Join(destPath, mode)
	if err := os.MkdirAll(fullDestPath, 0755); err != nil {
		if jsonOutput {
			emitJSONError(fmt.Sprintf("failed to create destination directory: %v", err))
		} else {
			fmt.Fprintf(os.Stderr, "Error: failed to create destination directory: %v\n", err)
		}
		os.Exit(1)
	}

	// Initialize state manager
	stateFile := filepath.Join(fullDestPath, stateFileName)
	stateManager, err := state.NewStateManager(stateFile)
	if err != nil {
		if jsonOutput {
			emitJSONError(fmt.Sprintf("failed to create state manager: %v", err))
		} else {
			fmt.Fprintf(os.Stderr, "Error: failed to create state manager: %v\n", err)
		}
		os.Exit(1)
	}
	defer stateManager.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if !jsonOutput {
			fmt.Println("\nShutdown signal received. Finishing current operations...")
		}
		cancel()
	}()

	// Create reporter based on output mode
	var reporter engine.ProgressReporter
	var jsonReporter *JSONReporter
	if jsonOutput {
		jsonReporter = NewJSONReporter()
		reporter = jsonReporter
		// Emit start event
		jsonReporter.emit("start", map[string]interface{}{
			"mode":       mode,
			"source":     sourcePath,
			"dest":       fullDestPath,
			"numWorkers": numWorkers,
		})
	} else {
		reporter = NewConsoleReporter(numWorkers)
		fmt.Printf("GusSync - Starting %s\n", mode)
		fmt.Printf("Source: %s\n", sourcePath)
		fmt.Printf("Dest: %s\n", fullDestPath)
	}

	// Create and run engine
	cfg := engine.EngineConfig{
		SourcePath: sourcePath,
		DestRoot:   fullDestPath,
		Mode:       mode,
		NumWorkers: numWorkers,
		Reporter:   reporter,
	}

	e := engine.NewEngine(cfg, stateManager)

	var exitCode int

	if mode == "verify" {
		results, err := e.VerifyBackup(ctx)
		if err != nil {
			if jsonOutput {
				jsonReporter.ReportError(err)
				jsonReporter.EmitComplete(false, err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			}
			exitCode = 1
		} else {
			if jsonOutput {
				jsonReporter.EmitVerifyResults(results)
				jsonReporter.EmitComplete(true, "Verification complete")
			} else {
				fmt.Printf("\nVerification complete:\n")
				fmt.Printf("  Verified: %d\n", results.Verified)
				fmt.Printf("  Missing Source: %d\n", results.MissingSource)
				fmt.Printf("  Missing Destination: %d\n", results.MissingDest)
				fmt.Printf("  Mismatches: %d\n", results.Mismatches)
			}
		}
	} else if mode == "cleanup" {
		results, err := e.RunCleanup(ctx)
		if err != nil {
			if jsonOutput {
				jsonReporter.ReportError(err)
				jsonReporter.EmitComplete(false, err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "Cleanup failed: %v\n", err)
			}
			exitCode = 1
		} else {
			if jsonOutput {
				jsonReporter.EmitCleanupResults(results)
				jsonReporter.EmitComplete(true, "Cleanup complete")
			} else {
				fmt.Printf("\nCleanup complete:\n")
				fmt.Printf("  Deleted: %d\n", results.Deleted)
				fmt.Printf("  Already Deleted: %d\n", results.AlreadyDeleted)
				fmt.Printf("  Failed: %d\n", results.Failed)
				fmt.Printf("  Skipped: %d\n", results.Skipped)
				fmt.Printf("  I/O Errors: %d\n", results.IOErrors)
			}
		}
	} else {
		if err := e.Run(ctx); err != nil {
			if jsonOutput {
				jsonReporter.ReportError(err)
				jsonReporter.EmitComplete(false, err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
			}
			exitCode = 1
		} else {
			if jsonOutput {
				jsonReporter.EmitComplete(true, "Backup complete")
			} else {
				fmt.Println("\nBackup complete!")
			}
		}
	}

	// Error log summary
	errorLogFile := filepath.Join(fullDestPath, "gus_errors.log")
	summary, err := engine.SummarizeErrorLog(errorLogFile)
	if err == nil && summary.TotalErrors > 0 {
		if jsonOutput {
			jsonReporter.EmitErrorSummary(summary)
		} else {
			fmt.Printf("\nError Log Summary:\n")
			fmt.Printf("  Total errors: %d\n", summary.TotalErrors)
			fmt.Printf("  Critical errors: %d\n", summary.CriticalErrors)
			fmt.Printf("  Timeouts: %d\n", summary.DirectoryTimeouts)
		}
	}

	os.Exit(exitCode)
}

// emitJSONError outputs an error in JSON format and exits
func emitJSONError(message string) {
	event := map[string]interface{}{
		"type": "error",
		"data": map[string]string{"message": message},
	}
	json.NewEncoder(os.Stderr).Encode(event)
}
