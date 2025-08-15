package cmd

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/indexing"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestNewExtractCmd(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	
	assert.Equal(t, "extract [SOURCE] [TARGET]", cmd.Use)
	assert.Equal(t, "Extract specific files from archived data", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	
	// Check that all expected flags are present
	expectedFlags := []string{
		"pattern", "extensions", "min-size", "max-size", "after", "before",
		"path-pattern", "max-files",
		"preserve-structure", "flatten", "overwrite", "dry-run", "verify-checksums",
		"format", "show-progress", "verbose", "quiet",
		"concurrent-downloads", "chunk-size", "temp-dir",
		"inventory-directory", "index-cache-dir", "rebuild-index", "no-cache",
	}
	
	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "Flag %s should exist", flagName)
	}
}

func TestParseExtractOptions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	
	// Set some test flags
	_ = cmd.Flags().Set("preserve-structure", "true")
	_ = cmd.Flags().Set("flatten", "false")
	_ = cmd.Flags().Set("overwrite", "true")
	_ = cmd.Flags().Set("verify-checksums", "false")
	_ = cmd.Flags().Set("max-files", "50")
	_ = cmd.Flags().Set("show-progress", "true")
	_ = cmd.Flags().Set("verbose", "false")
	_ = cmd.Flags().Set("concurrent-downloads", "8")
	_ = cmd.Flags().Set("chunk-size", "16")
	_ = cmd.Flags().Set("pattern", "*.bam")
	
	options, err := parseExtractOptions(cmd, "/specific/file/path.txt")
	require.NoError(t, err)
	
	assert.True(t, options.PreserveStructure)
	assert.False(t, options.Flatten)
	assert.True(t, options.Overwrite)
	assert.False(t, options.VerifyChecksums)
	assert.Equal(t, 50, options.MaxFiles)
	assert.True(t, options.ShowProgress)
	assert.False(t, options.Verbose)
	assert.Equal(t, 8, options.ConcurrentDownloads)
	assert.Equal(t, 16, options.ChunkSizeMB)
	assert.Equal(t, "/specific/file/path.txt", options.SpecificPath)
	
	assert.NotNil(t, options.Filter)
	assert.Equal(t, "*.bam", options.Filter.NamePattern)
}

func TestParseExtractOptionsConflictingFlags(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	
	// Set conflicting flags (both flatten and preserve-structure)
	_ = cmd.Flags().Set("preserve-structure", "true")
	_ = cmd.Flags().Set("flatten", "true")
	
	options, err := parseExtractOptions(cmd, "")
	require.NoError(t, err)
	
	// Flatten should take precedence
	assert.True(t, options.Flatten)
	assert.False(t, options.PreserveStructure)
}

