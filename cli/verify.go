package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// runVerifyMode handles the verify mode: verifies that backed up files match source files
func runVerifyMode(sourceRoot, destRoot string) error {
	// Build state file path from destination (same location as backup mode)
	// For verify, we need to determine which mode was used (mount or adb)
	// Try mount first, then adb
	var stateFile string
	var stateManager *StateManager
	var mode string
	var err error

	// Try mount mode state file
	mountDestPath := filepath.Join(destRoot, "mount")
	mountStateFile := filepath.Join(mountDestPath, stateFileName)
	var adbDestPath string
	if _, statErr := os.Stat(mountStateFile); statErr == nil {
		stateFile = mountStateFile
		stateManager, err = NewStateManager(stateFile)
		if err != nil {
			return fmt.Errorf("failed to open mount state file: %w", err)
		}
		defer stateManager.Close()
		mode = "mount"
	} else {
		// Try adb mode state file
		adbDestPath = filepath.Join(destRoot, "adb")
		adbStateFile := filepath.Join(adbDestPath, stateFileName)
		if _, statErr := os.Stat(adbStateFile); statErr == nil {
			stateFile = adbStateFile
			stateManager, err = NewStateManager(adbStateFile)
			if err != nil {
				return fmt.Errorf("failed to open adb state file: %w", err)
			}
			defer stateManager.Close()
			mode = "adb"
		} else {
			return fmt.Errorf("no state file found in %s/mount or %s/adb", destRoot, destRoot)
		}
	}

	fmt.Printf("GusSync - Verify Mode\n")
	fmt.Printf("Source: %s\n", sourceRoot)
	fmt.Printf("State file: %s\n", stateFile)
	fmt.Printf("Mode: %s\n\n", mode)

	// Determine destination base path
	var destBase string
	if mode == "mount" {
		destBase = mountDestPath
	} else {
		destBase = adbDestPath
	}

	// Get number of workers (use same logic as backup mode)
	numWorkers := runtime.NumCPU()
	if numWorkers > 4 {
		numWorkers = 4 // Cap at 4 for verification
	}

	// Get the appropriate copier for re-copy on mismatch
	var copier Copier
	if mode == "mount" {
		copier = &FSCopier{}
	} else {
		copier = &ADBCopier{}
	}

	// Run verification
	verifyResults := verifyBackup(sourceRoot, destBase, stateManager, numWorkers, mode, copier)

	// Print results
	fmt.Printf("\nVerification complete:\n")
	fmt.Printf("  Files verified: %d\n", verifyResults.verified)
	fmt.Printf("  Files missing in source: %d\n", verifyResults.missingSource)
	fmt.Printf("  Files missing in destination: %d\n", verifyResults.missingDest)
	fmt.Printf("  Hash mismatches: %d\n", verifyResults.mismatches)

	if verifyResults.mismatches > 0 || verifyResults.missingDest > 0 {
		fmt.Printf("\n⚠️  WARNING: Backup verification found issues!\n")
		fmt.Printf("   Check the error output above for details.\n")
		return fmt.Errorf("verification found %d mismatches and %d missing destination files", verifyResults.mismatches, verifyResults.missingDest)
	} else {
		fmt.Printf("\n✅ All files verified successfully!\n")
	}

	return nil
}

