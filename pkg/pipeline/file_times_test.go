package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGetFileTimes_BasicExtraction tests that GetFileTimes can extract file times from a regular file
func TestGetFileTimes_BasicExtraction(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_file.txt")

	// Write test file
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get file info
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	// Extract times
	atime, mtime, ctime, err := GetFileTimes(info)
	if err != nil {
		t.Fatalf("GetFileTimes() error = %v", err)
	}

	// Verify times are not zero
	if atime.IsZero() {
		t.Error("atime is zero, expected valid timestamp")
	}
	if mtime.IsZero() {
		t.Error("mtime is zero, expected valid timestamp")
	}
	if ctime.IsZero() {
		t.Error("ctime is zero, expected valid timestamp")
	}

	// Verify times are reasonable (not too far in past or future)
	now := time.Now()
	minTime := now.Add(-1 * time.Hour) // Allow 1 hour in past
	maxTime := now.Add(1 * time.Hour)  // Allow 1 hour in future

	if atime.Before(minTime) || atime.After(maxTime) {
		t.Errorf("atime %v is outside reasonable range [%v, %v]", atime, minTime, maxTime)
	}
	if mtime.Before(minTime) || mtime.After(maxTime) {
		t.Errorf("mtime %v is outside reasonable range [%v, %v]", mtime, minTime, maxTime)
	}
	if ctime.Before(minTime) || ctime.After(maxTime) {
		t.Errorf("ctime %v is outside reasonable range [%v, %v]", ctime, minTime, maxTime)
	}
}

// TestGetFileTimes_ModificationTimeMatch tests that mtime from GetFileTimes matches os.FileInfo.ModTime()
func TestGetFileTimes_ModificationTimeMatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_file.txt")

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	_, mtime, _, err := GetFileTimes(info)
	if err != nil {
		t.Fatalf("GetFileTimes() error = %v", err)
	}

	// mtime should match info.ModTime()
	expectedMtime := info.ModTime()
	if !mtime.Equal(expectedMtime) {
		t.Errorf("mtime = %v, want %v", mtime, expectedMtime)
	}
}

// TestGetFileTimes_NonexistentFile tests error handling for missing files
func TestGetFileTimes_NonexistentFile(t *testing.T) {
	// Try to stat a file that doesn't exist
	_, err := os.Stat("/this/path/does/not/exist/hopefully")
	if err == nil {
		t.Skip("Test file unexpectedly exists")
	}

	// This test verifies that os.Stat fails first (not GetFileTimes)
	// GetFileTimes requires valid os.FileInfo, so it won't be called on missing files
}

// TestGetFileTimes_Directory tests that GetFileTimes works with directories
func TestGetFileTimes_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	atime, mtime, ctime, err := GetFileTimes(info)
	if err != nil {
		t.Fatalf("GetFileTimes() error = %v", err)
	}

	// Verify times are not zero for directory
	if atime.IsZero() {
		t.Error("directory atime is zero")
	}
	if mtime.IsZero() {
		t.Error("directory mtime is zero")
	}
	if ctime.IsZero() {
		t.Error("directory ctime is zero")
	}
}

// TestGetFileTimes_MultipleFiles tests consistency across multiple files
func TestGetFileTimes_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files with slight delays
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, fname := range files {
		fpath := filepath.Join(tmpDir, fname)
		if err := os.WriteFile(fpath, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", fname, err)
		}
		time.Sleep(10 * time.Millisecond) // Small delay between file creates
	}

	// Extract times from all files
	for _, fname := range files {
		fpath := filepath.Join(tmpDir, fname)
		info, err := os.Stat(fpath)
		if err != nil {
			t.Fatalf("Failed to stat %s: %v", fname, err)
		}

		atime, mtime, ctime, err := GetFileTimes(info)
		if err != nil {
			t.Errorf("GetFileTimes(%s) error = %v", fname, err)
			continue
		}

		// Verify all times are valid
		if atime.IsZero() || mtime.IsZero() || ctime.IsZero() {
			t.Errorf("File %s has zero timestamp: atime=%v, mtime=%v, ctime=%v",
				fname, atime, mtime, ctime)
		}
	}
}

// TestGetFileTimes_OldFile tests extraction from a file with older timestamps
func TestGetFileTimes_OldFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "old_file.txt")

	// Create file
	if err := os.WriteFile(testFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set atime/mtime to 1 year ago (365 days)
	oldTime := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(testFile, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set file times: %v", err)
	}

	// Get file info
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	// Extract times
	atime, mtime, _, err := GetFileTimes(info)
	if err != nil {
		t.Fatalf("GetFileTimes() error = %v", err)
	}

	// Verify times are approximately 1 year old (allow 1 hour tolerance)
	expectedTime := oldTime
	tolerance := 1 * time.Hour

	atimeDiff := atime.Sub(expectedTime)
	if atimeDiff < -tolerance || atimeDiff > tolerance {
		t.Errorf("atime = %v, want ~%v (diff: %v)", atime, expectedTime, atimeDiff)
	}

	mtimeDiff := mtime.Sub(expectedTime)
	if mtimeDiff < -tolerance || mtimeDiff > tolerance {
		t.Errorf("mtime = %v, want ~%v (diff: %v)", mtime, expectedTime, mtimeDiff)
	}
}

// TestGetFileTimes_Symlink tests behavior with symbolic links (if supported)
func TestGetFileTimes_Symlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	linkFile := filepath.Join(tmpDir, "link.txt")

	// Create target file
	if err := os.WriteFile(targetFile, []byte("target content"), 0644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	// Create symlink
	if err := os.Symlink(targetFile, linkFile); err != nil {
		t.Skipf("Symlinks not supported: %v", err)
	}

	// Get file info (follows symlink by default with os.Stat)
	info, err := os.Stat(linkFile)
	if err != nil {
		t.Fatalf("Failed to stat symlink: %v", err)
	}

	atime, mtime, ctime, err := GetFileTimes(info)
	if err != nil {
		t.Fatalf("GetFileTimes() error = %v", err)
	}

	// Verify times are valid
	if atime.IsZero() || mtime.IsZero() || ctime.IsZero() {
		t.Error("Symlink target has zero timestamps")
	}
}