func TestParseExtractionFilter(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name      string
		flags     map[string]string
		sliceFlags map[string][]string
		wantNil   bool
	}{
		{
			name:    "no filters",
			flags:   map[string]string{},
			wantNil: true,
		},
		{
			name: "pattern filter",
			flags: map[string]string{
				"pattern": "*.vcf",
			},
			wantNil: false,
		},
		{
			name: "size filters",
			flags: map[string]string{
				"min-size": "1GB",
				"max-size": "50GB",
			},
			wantNil: false,
		},
		{
			name: "date filters",
			flags: map[string]string{
				"after":  "2024-06-01",
				"before": "2024-12-31",
			},
			wantNil: false,
		},
		{
			name: "extension filter",
			sliceFlags: map[string][]string{
				"extensions": {".fastq", ".fq.gz"},
			},
			wantNil: false,
		},
		{
			name: "max files limit",
			flags: map[string]string{
				"max-files": "25",
			},
			wantNil: false,
		},
		{
			name: "path pattern filter",
			flags: map[string]string{
				"path-pattern": "/results/*",
			},
			wantNil: false,
		},
		{
			name: "multiple filters",
			flags: map[string]string{
				"pattern":      "sample*",
				"min-size":     "500MB",
				"path-pattern": "/data/*",
			},
			sliceFlags: map[string][]string{
				"extensions": {".bam", ".sam"},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewExtractCmd()
			
			// Set string flags
			for flag, value := range tt.flags {
				_ = cmd.Flags().Set(flag, value)
			}
			
			// Set slice flags
			for flag, values := range tt.sliceFlags {
				for _, value := range values {
					_ = cmd.Flags().Set(flag, value)
				}
			}
			
			filter, err := parseExtractionFilter(cmd)
			require.NoError(t, err)
			
			if tt.wantNil {
				assert.Nil(t, filter)
			} else {
				assert.NotNil(t, filter)
				
				// Verify specific filter values if set
				if pattern := tt.flags["pattern"]; pattern != "" {
					assert.Equal(t, pattern, filter.NamePattern)
				}
				
				if pathPattern := tt.flags["path-pattern"]; pathPattern != "" {
					assert.Equal(t, pathPattern, filter.PathPattern)
				}
				
				if minSize := tt.flags["min-size"]; minSize != "" {
					assert.True(t, filter.MinSize > 0)
				}
				
				if maxSize := tt.flags["max-size"]; maxSize != "" {
					assert.True(t, filter.MaxSize > 0)
				}
				
				if extensions := tt.sliceFlags["extensions"]; len(extensions) > 0 {
					assert.Equal(t, extensions, filter.Extensions)
				}
				
				if maxFiles := tt.flags["max-files"]; maxFiles != "" {
					assert.True(t, filter.MaxResults > 0)
				}
			}
		})
	}
}

func TestParseExtractionFilterInvalidDates(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	_ = cmd.Flags().Set("after", "invalid-date")
	
	_, err := parseExtractionFilter(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid after date format")
}

func TestParseExtractionFilterInvalidSizes(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	_ = cmd.Flags().Set("min-size", "invalid-size")
	
	_, err := parseExtractionFilter(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid min-size")
}

func TestSourceParsing(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name             string
		source           string
		expectedArchive  string
		expectedSpecific string
	}{
		{
			name:             "S3 archive without specific path",
			source:           "s3://bucket/archive.tar.gz",
			expectedArchive:  "s3://bucket/archive.tar.gz",
			expectedSpecific: "",
		},
		{
			name:             "S3 archive with specific path",
			source:           "s3://bucket/archive.tar.gz:/path/to/file.txt",
			expectedArchive:  "s3://bucket/archive.tar.gz",
			expectedSpecific: "/path/to/file.txt",
		},
		{
			name:             "local archive without specific path",
			source:           "/path/to/archive.tar.gz",
			expectedArchive:  "/path/to/archive.tar.gz",
			expectedSpecific: "",
		},
		{
			name:             "local archive with specific path",
			source:           "/path/to/archive.tar.gz:/internal/file.txt",
			expectedArchive:  "/path/to/archive.tar.gz",
			expectedSpecific: "/internal/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse source for specific file path
			var archiveLocation string
			var specificPath string
			
			source := tt.source
			if strings.Contains(source, ":") && !strings.HasPrefix(source, "s3://") {
				// Handle local archive with specific path
				parts := strings.SplitN(source, ":", 2)
				archiveLocation = parts[0]
				specificPath = parts[1]
			} else if strings.Contains(source, ":") && strings.HasPrefix(source, "s3://") {
				// Handle S3 archive with specific path
				if colonIndex := strings.LastIndex(source, ":"); colonIndex > 6 { // After "s3://"
					archiveLocation = source[:colonIndex]
					specificPath = source[colonIndex+1:]
				} else {
					archiveLocation = source
				}
			} else {
				archiveLocation = source
			}
			
			assert.Equal(t, tt.expectedArchive, archiveLocation)
			assert.Equal(t, tt.expectedSpecific, specificPath)
		})
	}
}

func TestEstimateExtractionTime(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "large_dataset.bam",
				Size: 200 * 1024 * 1024, // 200MB
			},
		},
	}

	duration := estimateExtractionTime(files)
	
	// Should take about 10 seconds for 200MB at 20MB/s
	assert.True(t, duration >= 9*time.Second && duration <= 11*time.Second)
}

