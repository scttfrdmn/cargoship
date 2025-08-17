package indexing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestNewIndexer(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	assert.NotNil(t, indexer)
	assert.Equal(t, tempDir, indexer.basePath)
	assert.NotNil(t, indexer.logger)
	assert.NotNil(t, indexer.indexes)
	assert.Equal(t, 0, len(indexer.indexes))
}

func TestCreateIndex(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	// Create test inventory
	inv := createTestInventory()

	ctx := context.Background()
	location := "s3://test-bucket/test-prefix"

	// Create index
	archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
	require.NoError(t, err)
	require.NotNil(t, archiveIndex)

	// Verify index properties
	assert.Equal(t, location, archiveIndex.Location)
	assert.Equal(t, len(inv.Files), archiveIndex.FileCount)
	assert.Equal(t, len(inv.Files), len(archiveIndex.Files))
	assert.Equal(t, "1.0", archiveIndex.IndexVersion)
	assert.NotEmpty(t, archiveIndex.Checksums)
	assert.True(t, archiveIndex.TotalSize > 0)

	// Verify files were converted to enhanced files
	for i, enhancedFile := range archiveIndex.Files {
		assert.Equal(t, inv.Files[i].Path, enhancedFile.Path)
		assert.Equal(t, inv.Files[i].Name, enhancedFile.Name)
		assert.Equal(t, inv.Files[i].Size, enhancedFile.Size)

		// Check enhanced metadata
		assert.NotEmpty(t, enhancedFile.StorageClass)
		assert.NotNil(t, enhancedFile.Tags)
		assert.True(t, enhancedFile.HasTag("location_type", "s3"))
		assert.True(t, enhancedFile.HasTag("s3_bucket", "test-bucket"))
	}

	// Verify statistics were calculated
	stats := archiveIndex.Statistics
	assert.True(t, stats.AverageFileSize > 0)
	assert.True(t, stats.LargestFileSize >= stats.SmallestFileSize)
	assert.NotEmpty(t, stats.FileTypeDistribution)
	assert.NotEmpty(t, stats.SizeDistribution)

	// Verify cache was updated
	cached, exists := indexer.indexes[location]
	assert.True(t, exists)
	assert.Equal(t, archiveIndex, cached)
}

func TestSaveAndLoadIndex(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	// Create test index
	inv := createTestInventory()
	ctx := context.Background()
	location := "local://test-path"

	archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
	require.NoError(t, err)

	// Save index
	err = indexer.SaveIndex(ctx, archiveIndex)
	require.NoError(t, err)

	// Verify file was created
	indexPath := indexer.getIndexPath(location)
	assert.FileExists(t, indexPath)

	// Clear cache and load index
	indexer.ClearCache()
	assert.Equal(t, 0, len(indexer.indexes))

	loadedIndex, err := indexer.LoadIndex(ctx, location)
	require.NoError(t, err)
	require.NotNil(t, loadedIndex)

	// Verify loaded index matches original
	assert.Equal(t, archiveIndex.Location, loadedIndex.Location)
	assert.Equal(t, archiveIndex.FileCount, loadedIndex.FileCount)
	assert.Equal(t, archiveIndex.TotalSize, loadedIndex.TotalSize)
	assert.Equal(t, len(archiveIndex.Files), len(loadedIndex.Files))

	// Verify cache was updated
	cached, exists := indexer.indexes[location]
	assert.True(t, exists)
	assert.Equal(t, loadedIndex, cached)
}

func TestLoadNonExistentIndex(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	ctx := context.Background()

	_, err = indexer.LoadIndex(ctx, "nonexistent://location")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index not found")
}

func TestListIndexes(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	ctx := context.Background()
	inv := createTestInventory()

	// Initially no indexes
	locations, err := indexer.ListIndexes(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, len(locations))

	// Create and save multiple indexes
	testLocations := []string{
		"s3://bucket1/prefix1",
		"s3://bucket2/prefix2",
		"local://path1",
	}

	for _, location := range testLocations {
		archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
		require.NoError(t, err)

		err = indexer.SaveIndex(ctx, archiveIndex)
		require.NoError(t, err)
	}

	// List indexes
	locations, err = indexer.ListIndexes(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(testLocations), len(locations))

	// Verify all locations are present
	for _, expectedLocation := range testLocations {
		assert.Contains(t, locations, expectedLocation)
	}
}

