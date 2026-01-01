package chunking

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// mockTierSelector implements TierSelector for testing
type mockTierSelector struct {
	hotDays     int
	coldDays    int
	archiveDays int
}

func (m *mockTierSelector) SelectTier(atime, mtime time.Time) types.StorageClass {
	accessTime := atime
	if atime.IsZero() {
		accessTime = mtime
	}

	if accessTime.IsZero() {
		return types.StorageClassStandard
	}

	daysSinceAccess := time.Since(accessTime).Hours() / 24

	switch {
	case daysSinceAccess >= float64(m.archiveDays):
		return types.StorageClassDeepArchive
	case daysSinceAccess >= float64(m.coldDays):
		return types.StorageClassGlacier
	case daysSinceAccess >= float64(m.hotDays):
		return types.StorageClassStandardIa
	default:
		return types.StorageClassStandard
	}
}

// mockChunkingStrategy implements ChunkingStrategy for testing
type mockChunkingStrategy struct {
	chunkSize int64
}

func (m *mockChunkingStrategy) CalculateOptimalChunkSize(
	totalSize int64,
	fileCount int,
	availableMemory int64,
	costSavingsTarget float64,
) (chunkSize int64, stats ChunkStats) {
	return m.chunkSize, ChunkStats{}
}

func (m *mockChunkingStrategy) GroupFilesIntoChunks(
	files []File,
	chunkSize int64,
) ([]Chunk, error) {
	if len(files) == 0 {
		return []Chunk{}, nil
	}

	var chunks []Chunk
	var currentChunk Chunk
	currentChunk.ID = 0
	currentChunk.Files = []File{}

	for _, file := range files {
		if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
			// Start new chunk
			currentChunk.FileCount = len(currentChunk.Files)
			chunks = append(chunks, currentChunk)

			currentChunk = Chunk{
				ID:    len(chunks),
				Files: []File{},
			}
		}

		currentChunk.Files = append(currentChunk.Files, file)
		currentChunk.TotalSize += file.Size
	}

	// Add final chunk
	if len(currentChunk.Files) > 0 {
		currentChunk.FileCount = len(currentChunk.Files)
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

func TestNewTierAwareChunker(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}

	chunker := NewTierAwareChunker(base, selector, 10000)

	if chunker.baseChunker == nil {
		t.Error("Expected baseChunker to be set")
	}

	if chunker.tierSelector == nil {
		t.Error("Expected tierSelector to be set")
	}

	if chunker.bufferSize != 10000 {
		t.Errorf("Expected bufferSize 10000, got %d", chunker.bufferSize)
	}
}