func TestExtractEngine(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "extract_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	testLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	engine := &ExtractEngine{
		indexer:      indexing.NewIndexer(tempDir, testLogger),
		searchEngine: indexing.NewSearchEngine(indexing.NewIndexer(tempDir, testLogger), testLogger),
		logger:       testLogger,
	}
	
	assert.NotNil(t, engine.indexer)
	assert.NotNil(t, engine.searchEngine)
	assert.NotNil(t, engine.logger)
}

func TestExtractStructures(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Test ExtractOptions
	options := &ExtractOptions{
		PreserveStructure:   true,
		Flatten:            false,
		Overwrite:          true,
		VerifyChecksums:    true,
		MaxFiles:           100,
		ShowProgress:       true,
		Verbose:           false,
		ConcurrentDownloads: 8,
		ChunkSizeMB:        16,
		SpecificPath:      "/data/results.json",
	}
	
	assert.True(t, options.PreserveStructure)
	assert.False(t, options.Flatten)
	assert.True(t, options.Overwrite)
	assert.Equal(t, 100, options.MaxFiles)
	assert.Equal(t, "/data/results.json", options.SpecificPath)
	
	// Test ExtractionPreview
	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{Name: "test.fastq", Size: 2048},
		},
	}
	
	preview := &ExtractionPreview{
		ArchiveLocation:   "s3://test-bucket/archive.tar.gz",
		Destination:       "/local/path",
		Files:             files,
		TotalFiles:        1,
		TotalSize:         2048,
		PreviewTime:       time.Now(),
		EstimatedTime:     time.Minute,
		RequiredSpace:     2048,
		PreserveStructure: true,
		Flatten:           false,
	}
	
	assert.Equal(t, "s3://test-bucket/archive.tar.gz", preview.ArchiveLocation)
	assert.Equal(t, "/local/path", preview.Destination)
	assert.Equal(t, 1, preview.TotalFiles)
	assert.Equal(t, int64(2048), preview.TotalSize)
	assert.True(t, preview.PreserveStructure)
	
	// Test ExtractionResult
	result := &ExtractionResult{
		ArchiveLocation: "s3://test-bucket/data.tar.gz",
		Destination:     "/output/directory",
		Files:           files,
		TotalFiles:      1,
		TotalSize:       2048,
		StartTime:       time.Now(),
		EndTime:         time.Now().Add(time.Minute),
		Success:         true,
		ExtractedFiles:  1,
		SkippedFiles:    0,
		FailedFiles:     0,
	}
	
	assert.Equal(t, "s3://test-bucket/data.tar.gz", result.ArchiveLocation)
	assert.Equal(t, "/output/directory", result.Destination)
	assert.Equal(t, 1, result.TotalFiles)
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.ExtractedFiles)
}

// Integration test that tests extract command help and flag parsing
func TestExtractCommandIntegration(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewExtractCmd()
	
	// Test help output
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	
	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "Extract individual files")
	assert.Contains(t, helpOutput, "--pattern")
	assert.Contains(t, helpOutput, "--dry-run")
	assert.Contains(t, helpOutput, "--preserve-structure")
	
	// Test invalid arguments
	buf.Reset() 
	cmd = NewExtractCmd() // Create new command to avoid state from previous test
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{}) // No arguments should fail
	err = cmd.Execute()
	assert.Error(t, err) // Should require at least one argument
}