func TestGetCacheStats(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	// Initially empty cache
	stats := indexer.GetCacheStats()
	assert.Equal(t, 0, stats["cached_indexes"])
	assert.Equal(t, 0, stats["total_cached_files"])
	assert.Equal(t, int64(0), stats["total_cached_size"])

	// Create index
	inv := createTestInventory()
	ctx := context.Background()
	location := "test://location"

	archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
	require.NoError(t, err)

	// Check updated stats
	stats = indexer.GetCacheStats()
	assert.Equal(t, 1, stats["cached_indexes"])
	assert.Equal(t, archiveIndex.FileCount, stats["total_cached_files"])
	assert.Equal(t, archiveIndex.TotalSize, stats["total_cached_size"])
}

func TestClearCache(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	indexer := NewIndexer(tempDir, logger)

	// Create index to populate cache
	inv := createTestInventory()
	ctx := context.Background()
	location := "test://location"

	_, err = indexer.CreateIndex(ctx, inv, location)
	require.NoError(t, err)

	// Verify cache has content
	assert.Equal(t, 1, len(indexer.indexes))

	// Clear cache
	indexer.ClearCache()

	// Verify cache is empty
	assert.Equal(t, 0, len(indexer.indexes))

	stats := indexer.GetCacheStats()
	assert.Equal(t, 0, stats["cached_indexes"])
}

func TestConvertFromInventoryFile(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	invFile := &inventory.File{
		Path:          "/test/path/file.txt",
		Destination:   "path/file.txt",
		Name:          "file.txt",
		Size:          1024,
		ArchiveTOC:    []string{"file1.txt", "file2.txt"},
		SuitcaseIndex: 1,
		SuitcaseName:  "test-suitcase-01-of-01.tar.zst",
	}

	enhanced := ConvertFromInventoryFile(invFile)

	assert.Equal(t, invFile.Path, enhanced.Path)
	assert.Equal(t, invFile.Destination, enhanced.Destination)
	assert.Equal(t, invFile.Name, enhanced.Name)
	assert.Equal(t, invFile.Size, enhanced.Size)
	assert.Equal(t, invFile.ArchiveTOC, enhanced.ArchiveTOC)
	assert.Equal(t, invFile.SuitcaseIndex, enhanced.SuitcaseIndex)
	assert.Equal(t, invFile.SuitcaseName, enhanced.SuitcaseName)

	// Check enhanced fields
	assert.Equal(t, "STANDARD", enhanced.StorageClass)
	assert.NotNil(t, enhanced.Tags)
	assert.False(t, enhanced.CreatedAt.IsZero())
	assert.False(t, enhanced.ModifiedAt.IsZero())
}

func TestEnhancedFileHelpers(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	enhanced := &EnhancedFile{
		File: inventory.File{
			Name: "test.txt",
			Size: 1024 * 1024, // 1MB
		},
		Tags: make(map[string]string),
		CompressionInfo: CompressionInfo{
			OriginalSize:   2048,
			CompressedSize: 1024,
		},
	}

	// Test tag operations
	assert.False(t, enhanced.HasTag("test", "value"))

	enhanced.AddTag("test", "value")
	assert.True(t, enhanced.HasTag("test", "value"))
	assert.False(t, enhanced.HasTag("test", "other"))

	// Test size helpers
	humanSize := enhanced.GetHumanSize()
	assert.Contains(t, humanSize, "MB")

	// Test compression helpers
	assert.True(t, enhanced.IsCompressed())
	ratio := enhanced.GetCompressionRatio()
	assert.Equal(t, 0.5, ratio)
}

