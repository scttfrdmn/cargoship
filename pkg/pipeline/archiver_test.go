package pipeline

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/compression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchiverStage_Name tests the Name method
func TestArchiverStage_Name(t *testing.T) {
	stage := &ArchiverStage{}
	name := stage.Name()
	if name != "archiver" {
		t.Errorf("Expected name 'archiver', got '%s'", name)
	}
}

// TestArchiverStage_GetCompressionStats tests the GetCompressionStats method
func TestArchiverStage_GetCompressionStats(t *testing.T) {
	stage := &ArchiverStage{}
	filesSkipped, timeSaved := stage.GetCompressionStats()

	// Should return zero values for new stage
	if filesSkipped != 0 {
		t.Errorf("Expected filesSkipped=0, got %d", filesSkipped)
	}
	if timeSaved != 0 {
		t.Errorf("Expected timeSaved=0, got %v", timeSaved)
	}
}

// TestArchiverStage_GetPaddingStats tests the GetPaddingStats method
func TestArchiverStage_GetPaddingStats(t *testing.T) {
	stage := &ArchiverStage{}
	paddingBytes := stage.GetPaddingStats()

	// Should return zero for new stage
	if paddingBytes != 0 {
		t.Errorf("Expected paddingBytes=0, got %d", paddingBytes)
	}
}

// TestArchiverStage_Stop tests the Stop method
func TestArchiverStage_Stop(t *testing.T) {
	config := &ArchiverConfig{
		Workers: 2,
	}

	input := make(chan *Job)
	output := make(chan *Job)

	stage, err := NewArchiverStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create archiver stage: %v", err)
	}

	// Close channels to allow clean shutdown
	close(input)

	// Stop should not error
	if err := stage.Stop(); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}

	// Calling Stop again should be safe (idempotent)
	if err := stage.Stop(); err != nil {
		t.Errorf("Second Stop() returned error: %v", err)
	}
}

// TestEncoderPool_Close tests the Close method
func TestEncoderPool_Close(t *testing.T) {
	pool, err := NewEncoderPool(2, 3) // 2 encoders at level 3
	if err != nil {
		t.Fatalf("Failed to create encoder pool: %v", err)
	}

	// Close should not error
	if err := pool.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Calling Close again should be safe
	if err := pool.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

// TestArchiverStage_AnalyzeChunkContentTypes tests content-aware compression level selection (Issue #105)
func TestArchiverStage_AnalyzeChunkContentTypes(t *testing.T) {
	// Create archiver stage with content-aware compressor
	config := &ArchiverConfig{
		Workers: 2,
	}

	input := make(chan *Job)
	output := make(chan *Job)

	stage, err := NewArchiverStage(config, input, output)
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()

	tests := []struct {
		name          string
		files         []chunking.File
		expectedLevel compression.Level
	}{
		{
			name: "pure code files - best compression",
			files: []chunking.File{
				{Path: "main.go", Size: 1024},
				{Path: "main_test.go", Size: 2048},
				{Path: "utils.go", Size: 512},
			},
			expectedLevel: compression.LevelBest, // 9
		},
		{
			name: "pure images - fastest compression",
			files: []chunking.File{
				{Path: "photo.jpg", Size: 1048576},
				{Path: "icon.png", Size: 51200},
				{Path: "background.webp", Size: 102400},
			},
			expectedLevel: compression.LevelFastest, // 1
		},
		{
			name: "pure documents - good compression",
			files: []chunking.File{
				{Path: "report.pdf", Size: 2097152},
				{Path: "spreadsheet.xlsx", Size: 524288},
			},
			expectedLevel: 6, // Level 6 for documents
		},
		{
			name: "pure binaries - fast compression",
			files: []chunking.File{
				{Path: "app.exe", Size: 10485760},
				{Path: "lib.so", Size: 1048576},
			},
			expectedLevel: compression.LevelFast, // 3
		},
		{
			name: "mixed with code predominant by size",
			files: []chunking.File{
				{Path: "main.go", Size: 10240},  // 10KB code
				{Path: "photo.jpg", Size: 5120}, // 5KB image
				{Path: "README.md", Size: 1024}, // 1KB text
			},
			expectedLevel: compression.LevelBest, // 9 (code is largest)
		},
		{
			name: "mixed with images predominant by size",
			files: []chunking.File{
				{Path: "photo.jpg", Size: 1048576}, // 1MB image
				{Path: "main.go", Size: 10240},     // 10KB code
			},
			expectedLevel: compression.LevelFastest, // 1 (images are largest)
		},
		{
			name: "archives - minimal compression",
			files: []chunking.File{
				{Path: "backup.zip", Size: 10485760},
				{Path: "data.tar.gz", Size: 5242880},
			},
			expectedLevel: compression.LevelFastest, // 1 (archives already compressed)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := stage.analyzeChunkContentTypes(tt.files)
			assert.Equal(t, tt.expectedLevel, level,
				"Expected compression level %d but got %d for %s",
				tt.expectedLevel, level, tt.name)
		})
	}
}

