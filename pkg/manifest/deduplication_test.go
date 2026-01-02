package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileDeduplicationIndex(t *testing.T) {
	index := NewFileDeduplicationIndex()
	if index == nil {
		t.Fatal("Expected non-nil index")
	}
	if index.Size() != 0 {
		t.Errorf("Expected empty index, got size %d", index.Size())
	}
}

func TestAddFile_UniqueFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file for the first time
	isDup, loc, err := index.AddFile(testFile, 0, 0, "s3://bucket/chunk-0")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	if isDup {
		t.Error("Expected file to not be duplicate on first add")
	}
	if loc != nil {
		t.Error("Expected nil location for unique file")
	}

	// Verify stats
	stats := index.GetStats()
	if stats.TotalFiles != 1 {
		t.Errorf("Expected TotalFiles=1, got %d", stats.TotalFiles)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("Expected UniqueFiles=1, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateFiles != 0 {
		t.Errorf("Expected DuplicateFiles=0, got %d", stats.DuplicateFiles)
	}
	if stats.BytesSaved != 0 {
		t.Errorf("Expected BytesSaved=0, got %d", stats.BytesSaved)
	}
}

func TestAddFile_DuplicateFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("duplicate content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file first time
	isDup1, _, err := index.AddFile(testFile, 0, 0, "s3://bucket/shard-0/chunk-0")
	if err != nil {
		t.Fatalf("Failed to add file first time: %v", err)
	}
	if isDup1 {
		t.Error("Expected file to not be duplicate on first add")
	}

	// Add same file second time (simulating duplicate in different shard)
	isDup2, loc2, err := index.AddFile(testFile, 1, 1, "s3://bucket/shard-1/chunk-1")
	if err != nil {
		t.Fatalf("Failed to add file second time: %v", err)
	}
	if !isDup2 {
		t.Error("Expected file to be duplicate on second add")
	}
	if loc2 == nil {
		t.Fatal("Expected non-nil location for duplicate file")
	}

	// Verify duplicate references original location
	if loc2.ShardID != 0 {
		t.Errorf("Expected duplicate to reference shard 0, got %d", loc2.ShardID)
	}
	if loc2.ChunkID != 0 {
		t.Errorf("Expected duplicate to reference chunk 0, got %d", loc2.ChunkID)
	}
	if loc2.S3Key != "s3://bucket/shard-0/chunk-0" {
		t.Errorf("Expected duplicate to reference original S3 key, got %s", loc2.S3Key)
	}
	if loc2.RefCount != 2 {
		t.Errorf("Expected RefCount=2, got %d", loc2.RefCount)
	}

	// Verify stats
	stats := index.GetStats()
	if stats.TotalFiles != 2 {
		t.Errorf("Expected TotalFiles=2, got %d", stats.TotalFiles)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("Expected UniqueFiles=1, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateFiles != 1 {
		t.Errorf("Expected DuplicateFiles=1, got %d", stats.DuplicateFiles)
	}
	if stats.BytesSaved != int64(len(content)) {
		t.Errorf("Expected BytesSaved=%d, got %d", len(content), stats.BytesSaved)
	}
}

func TestAddFile_MultipleDuplicates(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("shared content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add same file 5 times (simulating 5 copies in different locations)
	for i := 0; i < 5; i++ {
		isDup, loc, err := index.AddFile(testFile, i, i, "s3://bucket/chunk")
		if err != nil {
			t.Fatalf("Failed to add file iteration %d: %v", i, err)
		}
		if i == 0 {
			if isDup {
				t.Errorf("Iteration %d: Expected file to not be duplicate", i)
			}
		} else {
			if !isDup {
				t.Errorf("Iteration %d: Expected file to be duplicate", i)
			}
			if loc.RefCount != int32(i+1) {
				t.Errorf("Iteration %d: Expected RefCount=%d, got %d", i, i+1, loc.RefCount)
			}
		}
	}

	// Verify stats
	stats := index.GetStats()
	if stats.TotalFiles != 5 {
		t.Errorf("Expected TotalFiles=5, got %d", stats.TotalFiles)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("Expected UniqueFiles=1, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateFiles != 4 {
		t.Errorf("Expected DuplicateFiles=4, got %d", stats.DuplicateFiles)
	}

	// 4 duplicates × file size
	expectedBytesSaved := int64(len(content)) * 4
	if stats.BytesSaved != expectedBytesSaved {
		t.Errorf("Expected BytesSaved=%d, got %d", expectedBytesSaved, stats.BytesSaved)
	}

	// Deduplication ratio should be 80% (4 out of 5 files were duplicates)
	expectedRatio := 0.8
	actualRatio := index.DeduplicationRatio()
	if actualRatio < expectedRatio-0.01 || actualRatio > expectedRatio+0.01 {
		t.Errorf("Expected deduplication ratio ~%.2f, got %.2f", expectedRatio, actualRatio)
	}
}

func TestDeduplicationIndex_FindFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("findable content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file
	_, _, err := index.AddFile(testFile, 0, 0, "s3://bucket/chunk-0")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Find file by computing its hash manually
	hash, _, _, err := index.computeFileHash(testFile)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	loc := index.FindFile(hash)
	if loc == nil {
		t.Fatal("Expected to find file by hash")
	}
	if loc.ShardID != 0 {
		t.Errorf("Expected ShardID=0, got %d", loc.ShardID)
	}
	if loc.ChunkID != 0 {
		t.Errorf("Expected ChunkID=0, got %d", loc.ChunkID)
	}
	if loc.Size != int64(len(content)) {
		t.Errorf("Expected Size=%d, got %d", len(content), loc.Size)
	}
}

func TestDeduplicationIndex_FindFileByPath(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("path-findable content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file
	_, _, err := index.AddFile(testFile, 0, 0, "s3://bucket/chunk-0")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Find file by path
	loc, err := index.FindFileByPath(testFile)
	if err != nil {
		t.Fatalf("Failed to find file by path: %v", err)
	}
	if loc == nil {
		t.Fatal("Expected to find file by path")
	}
	if loc.Path != testFile {
		t.Errorf("Expected Path=%s, got %s", testFile, loc.Path)
	}
}

func TestDifferentFilesAreUnique(t *testing.T) {
	// Create two different temporary files
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	if err := os.WriteFile(file1, []byte("content 1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("content 2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add both files
	isDup1, _, err := index.AddFile(file1, 0, 0, "s3://bucket/chunk-0")
	if err != nil {
		t.Fatalf("Failed to add file1: %v", err)
	}
	if isDup1 {
		t.Error("Expected file1 to not be duplicate")
	}

	isDup2, _, err := index.AddFile(file2, 0, 1, "s3://bucket/chunk-1")
	if err != nil {
		t.Fatalf("Failed to add file2: %v", err)
	}
	if isDup2 {
		t.Error("Expected file2 to not be duplicate")
	}

	// Verify stats
	stats := index.GetStats()
	if stats.UniqueFiles != 2 {
		t.Errorf("Expected UniqueFiles=2, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateFiles != 0 {
		t.Errorf("Expected DuplicateFiles=0, got %d", stats.DuplicateFiles)
	}
	if index.Size() != 2 {
		t.Errorf("Expected index size=2, got %d", index.Size())
	}
}

func TestExportToManifest(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	content1 := []byte("unique content 1")
	content2 := []byte("unique content 2")

	if err := os.WriteFile(file1, content1, 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, content2, 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add files (file1 once, file2 twice)
	_, _, _ = index.AddFile(file1, 0, 0, "s3://bucket/chunk-0")
	_, _, _ = index.AddFile(file2, 1, 1, "s3://bucket/chunk-1")
	_, _, _ = index.AddFile(file2, 2, 2, "s3://bucket/chunk-2") // Duplicate

	// Export to manifest
	manifestDedup := index.ExportToManifest()

	if !manifestDedup.Enabled {
		t.Error("Expected deduplication to be enabled")
	}
	if manifestDedup.UniqueFiles != 2 {
		t.Errorf("Expected UniqueFiles=2, got %d", manifestDedup.UniqueFiles)
	}
	if manifestDedup.DuplicateFiles != 1 {
		t.Errorf("Expected DuplicateFiles=1, got %d", manifestDedup.DuplicateFiles)
	}
	if len(manifestDedup.FileReferences) != 2 {
		t.Errorf("Expected 2 file references, got %d", len(manifestDedup.FileReferences))
	}

	// Verify deduplication percentage
	expectedPct := (float64(len(content2)) / float64(len(content1)+len(content2)*2)) * 100
	if manifestDedup.DeduplicationPct < expectedPct-1 || manifestDedup.DeduplicationPct > expectedPct+1 {
		t.Errorf("Expected DeduplicationPct ~%.2f, got %.2f", expectedPct, manifestDedup.DeduplicationPct)
	}
}

func TestClear(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file
	_, _, _ = index.AddFile(testFile, 0, 0, "s3://bucket/chunk-0")

	if index.Size() != 1 {
		t.Errorf("Expected size=1 before clear, got %d", index.Size())
	}

	// Clear index
	index.Clear()

	if index.Size() != 0 {
		t.Errorf("Expected size=0 after clear, got %d", index.Size())
	}
}

func TestAddFile_NonexistentFile(t *testing.T) {
	index := NewFileDeduplicationIndex()

	// Try to add nonexistent file
	_, _, err := index.AddFile("/nonexistent/file.txt", 0, 0, "s3://bucket/chunk-0")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}

	// Stats should not be updated
	stats := index.GetStats()
	if stats.TotalFiles != 0 {
		t.Errorf("Expected TotalFiles=0 for failed add, got %d", stats.TotalFiles)
	}
}

func TestUpdateFileLocation(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file with placeholder location
	_, _, err := index.AddFile(testFile, -1, -1, "")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Get hash
	hash, _, _, err := index.computeFileHash(testFile)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	// Update location
	err = index.UpdateFileLocation(hash, 5, 10, "s3://bucket/shard-5/chunk-10")
	if err != nil {
		t.Fatalf("Failed to update location: %v", err)
	}

	// Verify location was updated
	loc := index.FindFile(hash)
	if loc == nil {
		t.Fatal("Expected to find file after update")
	}
	if loc.ShardID != 5 {
		t.Errorf("Expected ShardID=5, got %d", loc.ShardID)
	}
	if loc.ChunkID != 10 {
		t.Errorf("Expected ChunkID=10, got %d", loc.ChunkID)
	}
	if loc.S3Key != "s3://bucket/shard-5/chunk-10" {
		t.Errorf("Expected S3Key=s3://bucket/shard-5/chunk-10, got %s", loc.S3Key)
	}
}

func TestUpdateFileLocationByPath(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	index := NewFileDeduplicationIndex()

	// Add file with placeholder location
	_, _, err := index.AddFile(testFile, -1, -1, "")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Update location by path
	err = index.UpdateFileLocationByPath(testFile, 7, 15, "s3://bucket/shard-7/chunk-15")
	if err != nil {
		t.Fatalf("Failed to update location by path: %v", err)
	}

	// Verify location was updated
	loc, err := index.FindFileByPath(testFile)
	if err != nil {
		t.Fatalf("Failed to find file by path: %v", err)
	}
	if loc == nil {
		t.Fatal("Expected to find file after update")
	}
	if loc.ShardID != 7 {
		t.Errorf("Expected ShardID=7, got %d", loc.ShardID)
	}
	if loc.ChunkID != 15 {
		t.Errorf("Expected ChunkID=15, got %d", loc.ChunkID)
	}
	if loc.S3Key != "s3://bucket/shard-7/chunk-15" {
		t.Errorf("Expected S3Key=s3://bucket/shard-7/chunk-15, got %s", loc.S3Key)
	}
}

func TestUpdateFileLocation_NotFound(t *testing.T) {
	index := NewFileDeduplicationIndex()

	// Try to update location for non-existent hash
	err := index.UpdateFileLocation("nonexistent-hash", 0, 0, "s3://bucket/chunk-0")
	if err == nil {
		t.Error("Expected error for non-existent hash")
	}
}

func TestUpdateFileLocationByPath_NonexistentFile(t *testing.T) {
	index := NewFileDeduplicationIndex()

	// Try to update location for non-existent file
	err := index.UpdateFileLocationByPath("/nonexistent/file.txt", 0, 0, "s3://bucket/chunk-0")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}
