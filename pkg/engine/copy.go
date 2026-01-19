package engine

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// CopyStats represents statistics for a copy operation
type CopyStats struct {
	Success     bool
	Skipped     bool
	IsTimeout   bool
	BytesCopied int64
}

// ConnectionChecker is a function that checks if the connection is still alive
// Returns error if connection is dead, nil if connection is alive
type ConnectionChecker func() error

// copyWithTimeout copies data with stall detection and progress reporting
func copyWithTimeout(src io.Reader, dst io.Writer, timeout time.Duration, progressChan chan<- int64, connChecker ConnectionChecker) (int64, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track progress atomically
	prog := &progressTracker{
		lastTime:     time.Now(),
		lastBytes:    0,
		progressChan: progressChan,
	}

	// Progress checker and reporter goroutine
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		progressTicker := time.NewTicker(ProgressUpdateInterval)
		connTicker := time.NewTicker(10 * time.Second) // Check connection every 10 seconds
		defer ticker.Stop()
		defer progressTicker.Stop()
		defer connTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-connTicker.C:
				// Check connection health if checker provided
				if connChecker != nil {
					if err := connChecker(); err != nil {
						// Connection is dead - cancel copy
						cancel()
						return
					}
				}
			case <-ticker.C:
				// Check for stalls: if bytes haven't increased in timeout period
				prog.Lock()
				currentBytes := prog.bytes
				timeSinceLastUpdate := time.Since(prog.lastTime)

				// If bytes haven't changed AND enough time has passed, consider it stalled
				if currentBytes == prog.lastBytes && timeSinceLastUpdate > timeout {
					prog.Unlock()
					cancel()
					return
				}

				// Update lastBytes only if bytes have increased (progress made)
				if currentBytes > prog.lastBytes {
					prog.lastBytes = currentBytes
					prog.lastTime = time.Now()
				}
				prog.Unlock()
			case <-progressTicker.C:
				// Report progress
				prog.Lock()
				currentBytes := prog.bytes
				prog.Unlock()
				if progressChan != nil {
					select {
					case progressChan <- currentBytes:
					default:
						// Channel full, skip this update
					}
				}
			}
		}
	}()

	// Wrap source reader to track progress (with context for cancellation)
	progressReader := &progressReader{
		Reader:   src,
		progress: prog,
		ctx:      ctx, // Store context to check in Read()
	}

	// Perform the copy
	var totalBytes int64
	var err error
	totalBytes, err = io.Copy(dst, progressReader)

	done <- true

	// Send final progress update
	if progressChan != nil {
		select {
		case progressChan <- totalBytes:
		default:
		}
	}

	if err != nil {
		return totalBytes, err
	}

	// Check if we were cancelled due to stall or connection loss
	select {
	case <-ctx.Done():
		prog.Lock()
		elapsed := time.Since(prog.lastTime)
		prog.Unlock()
		// Check if it was a connection issue or stall
		if connChecker != nil {
			if err := connChecker(); err != nil {
				return totalBytes, fmt.Errorf("connection lost during copy: %w", err)
			}
		}
		return totalBytes, fmt.Errorf("copy stalled: no progress for %v", elapsed)
	default:
		return totalBytes, nil
	}
}

// progressTracker tracks copy progress for stall detection and reporting
type progressTracker struct {
	lastTime     time.Time
	lastBytes    int64 // Bytes at last check (for stall detection)
	bytes        int64
	progressChan chan<- int64 // Channel to report progress (optional)
	sync.Mutex
}

// progressReader wraps a Reader to track progress
type progressReader struct {
	io.Reader
	progress *progressTracker
	ctx      context.Context // Context to check for cancellation
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	// Check if context is cancelled BEFORE attempting to read
	// This allows us to abort the read immediately when stalled
	select {
	case <-pr.ctx.Done():
		// Context cancelled (stall detected) - return error to abort io.Copy
		// Use "stalled" in error message so worker recognizes it as a timeout
		pr.progress.Lock()
		elapsed := time.Since(pr.progress.lastTime)
		pr.progress.Unlock()
		return 0, fmt.Errorf("copy stalled: no progress for %v", elapsed)
	default:
		// Context not cancelled, proceed with read
	}

	n, err = pr.Reader.Read(p)
	if n > 0 {
		pr.progress.Lock()
		pr.progress.bytes += int64(n)
		pr.progress.lastTime = time.Now()
		// Update lastBytes when we read data (progress is being made)
		pr.progress.lastBytes = pr.progress.bytes
		pr.progress.Unlock()
	}
	return n, err
}
