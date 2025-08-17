package indexing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestSearchEngine(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Creation_And_Basic_Properties", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "search_test")
		require.NoError(t, err)
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				_ = err // Ignore cleanup errors in tests
			}
		}()

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		indexer := NewIndexer(tempDir, logger)
		searchEngine := NewSearchEngine(indexer, logger)

		assert.NotNil(t, searchEngine)
		assert.NotNil(t, searchEngine.indexer)
		assert.NotNil(t, searchEngine.logger)
	})

	t.Run("Search_With_Empty_Index", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "search_empty_test")
		require.NoError(t, err)
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				_ = err // Ignore cleanup errors in tests
			}
		}()

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		indexer := NewIndexer(tempDir, logger)
		searchEngine := NewSearchEngine(indexer, logger)

		ctx := context.Background()
		filter := SearchFilter{}

		result, err := searchEngine.Search(ctx, filter)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Files)
		assert.Equal(t, 0, result.TotalMatches)
		assert.Equal(t, "none", result.IndexUsed)
		assert.False(t, result.Truncated)
	})
}

func TestSearchFilter(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Basic_Filter_Creation", func(t *testing.T) {
		filter := SearchFilter{
			NamePattern:  "*.fastq.gz",
			MinSize:      1024 * 1024,        // 1MB
			MaxSize:      1024 * 1024 * 1024, // 1GB
			Extensions:   []string{".fastq.gz", ".bam"},
			ContentType:  "application/gzip",
			StorageClass: "STANDARD_IA",
			MaxResults:   100,
		}

		assert.Equal(t, "*.fastq.gz", filter.NamePattern)
		assert.Equal(t, int64(1024*1024), filter.MinSize)
		assert.Equal(t, int64(1024*1024*1024), filter.MaxSize)
		assert.Len(t, filter.Extensions, 2)
		assert.Contains(t, filter.Extensions, ".fastq.gz")
		assert.Contains(t, filter.Extensions, ".bam")
		assert.Equal(t, "application/gzip", filter.ContentType)
		assert.Equal(t, "STANDARD_IA", filter.StorageClass)
		assert.Equal(t, 100, filter.MaxResults)
	})

	t.Run("Date_Range_Filters", func(t *testing.T) {
		startDate := time.Now().Add(-30 * 24 * time.Hour)
		endDate := time.Now().Add(-7 * 24 * time.Hour)

		filter := SearchFilter{
			ModifiedAfter:  &startDate,
			ModifiedBefore: &endDate,
		}

		assert.NotNil(t, filter.ModifiedAfter)
		assert.NotNil(t, filter.ModifiedBefore)
		assert.True(t, filter.ModifiedAfter.Before(*filter.ModifiedBefore))
	})

	t.Run("Tags_Filter", func(t *testing.T) {
		filter := SearchFilter{
			Tags: map[string]string{
				"project":    "genomics",
				"researcher": "dr-smith",
				"stage":      "analysis",
			},
		}

		assert.Len(t, filter.Tags, 3)
		assert.Equal(t, "genomics", filter.Tags["project"])
		assert.Equal(t, "dr-smith", filter.Tags["researcher"])
		assert.Equal(t, "analysis", filter.Tags["stage"])
	})

	t.Run("Compression_Filters", func(t *testing.T) {
		filter := SearchFilter{
			CompressionType:     "zstd",
			MinCompressionRatio: 0.3,
			HasArchiveTOC:       true,
		}

		assert.Equal(t, "zstd", filter.CompressionType)
		assert.Equal(t, 0.3, filter.MinCompressionRatio)
		assert.True(t, filter.HasArchiveTOC)
	})
}