// Test the extract command structure and flag validation
func TestExtractCommandWithDryRun(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "extract_dry_run_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	// Create a test inventory file
	inventoryFile := filepath.Join(tempDir, "test-inventory.yaml")
	
	// Write inventory to YAML file (simplified - in real use we'd use proper YAML marshaling)
	yamlContent := `
files:
  - path: /test/genomics/sample1.fastq.gz
    destination: genomics/sample1.fastq.gz
    name: sample1.fastq.gz
    size: 104857600
    suitcase_index: 1
    suitcase_name: test-extract-01-of-01.tar.zst
  - path: /test/results/analysis.bam
    destination: results/analysis.bam
    name: analysis.bam
    size: 209715200
    suitcase_index: 1
    suitcase_name: test-extract-01-of-01.tar.zst
  - path: /test/docs/metadata.json
    destination: docs/metadata.json
    name: metadata.json
    size: 16384
    suitcase_index: 1
    suitcase_name: test-extract-01-of-01.tar.zst
total_indexes: 1
options:
  user: testuser
  prefix: test-extract
  max_suitcase_size: 1073741824
  suitcase_format: tar.zst
`
	
	err = os.WriteFile(inventoryFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Test extract dry-run command structure (don't actually execute due to dependencies)
	cmd := NewExtractCmd()
	
	// Set up command with test parameters
	_ = cmd.Flags().Set("inventory-directory", tempDir)
	_ = cmd.Flags().Set("index-cache-dir", filepath.Join(tempDir, "cache"))
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("pattern", "*.fastq.gz")
	_ = cmd.Flags().Set("format", "table")
	
	// Verify flags are set correctly
	inventoryDir, _ := cmd.Flags().GetStringArray("inventory-directory")
	assert.Contains(t, inventoryDir, tempDir)
	
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	assert.True(t, dryRun)
	
	pattern, _ := cmd.Flags().GetString("pattern")
	assert.Equal(t, "*.fastq.gz", pattern)
	
	format, _ := cmd.Flags().GetString("format")
	assert.Equal(t, "table", format)
}

func TestGetFilesToExtract(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "get_files_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	testLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	engine := &ExtractEngine{
		indexer:      indexing.NewIndexer(tempDir, testLogger),
		searchEngine: indexing.NewSearchEngine(indexing.NewIndexer(tempDir, testLogger), testLogger),
		logger:       testLogger,
	}

	// Test with specific path
	options := &ExtractOptions{
		SpecificPath: "/test/specific/file.txt",
	}

	// This would fail without a proper index, but we can test the structure
	_, err = getFilesToExtract(context.Background(), engine, "test://location", options)
	assert.Error(t, err) // Expected since we don't have a real index
	assert.Contains(t, err.Error(), "failed to load archive index")
}

// Test helper functions used in extract.go
func TestExtractHelperFunctions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "file1.txt",
				Size: 100 * 1024 * 1024, // 100MB to ensure non-zero duration
			},
		},
		{
			File: inventory.File{
				Name: "file2.txt",
				Size: 200 * 1024 * 1024, // 200MB
			},
		},
	}

	// Test calculateTotalSize (reused from other commands)
	total := calculateTotalSize(files)
	expectedTotal := int64(300 * 1024 * 1024) // 100MB + 200MB = 300MB
	assert.Equal(t, expectedTotal, total)

	// Test calculateRequiredSpace (reused from other commands)
	space := calculateRequiredSpace(files, true)
	assert.Equal(t, expectedTotal, space) // Same as total size for now

	// Test estimateExtractionTime
	duration := estimateExtractionTime(files)
	assert.Greater(t, duration, time.Duration(0))

	// Test humanizeBytes (reused from other commands)
	human := humanizeBytes(expectedTotal)
	assert.Equal(t, "300.0 MB", human)
}

func TestExtractDisplayFunctions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{Name: "test.txt", Size: 1024},
		},
	}

	preview := &ExtractionPreview{
		ArchiveLocation:   "test://archive",
		Destination:       "/output",
		Files:             files,
		TotalFiles:        1,
		TotalSize:         1024,
		PreviewTime:       time.Now(),
		EstimatedTime:     time.Second,
		RequiredSpace:     1024,
		PreserveStructure: true,
		Flatten:           false,
	}

	cmd := NewExtractCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Test table display
	err := displayExtractionPreview(preview, cmd)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Extraction Preview")
	assert.Contains(t, output, "test://archive")
	assert.Contains(t, output, "/output")
	assert.Contains(t, output, "Files to extract: 1")

	// Test JSON display
	buf.Reset()
	_ = cmd.Flags().Set("format", "json")
	
	err = displayExtractionPreview(preview, cmd)
	require.NoError(t, err)
	
	jsonOutput := buf.String()
	assert.Contains(t, jsonOutput, "archivelocation")
	assert.Contains(t, jsonOutput, "test://archive")
}