// TestArchiverStage_AnalyzeChunkContentTypesWithMagika tests Magika metadata integration (Issue #30 + #105)
func TestArchiverStage_AnalyzeChunkContentTypesWithMagika(t *testing.T) {
	config := &ArchiverConfig{
		Workers: 2,
	}

	input := make(chan *Job)
	output := make(chan *Job)

	stage, err := NewArchiverStage(config, input, output)
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()

	tests := []struct {
		name          string
		files         []chunking.File
		expectedLevel compression.Level
	}{
		{
			name: "Magika detects Python in .bin file",
			files: []chunking.File{
				{
					Path: "unknown.bin",
					Size: 10240,
					Metadata: map[string]string{
						"magika_type": "python",
					},
				},
			},
			expectedLevel: compression.LevelBest, // 9 (code)
		},
		{
			name: "Magika detects JPEG in .data file",
			files: []chunking.File{
				{
					Path: "file.data",
					Size: 1048576,
					Metadata: map[string]string{
						"magika_type": "jpeg",
					},
				},
			},
			expectedLevel: compression.LevelFastest, // 1 (image)
		},
		{
			name: "Magika overrides extension",
			files: []chunking.File{
				{
					Path: "photo.txt", // Extension says text
					Size: 1048576,
					Metadata: map[string]string{
						"magika_type": "jpeg", // But Magika says image
					},
				},
			},
			expectedLevel: compression.LevelFastest, // 1 (Magika wins)
		},
		{
			name: "Falls back to extension when no Magika metadata",
			files: []chunking.File{
				{
					Path:     "script.py",
					Size:     10240,
					Metadata: map[string]string{}, // No magika_type
				},
			},
			expectedLevel: compression.LevelBest, // 9 (extension-based detection)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := stage.analyzeChunkContentTypes(tt.files)
			assert.Equal(t, tt.expectedLevel, level)
		})
	}
}

// TestCreateEncoderPools tests multi-level encoder pool creation (Issue #105)
func TestCreateEncoderPools(t *testing.T) {
	poolSize := 4
	pools, err := createEncoderPools(poolSize, 0)
	require.NoError(t, err)
	require.NotNil(t, pools)

	// Verify all expected levels have pools
	expectedLevels := []compression.Level{1, 3, 6, 9}
	for _, level := range expectedLevels {
		pool, exists := pools[level]
		assert.True(t, exists, "Expected pool for level %d to exist", level)
		assert.NotNil(t, pool, "Pool for level %d should not be nil", level)
		assert.Equal(t, poolSize, pool.size, "Pool size mismatch for level %d", level)
		assert.Equal(t, level, pool.level, "Pool level mismatch")
	}

	// Cleanup
	for _, pool := range pools {
		_ = pool.Close()
	}
}

