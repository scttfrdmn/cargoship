package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCalculateOptimalWorkers_SmallWorkload tests worker calculation for small workloads (<100 files)
func TestCalculateOptimalWorkers_SmallWorkload(t *testing.T) {
	tests := []struct {
		name      string
		fileCount int64
		totalSize int64
		want      WorkerCounts
	}{
		{"1 file", 1, 1024, WorkerCounts{1, 2, 2}},
		{"50 files", 50, 50 * 1024, WorkerCounts{1, 2, 2}},
		{"99 files", 99, 99 * 1024, WorkerCounts{1, 2, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOptimalWorkers(tt.fileCount, tt.totalSize)
			if got != tt.want {
				t.Errorf("CalculateOptimalWorkers() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCalculateOptimalWorkers_LightWorkload tests worker calculation for light workloads (100-1000 files)
func TestCalculateOptimalWorkers_LightWorkload(t *testing.T) {
	tests := []struct {
		name      string
		fileCount int64
		totalSize int64
		want      WorkerCounts
	}{
		{"100 files", 100, 100 * 1024, WorkerCounts{2, 4, 2}},
		{"500 files", 500, 500 * 1024, WorkerCounts{2, 4, 2}},
		{"999 files", 999, 999 * 1024, WorkerCounts{2, 4, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOptimalWorkers(tt.fileCount, tt.totalSize)
			if got != tt.want {
				t.Errorf("CalculateOptimalWorkers() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCalculateOptimalWorkers_MediumWorkload tests worker calculation for medium workloads (1000-10000 files)
func TestCalculateOptimalWorkers_MediumWorkload(t *testing.T) {
	tests := []struct {
		name      string
		fileCount int64
		totalSize int64
		want      WorkerCounts
	}{
		{"1000 files", 1000, 1000 * 1024, WorkerCounts{4, 8, 4}},
		{"5000 files", 5000, 5000 * 1024, WorkerCounts{4, 8, 4}},
		{"9999 files", 9999, 9999 * 1024, WorkerCounts{4, 8, 4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOptimalWorkers(tt.fileCount, tt.totalSize)
			if got != tt.want {
				t.Errorf("CalculateOptimalWorkers() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCalculateOptimalWorkers_LargeWorkload tests worker calculation for large workloads (>10k files)
func TestCalculateOptimalWorkers_LargeWorkload(t *testing.T) {
	cores := runtime.NumCPU()

	tests := []struct {
		name      string
		fileCount int64
		totalSize int64
	}{
		{"10000 files", 10000, 10000 * 1024},
		{"50000 files", 50000, 50000 * 1024},
		{"100000 files", 100000, 100000 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOptimalWorkers(tt.fileCount, tt.totalSize)

			// Scanner should be cores/2, capped at 8
			expectedScanner := min(cores/2, 8)
			if got.Scanner != expectedScanner {
				t.Errorf("Scanner workers = %d, want %d (cores=%d)", got.Scanner, expectedScanner, cores)
			}

			// Archiver should be cores, capped at 16
			expectedArchiver := min(cores, 16)
			if got.Archiver != expectedArchiver {
				t.Errorf("Archiver workers = %d, want %d (cores=%d)", got.Archiver, expectedArchiver, cores)
			}

			// Uploader should be cores/2, capped at 8
			expectedUploader := min(cores/2, 8)
			if got.Uploader != expectedUploader {
				t.Errorf("Uploader workers = %d, want %d (cores=%d)", got.Uploader, expectedUploader, cores)
			}
		})
	}
}

// TestCalculateOptimalWorkers_MaxLimits tests that worker counts respect maximum limits
func TestCalculateOptimalWorkers_MaxLimits(t *testing.T) {
	// Use a very large file count to ensure we hit the CPU-scaled path
	got := CalculateOptimalWorkers(1000000, 1000000*1024)

	if got.Scanner > 8 {
		t.Errorf("Scanner workers = %d, exceeds max of 8", got.Scanner)
	}

	if got.Archiver > 16 {
		t.Errorf("Archiver workers = %d, exceeds max of 16", got.Archiver)
	}

	if got.Uploader > 8 {
		t.Errorf("Uploader workers = %d, exceeds max of 8", got.Uploader)
	}
}

// TestCalculateOptimalWorkers_ZeroFiles tests handling of empty directories
func TestCalculateOptimalWorkers_ZeroFiles(t *testing.T) {
	got := CalculateOptimalWorkers(0, 0)

	// With 0 files, should use minimum workers
	want := WorkerCounts{1, 2, 2}

	if got != want {
		t.Errorf("CalculateOptimalWorkers(0, 0) = %+v, want %+v", got, want)
	}
}

// TestEstimateWorkload tests the workload estimation function
func TestEstimateWorkload(t *testing.T) {
	// Create a temporary test directory
	testDir := t.TempDir()

	// Create some test files
	testFiles := []struct {
		name string
		size int64
	}{
		{"file1.txt", 1024},
		{"file2.txt", 2048},
		{"file3.txt", 4096},
	}

	for _, tf := range testFiles {
		path := filepath.Join(testDir, tf.name)
		data := make([]byte, tf.size)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Estimate workload
	ctx := context.Background()
	fileCount, totalSize, err := EstimateWorkload(ctx, testDir)

	if err != nil {
		t.Fatalf("EstimateWorkload() error = %v", err)
	}

	// Verify counts
	expectedCount := int64(len(testFiles))
	if fileCount != expectedCount {
		t.Errorf("fileCount = %d, want %d", fileCount, expectedCount)
	}

	// Verify total size
	var expectedSize int64
	for _, tf := range testFiles {
		expectedSize += tf.size
	}
	if totalSize != expectedSize {
		t.Errorf("totalSize = %d, want %d", totalSize, expectedSize)
	}
}

// TestEstimateWorkload_SkipsHiddenFiles tests that hidden files are skipped
func TestEstimateWorkload_SkipsHiddenFiles(t *testing.T) {
	// Create a temporary test directory
	testDir := t.TempDir()

	// Create regular and hidden files
	regularFile := filepath.Join(testDir, "regular.txt")
	hiddenFile := filepath.Join(testDir, ".hidden.txt")

	if err := os.WriteFile(regularFile, []byte("regular"), 0644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}
	if err := os.WriteFile(hiddenFile, []byte("hidden"), 0644); err != nil {
		t.Fatalf("Failed to create hidden file: %v", err)
	}

	// Estimate workload
	ctx := context.Background()
	fileCount, _, err := EstimateWorkload(ctx, testDir)

	if err != nil {
		t.Fatalf("EstimateWorkload() error = %v", err)
	}

	// Should only count the regular file
	if fileCount != 1 {
		t.Errorf("fileCount = %d, want 1 (hidden file should be skipped)", fileCount)
	}
}

// TestEstimateWorkload_EmptyDirectory tests handling of empty directories
func TestEstimateWorkload_EmptyDirectory(t *testing.T) {
	// Create an empty temporary directory
	testDir := t.TempDir()

	// Estimate workload
	ctx := context.Background()
	fileCount, totalSize, err := EstimateWorkload(ctx, testDir)

	if err != nil {
		t.Fatalf("EstimateWorkload() error = %v", err)
	}

	// Should return 0 for empty directory
	if fileCount != 0 {
		t.Errorf("fileCount = %d, want 0", fileCount)
	}
	if totalSize != 0 {
		t.Errorf("totalSize = %d, want 0", totalSize)
	}
}

// TestEstimateWorkload_NonexistentDirectory tests error handling for missing directory
func TestEstimateWorkload_NonexistentDirectory(t *testing.T) {
	// Use a path that doesn't exist
	nonexistentPath := "/this/path/does/not/exist/hopefully"

	// Estimate workload
	ctx := context.Background()
	_, _, err := EstimateWorkload(ctx, nonexistentPath)

	// Should return an error
	if err == nil {
		t.Error("EstimateWorkload() expected error for nonexistent directory, got nil")
	}
}

// TestEstimateWorkload_CancellationContext tests context cancellation
func TestEstimateWorkload_CancellationContext(t *testing.T) {
	// Create a temporary test directory with many files
	testDir := t.TempDir()

	// Create many files to ensure there's time to cancel
	for i := 0; i < 100; i++ {
		path := filepath.Join(testDir, filepath.Base(t.TempDir()))
		data := make([]byte, 1024)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Estimate workload with cancelled context
	_, _, err := EstimateWorkload(ctx, testDir)

	// Should return context cancellation error
	if err != context.Canceled {
		t.Errorf("EstimateWorkload() with cancelled context error = %v, want %v", err, context.Canceled)
	}
}

// TestEstimateWorkload_NestedDirectories tests handling of nested directory structures
func TestEstimateWorkload_NestedDirectories(t *testing.T) {
	// Create a temporary test directory
	testDir := t.TempDir()

	// Create nested directory structure
	subDir := filepath.Join(testDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create files in both root and subdirectory
	rootFile := filepath.Join(testDir, "root.txt")
	subFile := filepath.Join(subDir, "sub.txt")

	if err := os.WriteFile(rootFile, []byte("root"), 0644); err != nil {
		t.Fatalf("Failed to create root file: %v", err)
	}
	if err := os.WriteFile(subFile, []byte("sub"), 0644); err != nil {
		t.Fatalf("Failed to create sub file: %v", err)
	}

	// Estimate workload
	ctx := context.Background()
	fileCount, _, err := EstimateWorkload(ctx, testDir)

	if err != nil {
		t.Fatalf("EstimateWorkload() error = %v", err)
	}

	// Should count files in both root and subdirectory
	if fileCount != 2 {
		t.Errorf("fileCount = %d, want 2", fileCount)
	}
}
