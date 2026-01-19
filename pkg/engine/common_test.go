package engine

import (
	"testing"
)

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Hidden files
		{".nomedia", true},
		{"some/path/.DS_Store", true},
		{"Thumbs.db", true},
		
		// Extensions
		{"video.exo", true},
		{"app.cache", true},
		{"data.tmp", true},
		{"image.jpg", false},
		
		// Android specific
		{"Android/data/com.google.android.gms/files/test", true},
		{"Android/media/com.whatsapp/WhatsApp/Media/image.jpg", false},
		{"Android/media/com.whatsapp/WhatsApp/Media/random.file", true}, // random extension in media
		
		// Cache paths
		{"some/app/cache/data", true},
		{"Pictures/thumbnails/img.jpg", true},
		
		// Normal files
		{"DCIM/Camera/IMG_2023.jpg", false},
		{"Documents/report.pdf", false},
		{"Download/manual.txt", false},
	}

	for _, tt := range tests {
		result := shouldExcludeFile(tt.path)
		if result != tt.expected {
			t.Errorf("shouldExcludeFile(%q) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestNormalizePhonePath(t *testing.T) {
	root := "/run/user/1000/gvfs/mtp:host=Xiaomi"
	
	tests := []struct {
		source   string
		expected string
	}{
		{root + "/Internal shared storage/DCIM/Camera/test.jpg", "DCIM/Camera/test.jpg"},
		{root + "/SD card/Music/song.mp3", "Music/song.mp3"},
		{root + "/Documents/work.pdf", "Documents/work.pdf"},
	}

	for _, tt := range tests {
		result, err := normalizePhonePath(tt.source, root)
		if err != nil {
			t.Errorf("normalizePhonePath(%q) error: %v", tt.source, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("normalizePhonePath(%q) = %q, expected %q", tt.source, result, tt.expected)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		result := formatSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %q, expected %q", tt.bytes, result, tt.expected)
		}
	}
}