// TestCreateEncoderPoolsWithOverride verifies an explicit --compression-level
// outside the content-aware tier set gets its own pool (#316). Without this the
// override would miss the map and Process would silently fall back to level 3 —
// the accepted-but-ignored behavior #316 exists to remove.
func TestCreateEncoderPoolsWithOverride(t *testing.T) {
	t.Run("out-of-tier override gets a pool", func(t *testing.T) {
		pools, err := createEncoderPools(2, 19)
		require.NoError(t, err)
		defer func() {
			for _, pool := range pools {
				_ = pool.Close()
			}
		}()

		pool, exists := pools[19]
		require.True(t, exists, "level 19 override must have its own pool, not fall back to 3")
		assert.Equal(t, compression.Level(19), pool.level)
		assert.Len(t, pools, 5, "4 content-aware tiers + 1 override")
	})

	t.Run("override coinciding with a tier does not duplicate", func(t *testing.T) {
		pools, err := createEncoderPools(2, 9)
		require.NoError(t, err)
		defer func() {
			for _, pool := range pools {
				_ = pool.Close()
			}
		}()

		assert.Len(t, pools, 4, "level 9 is already a tier; no extra pool")
	})
}

// TestConvertToZstdLevel tests compression level conversion (Issue #105)
func TestConvertToZstdLevel(t *testing.T) {
	tests := []struct {
		name  string
		level compression.Level
		// We can't directly compare zstd.EncoderLevel values, but we can verify
		// the function doesn't panic and returns consistent results
	}{
		{"Level 1 - Fastest", 1},
		{"Level 3 - Fast/Default", 3},
		{"Level 6 - Better", 6},
		{"Level 9 - Best", 9},
		{"Level 5 - Default fallback", 5},
		{"Level 0 - Default fallback", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			zstdLevel := convertToZstdLevel(tt.level)
			assert.NotNil(t, zstdLevel)

			// Verify consistency
			zstdLevel2 := convertToZstdLevel(tt.level)
			assert.Equal(t, zstdLevel, zstdLevel2, "Conversion should be deterministic")
		})
	}
}

// TestEncoderPool_MultiLevel tests that different pools maintain different compression levels
func TestEncoderPool_MultiLevel(t *testing.T) {
	levels := []compression.Level{1, 3, 6, 9}
	pools := make([]*EncoderPool, len(levels))

	// Create pools at different levels
	for i, level := range levels {
		pool, err := NewEncoderPool(2, level)
		require.NoError(t, err)
		pools[i] = pool
		defer func() { _ = pool.Close() }()

		// Verify pool tracks correct level
		assert.Equal(t, level, pool.level)
	}

	// Verify each pool has 2 encoders
	for i, pool := range pools {
		encoder1 := pool.Get()
		require.NotNil(t, encoder1)

		encoder2 := pool.Get()
		require.NotNil(t, encoder2)

		// Return encoders
		pool.Put(encoder1)
		pool.Put(encoder2)

		t.Logf("Pool %d (level %d) successfully created with 2 encoders", i, levels[i])
	}
}

// TestNewArchiverStageWithSharding tests the sharding constructor
func TestNewArchiverStageWithSharding(t *testing.T) {
	config := &ArchiverConfig{
		Workers: 2,
	}

	input := make(chan *Job)
	outputs := map[string]chan<- *Job{
		"shard-0": make(chan *Job),
		"shard-1": make(chan *Job),
		"shard-2": make(chan *Job),
	}
	shardCount := 3

	stage, err := NewArchiverStageWithSharding(config, input, outputs, shardCount)
	require.NoError(t, err)
	require.NotNil(t, stage)
	defer func() { _ = stage.Stop() }()

	// Verify configuration
	assert.Equal(t, shardCount, stage.shardCount)
	assert.NotNil(t, stage.outputs)
	assert.Len(t, stage.shardDistribution, 3)
	assert.NotNil(t, stage.encoderPools)
	assert.NotNil(t, stage.contentAwareCompressor)
}