func TestNewTierAwareChunker_DefaultBufferSize(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}

	chunker := NewTierAwareChunker(base, selector, 0)

	if chunker.bufferSize != 100000 {
		t.Errorf("Expected default bufferSize 100000, got %d", chunker.bufferSize)
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_EmptyFiles(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	chunks, err := chunker.GroupFilesIntoChunks([]File{}, 100*1024*1024)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks, got %d", len(chunks))
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_SingleTier(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 50 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		{Path: "file1.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "file2.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-15 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "file3.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 50*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// All files should be in STANDARD tier (< 30 days old)
	// Total size: 60MB, chunk size: 50MB → 2 chunks
	if len(chunks) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(chunks))
	}

	// All chunks should have STANDARD tier
	for i, chunk := range chunks {
		if chunk.PreAssignedTier != types.StorageClassStandard {
			t.Errorf("Chunk %d: expected STANDARD tier, got %v", i, chunk.PreAssignedTier)
		}
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_MultipleTiers(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		// STANDARD (< 30 days)
		{Path: "hot1.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "hot2.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)}},

		// STANDARD_IA (30-90 days)
		{Path: "warm1.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-50 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "warm2.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-60 * 24 * time.Hour).Format(time.RFC3339)}},

		// GLACIER (90-180 days)
		{Path: "cold1.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold2.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-120 * 24 * time.Hour).Format(time.RFC3339)}},

		// DEEP_ARCHIVE (>= 180 days)
		{Path: "archive1.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-200 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "archive2.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-300 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 100*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should create 4 chunks (one per tier, all fit in single chunk per tier)
	if len(chunks) != 4 {
		t.Errorf("Expected 4 chunks (one per tier), got %d", len(chunks))
	}

	// Verify tier assignment
	expectedTiers := []types.StorageClass{
		types.StorageClassStandard,
		types.StorageClassStandardIa,
		types.StorageClassGlacier,
		types.StorageClassDeepArchive,
	}

	tierCounts := make(map[types.StorageClass]int)
	for _, chunk := range chunks {
		tierCounts[chunk.PreAssignedTier]++
	}

	for _, expectedTier := range expectedTiers {
		if tierCounts[expectedTier] != 1 {
			t.Errorf("Expected 1 chunk with tier %v, got %d", expectedTier, tierCounts[expectedTier])
		}
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_TierGroupSpansMultipleChunks(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 30 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		// 5 GLACIER files, each 20MB → 100MB total
		// With 30MB chunk size → should create ~4 chunks
		{Path: "cold1.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold2.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold3.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold4.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold5.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 30*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should create multiple chunks, all with GLACIER tier
	if len(chunks) < 3 {
		t.Errorf("Expected at least 3 chunks, got %d", len(chunks))
	}

	// All chunks should be GLACIER
	for i, chunk := range chunks {
		if chunk.PreAssignedTier != types.StorageClassGlacier {
			t.Errorf("Chunk %d: expected GLACIER tier, got %v", i, chunk.PreAssignedTier)
		}
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_MissingAtimeFallbackToMtime(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	oldMtime := now.Add(-100 * 24 * time.Hour)

	files := []File{
		// No atime, should fall back to mtime (100 days old → GLACIER)
		{Path: "file1.txt", Size: 10 * 1024 * 1024, ModTime: oldMtime, Metadata: map[string]string{}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 100*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	// Should select GLACIER based on mtime
	if chunks[0].PreAssignedTier != types.StorageClassGlacier {
		t.Errorf("Expected GLACIER tier (based on mtime), got %v", chunks[0].PreAssignedTier)
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_InvalidAtimeFormat(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		// Invalid atime format, should fall back to mtime
		{Path: "file1.txt", Size: 10 * 1024 * 1024, ModTime: now.Add(-100 * 24 * time.Hour), Metadata: map[string]string{"atime": "invalid-date"}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 100*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	// Should fall back to mtime and select GLACIER
	if chunks[0].PreAssignedTier != types.StorageClassGlacier {
		t.Errorf("Expected GLACIER tier (fallback to mtime), got %v", chunks[0].PreAssignedTier)
	}
}

func TestTierAwareChunker_GroupFilesIntoChunks_BufferSizeExceeded(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 2) // Small buffer: only 2 files per tier

	now := time.Now()
	files := []File{
		// 3 files in STANDARD tier → should exceed buffer
		{Path: "file1.txt", Size: 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "file2.txt", Size: 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "file3.txt", Size: 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	_, err := chunker.GroupFilesIntoChunks(files, 100*1024*1024)

	if err == nil {
		t.Error("Expected buffer size exceeded error, got nil")
	}

	expectedError := "tier group buffer size exceeded"
	if err != nil && !contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing %q, got %q", expectedError, err.Error())
	}
}

func TestTierAwareChunker_ChunkIDsAreSequential(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 20 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		// STANDARD tier - 2 chunks
		{Path: "hot1.txt", Size: 15 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "hot2.txt", Size: 15 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},

		// GLACIER tier - 2 chunks
		{Path: "cold1.txt", Size: 15 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold2.txt", Size: 15 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	chunks, err := chunker.GroupFilesIntoChunks(files, 20*1024*1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify IDs are sequential: 0, 1, 2, 3
	expectedIDs := []int{0, 1, 2, 3}
	for i, chunk := range chunks {
		if chunk.ID != expectedIDs[i] {
			t.Errorf("Chunk %d: expected ID %d, got %d", i, expectedIDs[i], chunk.ID)
		}
	}
}

func TestTierAwareChunker_GetTierDistribution(t *testing.T) {
	base := &mockChunkingStrategy{chunkSize: 100 * 1024 * 1024}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	now := time.Now()
	files := []File{
		// 2 STANDARD files
		{Path: "hot1.txt", Size: 10 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "hot2.txt", Size: 20 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},

		// 3 GLACIER files
		{Path: "cold1.txt", Size: 30 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold2.txt", Size: 40 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
		{Path: "cold3.txt", Size: 50 * 1024 * 1024, ModTime: now, Metadata: map[string]string{"atime": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	distribution := chunker.GetTierDistribution(files)

	// Verify STANDARD tier
	standardStats := distribution[types.StorageClassStandard]
	if standardStats.FileCount != 2 {
		t.Errorf("Expected 2 STANDARD files, got %d", standardStats.FileCount)
	}
	if standardStats.TotalSize != 30*1024*1024 {
		t.Errorf("Expected STANDARD size %d, got %d", 30*1024*1024, standardStats.TotalSize)
	}

	// Verify GLACIER tier
	glacierStats := distribution[types.StorageClassGlacier]
	if glacierStats.FileCount != 3 {
		t.Errorf("Expected 3 GLACIER files, got %d", glacierStats.FileCount)
	}
	if glacierStats.TotalSize != 120*1024*1024 {
		t.Errorf("Expected GLACIER size %d, got %d", 120*1024*1024, glacierStats.TotalSize)
	}
}

func TestParseAtime(t *testing.T) {
	tests := []struct {
		name      string
		atimeStr  string
		wantError bool
	}{
		{
			name:      "valid RFC3339",
			atimeStr:  "2024-01-15T10:30:00Z",
			wantError: false,
		},
		{
			name:      "valid RFC3339 with timezone",
			atimeStr:  "2024-01-15T10:30:00-08:00",
			wantError: false,
		},
		{
			name:      "empty string",
			atimeStr:  "",
			wantError: true,
		},
		{
			name:      "invalid format",
			atimeStr:  "2024-01-15",
			wantError: true,
		},
		{
			name:      "garbage",
			atimeStr:  "not-a-date",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAtime(tt.atimeStr)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if result.IsZero() {
					t.Error("Expected non-zero time")
				}
			}
		})
	}
}

func TestTierAwareChunker_CalculateOptimalChunkSize_DelegatesToBase(t *testing.T) {
	expectedChunkSize := int64(100 * 1024 * 1024)
	base := &mockChunkingStrategy{chunkSize: expectedChunkSize}
	selector := &mockTierSelector{hotDays: 30, coldDays: 90, archiveDays: 180}
	chunker := NewTierAwareChunker(base, selector, 10000)

	chunkSize, _ := chunker.CalculateOptimalChunkSize(1000*1024*1024, 100, 512*1024*1024, 1000.0)

	if chunkSize != expectedChunkSize {
		t.Errorf("Expected chunk size %d, got %d", expectedChunkSize, chunkSize)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
