package main

import (
	"path/filepath"
	"strings"
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