// TestNewArchiverStageWithSharding_Errors tests error handling
func TestNewArchiverStageWithSharding_Errors(t *testing.T) {
	tests := []struct {
		name       string
		config     *ArchiverConfig
		input      <-chan *Job
		outputs    map[string]chan<- *Job
		shardCount int
		wantErr    string
	}{
		{
			name:       "nil config",
			config:     nil,
			input:      make(chan *Job),
			outputs:    map[string]chan<- *Job{"shard-0": make(chan *Job)},
			shardCount: 1,
			wantErr:    "archiver config cannot be nil",
		},
		{
			name:       "empty outputs",
			config:     &ArchiverConfig{Workers: 2},
			input:      make(chan *Job),
			outputs:    map[string]chan<- *Job{},
			shardCount: 1,
			wantErr:    "outputs map cannot be empty",
		},
		{
			name:       "zero shard count",
			config:     &ArchiverConfig{Workers: 2},
			input:      make(chan *Job),
			outputs:    map[string]chan<- *Job{"shard-0": make(chan *Job)},
			shardCount: 0,
			wantErr:    "shard count must be positive",
		},
		{
			name:       "negative shard count",
			config:     &ArchiverConfig{Workers: 2},
			input:      make(chan *Job),
			outputs:    map[string]chan<- *Job{"shard-0": make(chan *Job)},
			shardCount: -1,
			wantErr:    "shard count must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage, err := NewArchiverStageWithSharding(tt.config, tt.input, tt.outputs, tt.shardCount)
			assert.Error(t, err)
			assert.Nil(t, stage)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestArchiverStage_SelectOutput tests output selection logic
func TestArchiverStage_SelectOutput(t *testing.T) {
	t.Run("single output mode", func(t *testing.T) {
		config := &ArchiverConfig{Workers: 2}
		input := make(chan *Job)
		output := make(chan *Job)

		stage, err := NewArchiverStage(config, input, output)
		require.NoError(t, err)
		defer func() { _ = stage.Stop() }()

		// Create a test job
		job := &Job{
			Chunk: chunking.Chunk{ID: 5},
		}

		// Should always return single output
		selectedOutput := stage.selectOutput(job)
		// Cast to chan<- *Job for comparison (selectOutput returns send-only channel)
		assert.Equal(t, chan<- *Job(output), selectedOutput)
	})

	t.Run("multi output sharding mode", func(t *testing.T) {
		config := &ArchiverConfig{Workers: 2}
		input := make(chan *Job)
		outputs := map[string]chan<- *Job{
			"shard-0": make(chan *Job, 10),
			"shard-1": make(chan *Job, 10),
			"shard-2": make(chan *Job, 10),
		}

		stage, err := NewArchiverStageWithSharding(config, input, outputs, 3)
		require.NoError(t, err)
		defer func() { _ = stage.Stop() }()

		// Test sharding distribution
		jobs := []*Job{
			{Chunk: chunking.Chunk{ID: 0}}, // 0 % 3 = 0 -> shard-0
			{Chunk: chunking.Chunk{ID: 1}}, // 1 % 3 = 1 -> shard-1
			{Chunk: chunking.Chunk{ID: 2}}, // 2 % 3 = 2 -> shard-2
			{Chunk: chunking.Chunk{ID: 3}}, // 3 % 3 = 0 -> shard-0
			{Chunk: chunking.Chunk{ID: 4}}, // 4 % 3 = 1 -> shard-1
		}

		for _, job := range jobs {
			output := stage.selectOutput(job)
			expectedShard := job.Chunk.ID % 3
			expectedOutput := outputs[fmt.Sprintf("shard-%d", expectedShard)]
			assert.Equal(t, expectedOutput, output)
		}

		// Verify shard distribution tracking
		assert.Equal(t, int64(2), *stage.shardDistribution["shard-0"]) // jobs 0, 3
		assert.Equal(t, int64(2), *stage.shardDistribution["shard-1"]) // jobs 1, 4
		assert.Equal(t, int64(1), *stage.shardDistribution["shard-2"]) // job 2
	})
}

// TestArchiverStage_Worker tests worker goroutine behavior
func TestArchiverStage_Worker(t *testing.T) {
	t.Run("worker processes jobs successfully", func(t *testing.T) {
		config := &ArchiverConfig{Workers: 1}
		input := make(chan *Job, 10)
		output := make(chan *Job, 10)

		stage, err := NewArchiverStage(config, input, output)
		require.NoError(t, err)

		// Create test context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start the stage
		err = stage.Start(ctx)
		require.NoError(t, err)

		// Send a job that will fail (no actual files)
		job := &Job{
			Chunk: chunking.Chunk{
				ID:    1,
				Files: []chunking.File{},
			},
		}

		input <- job
		close(input)

		// Wait for job to be processed
		processedJob := <-output
		assert.NotNil(t, processedJob)

		// Stop stage
		err = stage.Stop()
		assert.NoError(t, err)
	})

	t.Run("worker handles context cancellation", func(t *testing.T) {
		config := &ArchiverConfig{Workers: 1}
		input := make(chan *Job, 10)
		output := make(chan *Job, 10)

		stage, err := NewArchiverStage(config, input, output)
		require.NoError(t, err)

		// Create test context
		ctx, cancel := context.WithCancel(context.Background())

		// Start the stage
		err = stage.Start(ctx)
		require.NoError(t, err)

		// Cancel context immediately
		cancel()

		// Stop stage (should not hang)
		err = stage.Stop()
		assert.NoError(t, err)
	})
}

// TestArchiverStage_Process_NoTruncation is the #275 regression test: it drives
// a real chunk of multiple files (including highly-compressible content, which
// exposed the lost-final-flush bug) through Process and asserts every file
// decodes back to its full declared length with a clean tar EOF. Before the
// defer-ordering fix, the last file's zstd tail was dropped (short by a block).
func TestArchiverStage_Process_NoTruncation(t *testing.T) {
	// Create highly-compressible test files (constant bytes) — the case that
	// compressed small enough to reveal the truncated final frame.
	dir := t.TempDir()
	const nFiles = 5
	const fileSize = 512 * 1024 // 512KB each
	var files []chunking.File
	want := make(map[string]int64)
	for i := 0; i < nFiles; i++ {
		content := make([]byte, fileSize)
		for j := range content {
			content[j] = byte(i)
		}
		path := fmt.Sprintf("%s/file%d.dat", dir, i)
		require.NoError(t, os.WriteFile(path, content, 0644))
		files = append(files, chunking.File{Path: path, Size: fileSize})
		want[path] = fileSize
	}

	config := &ArchiverConfig{Workers: 2, CompressionType: "zstd"}
	input := make(chan *Job, 1)
	output := make(chan *Job, 1)
	stage, err := NewArchiverStage(config, input, output)
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()

	job := &Job{ID: 0, Chunk: chunking.Chunk{ID: 0, Files: files, TotalSize: nFiles * fileSize}}
	require.NoError(t, stage.Process(context.Background(), job))

	out := <-output
	require.NotNil(t, out.Archive)

	// Read the full archive stream, decompress, and confirm every file entry
	// reads back to its declared size with a clean EOF (no truncation).
	raw, err := io.ReadAll(out.Archive)
	require.NoError(t, err)
	_ = out.Archive.Close()

	dec, err := zstd.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer dec.Close()
	tr := tar.NewReader(dec)

	got := make(map[string]int64)
	for {
		hdr, terr := tr.Next()
		if terr == io.EOF {
			break
		}
		require.NoError(t, terr, "tar stream must not be truncated")
		if hdr.Name == ".padding" {
			continue
		}
		n, cerr := io.Copy(io.Discard, tr)
		require.NoError(t, cerr, "file %q content must not be truncated", hdr.Name)
		require.Equal(t, hdr.Size, n, "file %q read %d of %d declared bytes", hdr.Name, n, hdr.Size)
		got[hdr.Name] = n
	}

	require.Len(t, got, nFiles, "all files must be present in the archive")
	for path, size := range want {
		assert.Equal(t, size, got[path], "file %q should be full length", path)
	}
}

// archiveOnce runs one chunk of source-code-like text through Process and
// returns the compressed archive bytes plus the level the archiver recorded.
func archiveOnce(t *testing.T, level int, files []chunking.File, totalSize int64) ([]byte, string) {
	t.Helper()

	config := &ArchiverConfig{Workers: 1, CompressionType: "zstd", CompressionLevel: level}
	input := make(chan *Job, 1)
	output := make(chan *Job, 1)
	stage, err := NewArchiverStage(config, input, output)
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()

	job := &Job{ID: 0, Chunk: chunking.Chunk{ID: 0, Files: files, TotalSize: totalSize}}
	require.NoError(t, stage.Process(context.Background(), job))

	out := <-output
	require.NotNil(t, out.Archive)
	raw, err := io.ReadAll(out.Archive)
	require.NoError(t, err)
	_ = out.Archive.Close()

	return raw, out.Metadata["compression_level"]
}

// codeFiles writes compressible text files that content-aware detection
// classifies as source code (level 9).
func codeFiles(t *testing.T, n int) ([]chunking.File, int64) {
	t.Helper()
	dir := t.TempDir()

	// Semi-repetitive text: compressible enough that level differences show up
	// in output size, but not so uniform that every level produces the same
	// tiny result.
	var body []byte
	for i := 0; i < 4000; i++ {
		body = append(body, []byte(fmt.Sprintf("func handler%d(w http.ResponseWriter, r *http.Request) { log.Printf(\"%d\") }\n", i, i))...)
	}

	var files []chunking.File
	var total int64
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("%s/src%d.go", dir, i)
		require.NoError(t, os.WriteFile(path, body, 0644))
		files = append(files, chunking.File{Path: path, Size: int64(len(body))})
		total += int64(len(body))
	}
	return files, total
}

// TestArchiverStage_CompressionLevelOverride is the behavioral test for #316:
// --compression-level must change what the archiver actually does. Before the
// fix ArchiverConfig.CompressionLevel was never read, so all three subtests
// below produced byte-identical output at the content-aware level.
func TestArchiverStage_CompressionLevelOverride(t *testing.T) {
	files, total := codeFiles(t, 3)

	fastest, fastestLevel := archiveOnce(t, 1, files, total)
	best, bestLevel := archiveOnce(t, 9, files, total)

	assert.Equal(t, "1", fastestLevel, "override must be recorded in job metadata")
	assert.Equal(t, "9", bestLevel)
	assert.NotEqual(t, fastest, best,
		"level 1 and level 9 must produce different compressed output")
	assert.Less(t, len(best), len(fastest),
		"level 9 must compress source code better than level 1")
}

// TestArchiverStage_CompressionLevelOutOfPoolOverride covers the trap inside the
// fix: levels outside the pre-built {1,3,6,9} pools used to fall through to
// level 3, which would have silently recreated the no-op bug for exactly the
// values users reach for (--compression-level 19 for cold archival).
func TestArchiverStage_CompressionLevelOutOfPoolOverride(t *testing.T) {
	files, total := codeFiles(t, 2)

	_, level19 := archiveOnce(t, 19, files, total)
	assert.Equal(t, "19", level19,
		"an out-of-pool override must be honored, not silently downgraded to 3")

	// 19 and 3 land in different zstd tiers, so the bytes must differ too.
	out19, _ := archiveOnce(t, 19, files, total)
	out3, _ := archiveOnce(t, 3, files, total)
	assert.NotEqual(t, out3, out19,
		"level 19 must not compress identically to the level-3 fallback")
}

// TestArchiverStage_CompressionLevelAutomaticByDefault guards the other
// direction: the override must not become a regression that flattens
// content-aware selection (#105/#30) for users who never pass the flag.
func TestArchiverStage_CompressionLevelAutomaticByDefault(t *testing.T) {
	dir := t.TempDir()

	codeBody := []byte("package main\nfunc main() { println(\"hello\") }\n")
	codePath := dir + "/main.go"
	require.NoError(t, os.WriteFile(codePath, codeBody, 0644))

	// A .jpg is detected as an image: content-aware picks level 1 for it and
	// level 9 for source code, so with CompressionLevel 0 the two chunks must
	// record different levels.
	imgBody := make([]byte, len(codeBody))
	imgPath := dir + "/photo.jpg"
	require.NoError(t, os.WriteFile(imgPath, imgBody, 0644))

	_, codeLevel := archiveOnce(t, 0,
		[]chunking.File{{Path: codePath, Size: int64(len(codeBody))}}, int64(len(codeBody)))
	_, imgLevel := archiveOnce(t, 0,
		[]chunking.File{{Path: imgPath, Size: int64(len(imgBody))}}, int64(len(imgBody)))

	assert.NotEqual(t, codeLevel, imgLevel,
		"with no override, level must still vary by chunk content (#105)")
	assert.Equal(t, "9", codeLevel, "source code should still select level 9")
}

// TestEffectiveZstdTier documents that the advertised 1-22 range collapses onto
// four zstd tiers, which is why the CLI reports the effective tier rather than
// echoing the number back.
func TestEffectiveZstdTier(t *testing.T) {
	tests := map[int]string{
		1:  "fastest",
		2:  "fastest",
		3:  "default",
		5:  "default",
		6:  "better",
		9:  "best", // #105 tier mapping, not zstd's own banding
		12: "best",
		19: "best",
		22: "best",
	}
	for level, want := range tests {
		assert.Equal(t, want, EffectiveZstdTier(level), "level %d", level)
	}
}

// TestArchiverStage_ShardStrategyAffectsS3Key ties the strategy to the observable
// output: the shard appears in the S3 key and in job.ShardID, and the manifest
// records ShardID so restore never recomputes assignment.
func TestArchiverStage_ShardStrategyAffectsS3Key(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/a.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))
	files := []chunking.File{{Path: path, Size: 5}}

	run := func(strategy string, chunkID int) (int, string) {
		config := &ArchiverConfig{
			Workers: 1, CompressionType: "zstd",
			ShardCount: 8, ShardStrategy: strategy, UploadID: "test",
		}
		input := make(chan *Job, 1)
		output := make(chan *Job, 1)
		stage, err := NewArchiverStage(config, input, output)
		require.NoError(t, err)
		defer func() { _ = stage.Stop() }()

		job := &Job{ID: chunkID, Chunk: chunking.Chunk{ID: chunkID, Files: files, TotalSize: 5}}
		require.NoError(t, stage.Process(context.Background(), job))
		out := <-output
		_, _ = io.Copy(io.Discard, out.Archive)
		_ = out.Archive.Close()
		return out.ShardID, out.S3Key
	}

	// Round-robin is the historical behavior: the chunk ID picks the shard.
	for _, id := range []int{0, 1, 2, 3} {
		shard, key := run(ShardStrategyRoundRobin, id)
		assert.Equal(t, id, shard, "round-robin: chunk %d of 8 shards → shard %d", id, id)
		assert.Contains(t, key, fmt.Sprintf("shard-%d/", id),
			"S3 key and ShardID must agree — they were computed separately before #316")
	}

	// Hash assigns by content, so four different chunk IDs holding identical
	// files must all land on ONE shard. Before #316 every strategy resolved to
	// chunkID %% shardCount, which spreads these across shards 0-3 — so this
	// assertion is what distinguishes a real hash from the round-robin alias.
	var hashShards []int
	for _, id := range []int{0, 1, 2, 3} {
		shard, key := run(ShardStrategyHash, id)
		assert.Contains(t, key, fmt.Sprintf("shard-%d/", shard),
			"S3 key must carry the shard the assigner chose")
		hashShards = append(hashShards, shard)
	}
	for _, got := range hashShards[1:] {
		assert.Equal(t, hashShards[0], got,
			"identical content must hash to one shard regardless of chunk ID; got %v", hashShards)
	}
}