func TestSearchWithMockData(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create a comprehensive test setup
	tempDir, err := os.MkdirTemp("", "search_mock_test")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			_ = err // Ignore cleanup errors in tests
		}
	}()

	// Create test inventory
	inventoryFile := filepath.Join(tempDir, "test-inventory.yaml")
	yamlContent := `
files:
  - path: /data/sample1.fastq.gz
    destination: genomics/raw/sample1.fastq.gz
    name: sample1.fastq.gz
    size: 524288000
    suitcase_name: genomics-01.tar.zst
  - path: /data/sample2.bam
    destination: genomics/aligned/sample2.bam
    name: sample2.bam
    size: 1073741824
    suitcase_name: genomics-02.tar.zst
  - path: /results/analysis.vcf
    destination: genomics/results/analysis.vcf
    name: analysis.vcf
    size: 104857600
    suitcase_name: genomics-03.tar.zst
  - path: /docs/readme.txt
    destination: docs/readme.txt
    name: readme.txt
    size: 4096
    suitcase_name: genomics-04.tar.zst
total_indexes: 4
`

	err = os.WriteFile(inventoryFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	t.Run("Search_With_Real_Inventory_Data", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		indexer := NewIndexer(filepath.Join(tempDir, "cache"), logger)
		searchEngine := NewSearchEngine(indexer, logger)

		// Load inventory and create index
		inv, err := inventory.NewInventoryWithFilename(inventoryFile)
		require.NoError(t, err)

		ctx := context.Background()
		location := "s3://test-bucket/genomics/"

		// Create index
		archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
		require.NoError(t, err)
		assert.Equal(t, 4, archiveIndex.FileCount)

		// Save index for searching
		err = indexer.SaveIndex(ctx, archiveIndex)
		require.NoError(t, err)

		// Test various search scenarios
		t.Run("Search_By_Name_Pattern", func(t *testing.T) {
			filter := SearchFilter{
				NamePattern: "*.fastq.gz",
			}

			result, err := searchEngine.Search(ctx, filter, location)
			require.NoError(t, err)
			assert.Equal(t, 1, result.TotalMatches)
			assert.Len(t, result.Files, 1)
			assert.Equal(t, "sample1.fastq.gz", result.Files[0].Name)
		})

		t.Run("Search_By_Size_Range", func(t *testing.T) {
			filter := SearchFilter{
				MinSize: 100 * 1024 * 1024, // 100MB
				MaxSize: 600 * 1024 * 1024, // 600MB
			}

			result, err := searchEngine.Search(ctx, filter, location)
			require.NoError(t, err)
			// Should find sample1.fastq.gz (500MB) and analysis.vcf (100MB)
			assert.Equal(t, 2, result.TotalMatches)
			assert.Len(t, result.Files, 2)
		})

		t.Run("Search_By_File_Extensions", func(t *testing.T) {
			filter := SearchFilter{
				Extensions: []string{".bam", ".vcf"},
			}

			result, err := searchEngine.Search(ctx, filter, location)
			require.NoError(t, err)
			// Should find sample2.bam and analysis.vcf
			assert.Equal(t, 2, result.TotalMatches)
			assert.Len(t, result.Files, 2)

			fileNames := []string{result.Files[0].Name, result.Files[1].Name}
			assert.Contains(t, fileNames, "sample2.bam")
			assert.Contains(t, fileNames, "analysis.vcf")
		})

		t.Run("Search_With_Max_Results_Limit", func(t *testing.T) {
			filter := SearchFilter{
				MaxResults: 2,
			}

			result, err := searchEngine.Search(ctx, filter, location)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(result.Files), 2)
			assert.LessOrEqual(t, len(result.Files), result.TotalMatches)

			if result.TotalMatches > 2 {
				assert.True(t, result.Truncated)
			}
		})
	})
}

func TestBrowseOperations(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Browse_Options_Creation", func(t *testing.T) {
		options := BrowseOptions{
			Recursive:    true,
			ShowMetadata: true,
			ShowHidden:   false,
			SortBy:       "name",
			SortOrder:    "asc",
			MaxDepth:     3,
			PageSize:     50,
			PageOffset:   0,
		}

		assert.True(t, options.Recursive)
		assert.True(t, options.ShowMetadata)
		assert.False(t, options.ShowHidden)
		assert.Equal(t, "name", options.SortBy)
		assert.Equal(t, "asc", options.SortOrder)
		assert.Equal(t, 3, options.MaxDepth)
		assert.Equal(t, 50, options.PageSize)
		assert.Equal(t, 0, options.PageOffset)
	})

	t.Run("Browse_Options_With_Filter", func(t *testing.T) {
		filter := &SearchFilter{
			NamePattern: "*.log",
			MinSize:     1024,
		}

		options := BrowseOptions{
			Filter:         filter,
			Recursive:      true,
			ContentPreview: true,
		}

		assert.NotNil(t, options.Filter)
		assert.Equal(t, "*.log", options.Filter.NamePattern)
		assert.Equal(t, int64(1024), options.Filter.MinSize)
		assert.True(t, options.Recursive)
		assert.True(t, options.ContentPreview)
	})
}

