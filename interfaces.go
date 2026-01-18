package main

import "context"

// FileJob represents a file to be processed
type FileJob struct {
	SourcePath string // Full source path
	RelPath    string // Relative path from source root
}

// Scanner interface for discovering files
type Scanner interface {
	// Scan discovers files and sends them to the jobs channel
	// root: the root path to scan
	// jobs: channel to send discovered files
	// errors: channel to send errors
	Scan(ctx context.Context, root string, jobs chan<- FileJob, errors chan<- error)
}

// Copier interface for copying files
type Copier interface {
	// Copy copies a file from source to destination
	// sourcePath: full source path
	// sourceRoot: root of source (for calculating relative paths)
	// destRoot: root of destination
	// progressChan: optional channel for progress updates (bytes copied)
	// Returns: bytes copied, error
	Copy(ctx context.Context, sourcePath, sourceRoot, destRoot string, progressChan chan<- int64) (int64, error)
}

