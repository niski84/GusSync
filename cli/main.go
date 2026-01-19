package main

import (
	"GusSync/pkg/engine"
	"GusSync/pkg/state"
	"context"
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
)

func init() {
	flag.StringVar(&sourcePath, "source", "", "Source directory to backup")
	flag.StringVar(&destPath, "dest", "", "Destination directory")
	flag.IntVar(&numWorkers, "workers", 0, "Number of worker threads")
	flag.StringVar(&mode, "mode", "mount", "Backup mode: 'mount', 'adb', 'cleanup', or 'verify'")
}

func main() {
	flag.Parse()

	if sourcePath == "" || destPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -source <src> -dest <dst>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate mode
	if mode != "mount" && mode != "adb" && mode != "cleanup" && mode != "verify" {
		fmt.Fprintf(os.Stderr, "Error: invalid mode '%s'\n", mode)
		os.Exit(1)
	}

	// Update destination path to include mode
	fullDestPath := filepath.Join(destPath, mode)
	if err := os.MkdirAll(fullDestPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create destination directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize state manager
	stateFile := filepath.Join(fullDestPath, stateFileName)
	stateManager, err := state.NewStateManager(stateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create state manager: %v\n", err)
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
		fmt.Println("\nShutdown signal received. Finishing current operations...")
		cancel()
	}()

	// Create reporter
	reporter := NewConsoleReporter(numWorkers)

	// Create and run engine
	cfg := engine.EngineConfig{
		SourcePath: sourcePath,
		DestRoot:   fullDestPath,
		Mode:       mode,
		NumWorkers: numWorkers,
		Reporter:   reporter,
	}

	e := engine.NewEngine(cfg, stateManager)

	fmt.Printf("GusSync - Starting %s\n", mode)
	fmt.Printf("Source: %s\n", sourcePath)
	fmt.Printf("Dest: %s\n", fullDestPath)

	if mode == "verify" {
		results, err := e.VerifyBackup(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nVerification complete:\n")
		fmt.Printf("  Verified: %d\n", results.Verified)
		fmt.Printf("  Missing Source: %d\n", results.MissingSource)
		fmt.Printf("  Missing Destination: %d\n", results.MissingDest)
		fmt.Printf("  Mismatches: %d\n", results.Mismatches)
	} else if mode == "cleanup" {
		results, err := e.RunCleanup(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cleanup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nCleanup complete:\n")
		fmt.Printf("  Deleted: %d\n", results.Deleted)
		fmt.Printf("  Already Deleted: %d\n", results.AlreadyDeleted)
		fmt.Printf("  Failed: %d\n", results.Failed)
		fmt.Printf("  Skipped: %d\n", results.Skipped)
		fmt.Printf("  I/O Errors: %d\n", results.IOErrors)
	} else {
		if err := e.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\nBackup complete!")
	}

	// Error log summary
	errorLogFile := filepath.Join(fullDestPath, "gus_errors.log")
	summary, err := engine.SummarizeErrorLog(errorLogFile)
	if err == nil && summary.TotalErrors > 0 {
		fmt.Printf("\nError Log Summary:\n")
		fmt.Printf("  Total errors: %d\n", summary.TotalErrors)
		fmt.Printf("  Critical errors: %d\n", summary.CriticalErrors)
		fmt.Printf("  Timeouts: %d\n", summary.DirectoryTimeouts)
	}
}