func TestSearchResult(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Search_Result_Properties", func(t *testing.T) {
		searchTime := 150 * time.Millisecond

		files := []*EnhancedFile{
			{
				File: inventory.File{
					Name: "test1.txt",
					Size: 1024,
				},
			},
			{
				File: inventory.File{
					Name: "test2.txt",
					Size: 2048,
				},
			},
		}

		result := &SearchResult{
			Files:        files,
			TotalMatches: 2,
			SearchTime:   searchTime,
			IndexUsed:    "s3://test-bucket/data/",
			Truncated:    false,
		}

		assert.Len(t, result.Files, 2)
		assert.Equal(t, 2, result.TotalMatches)
		assert.Equal(t, searchTime, result.SearchTime)
		assert.Equal(t, "s3://test-bucket/data/", result.IndexUsed)
		assert.False(t, result.Truncated)
		// Result struct doesn't have Query or Timestamp fields
	})

	t.Run("Truncated_Search_Result", func(t *testing.T) {
		// Simulate a search that returned more results than the limit
		files := make([]*EnhancedFile, 100) // 100 files returned
		for i := 0; i < 100; i++ {
			files[i] = &EnhancedFile{
				File: inventory.File{
					Name: fmt.Sprintf("file%d.txt", i),
					Size: int64(1024 * (i + 1)),
				},
			}
		}

		result := &SearchResult{
			Files:        files,
			TotalMatches: 150, // But there were actually 150 matches total
			SearchTime:   300 * time.Millisecond,
			IndexUsed:    "test-index",
			Truncated:    true, // Results were truncated
		}

		assert.Len(t, result.Files, 100)
		assert.Equal(t, 150, result.TotalMatches)
		assert.True(t, result.Truncated)
		assert.Greater(t, result.TotalMatches, len(result.Files))
	})
}

func TestBrowseResult(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Browse_Result_With_Directories", func(t *testing.T) {
		directories := []DirectoryInfo{
			{
				Name:         "raw-data",
				Path:         "/project/genomics/raw-data",
				FileCount:    25,
				TotalSize:    1024 * 1024 * 500, // 500MB
				LastModified: time.Now(),
				IsArchive:    false,
			},
			{
				Name:         "results.tar.gz",
				Path:         "/project/genomics/results.tar.gz",
				FileCount:    10,
				TotalSize:    1024 * 1024 * 100, // 100MB
				LastModified: time.Now(),
				IsArchive:    true,
			},
		}

		files := []*EnhancedFile{
			{
				File: inventory.File{
					Name: "metadata.json",
					Size: 4096,
				},
			},
		}

		result := &BrowseResult{
			Path:        "/project/genomics/",
			Directories: directories,
			Files:       files,
			TotalFiles:  1,
			BrowseTime:  100 * time.Millisecond,
			HasMore:     false,
		}

		assert.Equal(t, "/project/genomics/", result.Path)
		assert.Len(t, result.Directories, 2)
		assert.Len(t, result.Files, 1)
		assert.Equal(t, 1, result.TotalFiles)
		assert.Equal(t, 100*time.Millisecond, result.BrowseTime)
		assert.False(t, result.HasMore)

		// Test directory properties
		rawDataDir := result.Directories[0]
		assert.Equal(t, "raw-data", rawDataDir.Name)
		assert.Equal(t, 25, rawDataDir.FileCount)
		assert.False(t, rawDataDir.IsArchive)

		archiveDir := result.Directories[1]
		assert.Equal(t, "results.tar.gz", archiveDir.Name)
		assert.True(t, archiveDir.IsArchive)
	})
}

// Performance benchmarks
func BenchmarkSearchFilter(b *testing.B) {
	now := time.Now()
	after := now.Add(-24 * time.Hour)
	before := now

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SearchFilter{
			NamePattern:         "*.fastq.gz",
			PathPattern:         "/genomics/*/",
			Extensions:          []string{".fastq.gz", ".bam", ".vcf"},
			MinSize:             1024 * 1024,
			MaxSize:             1024 * 1024 * 1024,
			ModifiedAfter:       &after,
			ModifiedBefore:      &before,
			ContentType:         "application/gzip",
			Tags:                map[string]string{"project": "test"},
			StorageClass:        "STANDARD",
			CompressionType:     "zstd",
			MinCompressionRatio: 0.5,
			MaxResults:          1000,
		}
	}
}

func BenchmarkSearchEngine(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bench_search")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			_ = err // Ignore cleanup errors in tests
		}
	}()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	indexer := NewIndexer(tempDir, logger)
	searchEngine := NewSearchEngine(indexer, logger)

	// Pre-create some test data
	ctx := context.Background()
	filter := SearchFilter{NamePattern: "*.txt"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchEngine.Search(ctx, filter)
	}
}