func TestHelperFunctions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Test sanitizeKey
	assert.Equal(t, "s3___bucket_path", sanitizeKey("s3://bucket/path"))

	// Test sanitizePathForFilename
	assert.Equal(t, "s3_bucket_path", sanitizePathForFilename("s3://bucket/path"))

	// Test getContentType
	assert.Equal(t, "text/plain", getContentType("file.txt"))
	assert.Equal(t, "application/json", getContentType("data.json"))
	assert.Equal(t, "application/x-fastq", getContentType("reads.fastq"))

	// Test parseS3Location
	bucket, key := parseS3Location("s3://test-bucket/prefix/key")
	assert.Equal(t, "test-bucket", bucket)
	assert.Equal(t, "prefix/key", key)

	bucket, key = parseS3Location("s3://bucket-only")
	assert.Equal(t, "bucket-only", bucket)
	assert.Equal(t, "", key)

	bucket, key = parseS3Location("not-s3://invalid")
	assert.Equal(t, "", bucket)
	assert.Equal(t, "", key)

	// Test isCompressedFile
	assert.True(t, isCompressedFile("file.gz"))
	assert.True(t, isCompressedFile("archive.tar.gz"))
	assert.False(t, isCompressedFile("file.txt"))

	// Test getCompressionAlgorithm
	assert.Equal(t, "gzip", getCompressionAlgorithm("file.gz"))
	assert.Equal(t, "zstd", getCompressionAlgorithm("file.zst"))
	assert.Equal(t, "zip", getCompressionAlgorithm("file.zip"))

	// Test getFileType
	assert.Equal(t, "bioinformatics", getFileType("reads.fastq"))
	assert.Equal(t, "data", getFileType("data.json"))
	assert.Equal(t, "text", getFileType("readme.txt"))
	assert.Equal(t, "archive", getFileType("backup.zip"))

	// Test getSizeCategory
	assert.Equal(t, "tiny", getSizeCategory(500))
	assert.Equal(t, "small", getSizeCategory(50*1024))
	assert.Equal(t, "medium", getSizeCategory(5*1024*1024))
	assert.Equal(t, "large", getSizeCategory(50*1024*1024))
	assert.Equal(t, "massive", getSizeCategory(2*1024*1024*1024*1024))
}

// Helper function to create test inventory
func createTestInventory() *inventory.Inventory {
	now := time.Now()

	files := []*inventory.File{
		{
			Path:        "/test/data/file1.txt",
			Destination: "data/file1.txt",
			Name:        "file1.txt",
			Size:        1024,
		},
		{
			Path:        "/test/data/reads.fastq.gz",
			Destination: "data/reads.fastq.gz",
			Name:        "reads.fastq.gz",
			Size:        1024 * 1024 * 100, // 100MB
			ArchiveTOC:  []string{"sequence1.fq", "sequence2.fq"},
		},
		{
			Path:        "/test/results/analysis.json",
			Destination: "results/analysis.json",
			Name:        "analysis.json",
			Size:        4096,
		},
		{
			Path:        "/test/docs/README.md",
			Destination: "docs/README.md",
			Name:        "README.md",
			Size:        2048,
		},
	}

	options := &inventory.Options{
		User:            "testuser",
		Prefix:          "test",
		MaxSuitcaseSize: 1024 * 1024 * 1024, // 1GB
		SuitcaseFormat:  "tar.zst",
	}

	inv := &inventory.Inventory{
		Files:        files,
		Options:      options,
		TotalIndexes: 1,
		IndexSummaries: map[int]*inventory.IndexSummary{
			1: {
				Count:     uint(len(files)),
				Size:      1024 + 1024*1024*100 + 4096 + 2048,
				HumanSize: "100MB",
			},
		},
		InternalMetadata: map[string]string{
			"created_at": now.Format(time.RFC3339),
			"test_data":  "true",
		},
		ExternalMetadata: map[string]string{
			"project": "test_project",
			"version": "1.0",
		},
	}

	return inv
}

// Benchmark tests
func BenchmarkCreateIndex(b *testing.B) {
	testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
		tempDir, err := os.MkdirTemp("", "indexer_benchmark")
		require.NoError(b, err)
		defer func() {
			if removeErr := os.RemoveAll(tempDir); removeErr != nil {
				_ = removeErr // Ignore remove error in benchmark
			}
		}()

		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		indexer := NewIndexer(tempDir, logger)
		inv := createTestInventory()
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			location := fmt.Sprintf("test://benchmark-%d", i)
			_, err := indexer.CreateIndex(ctx, inv, location)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkSearchIndex(b *testing.B) {
	testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
		tempDir, err := os.MkdirTemp("", "search_benchmark")
		require.NoError(b, err)
		defer func() {
			if removeErr := os.RemoveAll(tempDir); removeErr != nil {
				_ = removeErr // Ignore remove error in benchmark
			}
		}()

		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		indexer := NewIndexer(tempDir, logger)
		searchEngine := NewSearchEngine(indexer, logger)

		// Create test index
		inv := createTestInventory()
		ctx := context.Background()
		location := "test://benchmark"

		_, err = indexer.CreateIndex(ctx, inv, location)
		require.NoError(b, err)

		filter := SearchFilter{
			NamePattern: "*.txt",
			MaxResults:  100,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := searchEngine.Search(ctx, filter, location)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
