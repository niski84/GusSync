package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PriorityPaths are common Android paths that should be processed first
// These are typical locations for photos, documents, and important user data
var PriorityPaths = []string{
	"DCIM",                    // Camera photos and videos
	"Camera",                  // Camera folder (some devices)
	"Pictures",                // Pictures folder
	"Documents",               // Documents folder
	"Download",                // Downloads
	"Movies",                  // Videos
	"Music",                   // Music files
	"Videos",                  // Videos folder
	"ScreenRecordings",        // Screen recordings
	"Screenshots",             // Screenshots
	"WhatsApp/Media",          // WhatsApp media
	"Android/media",           // App media (contains WhatsApp, etc.)
	"Android/data",            // App data
}

const (
	// StallTimeout is the duration to wait for bytes before considering a transfer stalled
	StallTimeout = 30 * time.Second
	// BufferSize for copying
	BufferSize = 64 * 1024 // 64KB
	// ProgressUpdateInterval is how often to report progress
	ProgressUpdateInterval = 2 * time.Second
)

// shouldExcludeFile determines if a file should be excluded from backup
// Returns true if the file should be skipped
func shouldExcludeFile(normalizedPath string) bool {
	// Extract file extension
	ext := strings.ToLower(filepath.Ext(normalizedPath))
	// Remove leading dot
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	
	// Extract base filename for pattern matching
	baseName := strings.ToLower(filepath.Base(normalizedPath))
	dirPath := strings.ToLower(filepath.Dir(normalizedPath))
	fullPathLower := strings.ToLower(normalizedPath)
	
	// 1. Hidden metadata files (exact matches)
	if baseName == ".nomedia" || baseName == ".ds_store" || baseName == "thumbs.db" {
		return true
	}
	
	// 2. File extensions to exclude
	excludedExts := map[string]bool{
		"exo": true, "cache": true, "tmp": true, "partial": true,
		"download": true, "crdownload": true, "dash": true, "m4s": true,
		"fmp4": true, "db": true, "db-wal": true, "db-shm": true,
		"journal": true, "log": true, "temp.mp4": true,
		"transcoded": true, "encoded": true, "working": true,
		"part": true, "aria2": true, "torrent": true, "resume": true,
		"stacktrace": true, "crash": true, "anr": true, "tombstone": true,
	}
	
	// Check extension
	if excludedExts[ext] {
		return true
	}
	
	// Special case: .temp.mp4 (check if filename ends with this)
	if strings.HasSuffix(baseName, ".temp.mp4") {
		return true
	}
	
	// 3. File patterns (thumbnails, cache images)
	if strings.HasPrefix(baseName, "thumb_") && ext == "jpg" {
		return true
	}
	if strings.HasSuffix(baseName, ".cache.jpg") {
		return true
	}
	
	// 4. Directory exclusions (check path patterns)
	// Android/data/** (entire directory - skip all)
	if strings.HasPrefix(dirPath, "android/data") {
		// EXCEPTION: Android/media/** should be included (still filtered by extension)
		if strings.HasPrefix(fullPathLower, "android/media/") {
			// Allow Android/media, but still apply extension filter
			// Will be filtered by allowlist below
		} else {
			// Skip all Android/data except Android/media
			return true
		}
	}
	
	// Android/obb/** (skip all)
	if strings.HasPrefix(dirPath, "android/obb") {
		return true
	}
	
	// Cache directories
	if strings.Contains(fullPathLower, "/cache/") ||
		strings.Contains(fullPathLower, "/code_cache/") ||
		strings.Contains(fullPathLower, "/files/cache/") ||
		strings.Contains(fullPathLower, "/thumbnails/") ||
		strings.Contains(fullPathLower, "/files/crash/") ||
		strings.Contains(fullPathLower, "/download/.cache/") {
		return true
	}
	
	// Specific Google Play Services paths
	if strings.HasPrefix(fullPathLower, "android/data/com.google.android.gms/") ||
		strings.HasPrefix(fullPathLower, "android/data/com.android.vending/") ||
		strings.HasPrefix(fullPathLower, "android/data/com.google.android.gms.policy_sidecar_aps/") {
		return true
	}
	
	// 5. Extension allowlist for Android/media (if we got here and path is Android/media)
	// Only allow specific media/document extensions
	if strings.HasPrefix(fullPathLower, "android/media/") {
		allowedExts := map[string]bool{
			// Images
			"jpg": true, "jpeg": true, "png": true, "heic": true, "webp": true,
			// Video
			"mp4": true, "mov": true, "mkv": true, "avi": true, "webm": true,
			// Audio
			"mp3": true, "flac": true, "wav": true, "m4a": true, "aac": true,
			// Documents
			"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
			"txt": true, "md": true,
		}
		
		// If extension not in allowlist, exclude it
		if ext == "" || !allowedExts[ext] {
			return true
		}
	}
	
	return false
}

// calculateFileHash computes SHA256 hash of a file
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// normalizePhonePath extracts the actual phone path from protocol-specific mount paths
// Returns the logical path on the phone, protocol-agnostic
func normalizePhonePath(sourcePath, sourceRoot string) (string, error) {
	// Calculate relative path from source root
	relPath, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return "", err
	}

	// Protocol-specific path prefixes to strip:
	if strings.HasPrefix(relPath, "Internal shared storage/") {
		relPath = strings.TrimPrefix(relPath, "Internal shared storage/")
	} else if strings.HasPrefix(relPath, "SD card/") {
		relPath = strings.TrimPrefix(relPath, "SD card/")
	}

	return relPath, nil
}

// formatSize formats bytes as human-readable size
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// CopyResult represents the result of a copy operation
type CopyResult struct {
	Success     bool
	SourceHash  string
	DestHash    string
	BytesCopied int64
	Error       error
}

// RobustCopy copies a file with stall detection and hash verification
func RobustCopy(sourcePath, sourceRoot, destRoot string, progressChan chan<- int64) *CopyResult {
	result := &CopyResult{}

	normalizedPath, err := normalizePhonePath(sourcePath, sourceRoot)
	if err != nil {
		result.Error = fmt.Errorf("failed to normalize phone path: %w", err)
		return result
	}

	destPath := filepath.Join(destRoot, normalizedPath)
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create dest dir: %w", err)
		return result
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to open source: %w", err)
		return result
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to create dest: %w", err)
		return result
	}
	defer destFile.Close()

	result.BytesCopied, result.Error = copyWithTimeout(sourceFile, destFile, StallTimeout, progressChan, nil)
	if result.Error != nil {
		return result
	}

	if err := destFile.Sync(); err != nil {
		result.Error = fmt.Errorf("failed to sync dest: %w", err)
		return result
	}

	result.SourceHash, err = calculateFileHash(sourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to hash source: %w", err)
		return result
	}

	result.DestHash, err = calculateFileHash(destPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to hash dest: %w", err)
		return result
	}

	if result.SourceHash != result.DestHash {
		result.Error = fmt.Errorf("hash mismatch: source=%s, dest=%s", result.SourceHash, result.DestHash)
		return result
	}

	result.Success = true
	return result
}


