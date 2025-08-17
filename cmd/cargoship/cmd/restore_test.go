package cmd

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/indexing"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestNewRestoreCmd(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewRestoreCmd()

	assert.Equal(t, "restore [LOCATION] [TARGET]", cmd.Use)
	assert.Equal(t, "Restore archived data with preview and cost estimation", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Check that all expected flags are present
	expectedFlags := []string{
		"preview", "estimate-cost", "list-contents", "dry-run",
		"pattern", "extensions", "min-size", "max-size", "after", "before",
		"path-pattern", "preserve-structure", "max-files",
		"format", "show-metadata", "show-checksums", "verbose-progress",
		"storage-class", "region", "include-transfer-costs", "show-cost-breakdown",
		"inventory-directory", "index-cache-dir", "rebuild-index", "no-cache",
	}

	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "Flag %s should exist", flagName)
	}
}

func TestParseRestoreOptions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewRestoreCmd()

	// Set some test flags
	_ = cmd.Flags().Set("preserve-structure", "true")
	_ = cmd.Flags().Set("show-metadata", "true")
	_ = cmd.Flags().Set("show-checksums", "false")
	_ = cmd.Flags().Set("max-files", "100")
	_ = cmd.Flags().Set("storage-class", "GLACIER")
	_ = cmd.Flags().Set("region", "us-west-2")
	_ = cmd.Flags().Set("include-transfer-costs", "false")
	_ = cmd.Flags().Set("pattern", "*.fastq")

	options, err := parseRestoreOptions(cmd)
	require.NoError(t, err)

	assert.True(t, options.PreserveStructure)
	assert.True(t, options.ShowMetadata)
	assert.False(t, options.ShowChecksums)
	assert.Equal(t, 100, options.MaxFiles)
	assert.Equal(t, "GLACIER", options.StorageClass)
	assert.Equal(t, "us-west-2", options.Region)
	assert.False(t, options.IncludeTransferCosts)

	assert.NotNil(t, options.Filter)
	assert.Equal(t, "*.fastq", options.Filter.NamePattern)
}

func TestParseRestoreFilter(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name       string
		flags      map[string]string
		sliceFlags map[string][]string
		wantNil    bool
	}{
		{
			name:    "no filters",
			flags:   map[string]string{},
			wantNil: true,
		},
		{
			name: "pattern filter",
			flags: map[string]string{
				"pattern": "*.bam",
			},
			wantNil: false,
		},
		{
			name: "size filters",
			flags: map[string]string{
				"min-size": "500MB",
				"max-size": "10GB",
			},
			wantNil: false,
		},
		{
			name: "date filters",
			flags: map[string]string{
				"after":  "2024-01-01",
				"before": "2024-12-31",
			},
			wantNil: false,
		},
		{
			name: "extension filter",
			sliceFlags: map[string][]string{
				"extensions": {".bam", ".sam"},
			},
			wantNil: false,
		},
		{
			name: "max files limit",
			flags: map[string]string{
				"max-files": "50",
			},
			wantNil: false,
		},
		{
			name: "multiple filters",
			flags: map[string]string{
				"pattern":      "analysis*",
				"min-size":     "100MB",
				"path-pattern": "/results/*",
			},
			sliceFlags: map[string][]string{
				"extensions": {".json", ".csv"},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRestoreCmd()

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

			filter, err := parseRestoreFilter(cmd)
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

func TestParseRestoreFilterInvalidDates(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewRestoreCmd()
	_ = cmd.Flags().Set("after", "invalid-date")

	_, err := parseRestoreFilter(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid after date format")
}

func TestParseRestoreFilterInvalidSizes(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewRestoreCmd()
	_ = cmd.Flags().Set("min-size", "invalid-size")

	_, err := parseRestoreFilter(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid min-size")
}

func TestCalculateTotalSize(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "file1.txt",
				Size: 1024,
			},
		},
		{
			File: inventory.File{
				Name: "file2.txt",
				Size: 2048,
			},
		},
		{
			File: inventory.File{
				Name: "file3.txt",
				Size: 512,
			},
		},
	}

	total := calculateTotalSize(files)
	assert.Equal(t, int64(3584), total) // 1024 + 2048 + 512
}

func TestEstimateRestoreTime(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "large_file.bin",
				Size: 100 * 1024 * 1024, // 100MB
			},
		},
	}

	duration := estimateRestoreTime(files)

	// Should take about 10 seconds for 100MB at 10MB/s
	assert.True(t, duration >= 9*time.Second && duration <= 11*time.Second)
}

func TestCalculateRequiredSpace(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "file1.txt",
				Size: 1024,
			},
		},
		{
			File: inventory.File{
				Name: "file2.txt",
				Size: 2048,
			},
		},
	}

	// For now, required space equals total size
	space := calculateRequiredSpace(files, true)
	assert.Equal(t, int64(3072), space) // 1024 + 2048

	space = calculateRequiredSpace(files, false)
	assert.Equal(t, int64(3072), space) // Same for now
}

func TestCalculateStorageCosts(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "test_file.txt",
				Size: 1024 * 1024 * 1024, // 1GB
			},
		},
	}

	// Test different storage classes
	standardCost := calculateStorageCosts(files, "STANDARD", "us-east-1")
	assert.True(t, standardCost > 0)

	glacierCost := calculateStorageCosts(files, "GLACIER", "us-east-1")
	assert.True(t, glacierCost > 0)
	assert.True(t, glacierCost < standardCost) // Glacier should be cheaper

	deepArchiveCost := calculateStorageCosts(files, "DEEP_ARCHIVE", "us-east-1")
	assert.True(t, deepArchiveCost > 0)
	assert.True(t, deepArchiveCost < glacierCost) // Deep Archive should be cheapest
}

func TestCalculateTransferCosts(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Test small files (under 1GB free tier)
	smallFiles := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "small_file.txt",
				Size: 512 * 1024 * 1024, // 512MB
			},
		},
	}

	smallCost := calculateTransferCosts(smallFiles, "us-east-1")
	assert.Equal(t, 0.0, smallCost) // Should be free

	// Test large files (over 1GB)
	largeFiles := []*indexing.EnhancedFile{
		{
			File: inventory.File{
				Name: "large_file.bin",
				Size: 2 * 1024 * 1024 * 1024, // 2GB
			},
		},
	}

	largeCost := calculateTransferCosts(largeFiles, "us-east-1")
	assert.True(t, largeCost > 0) // Should have cost for 1GB over the free tier
}

func TestRestoreStructures(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Test RestoreOptions
	options := &RestoreOptions{
		PreserveStructure:    true,
		ShowMetadata:         false,
		MaxFiles:             100,
		StorageClass:         "STANDARD",
		Region:               "us-west-2",
		IncludeTransferCosts: true,
	}

	assert.True(t, options.PreserveStructure)
	assert.False(t, options.ShowMetadata)
	assert.Equal(t, 100, options.MaxFiles)
	assert.Equal(t, "STANDARD", options.StorageClass)

	// Test RestorePreview
	files := []*indexing.EnhancedFile{
		{
			File: inventory.File{Name: "test.txt", Size: 1024},
		},
	}

	preview := &RestorePreview{
		Location:      "s3://test-bucket/archive.tar.gz",
		Destination:   "/local/path",
		Files:         files,
		TotalFiles:    1,
		TotalSize:     1024,
		PreviewTime:   time.Now(),
		EstimatedTime: time.Minute,
		RequiredSpace: 1024,
	}

	assert.Equal(t, "s3://test-bucket/archive.tar.gz", preview.Location)
	assert.Equal(t, "/local/path", preview.Destination)
	assert.Equal(t, 1, preview.TotalFiles)
	assert.Equal(t, int64(1024), preview.TotalSize)

	// Test RestoreCostEstimate
	estimate := &RestoreCostEstimate{
		Location:     "s3://test-bucket/data/",
		Files:        files,
		TotalFiles:   1,
		TotalSize:    1024,
		Region:       "us-east-1",
		StorageCost:  0.023,
		TransferCost: 0.0,
		TotalCost:    0.023,
	}

	assert.Equal(t, "s3://test-bucket/data/", estimate.Location)
	assert.Equal(t, 1, estimate.TotalFiles)
	assert.Equal(t, 0.023, estimate.StorageCost)
	assert.Equal(t, 0.023, estimate.TotalCost)

	// Test ArchiveContents
	contents := &ArchiveContents{
		Location:     "s3://bucket/archive.tar.gz",
		Files:        files,
		TotalFiles:   1,
		TotalSize:    1024,
		IndexVersion: "1.0",
		CreatedAt:    time.Now(),
	}

	assert.Equal(t, "s3://bucket/archive.tar.gz", contents.Location)
	assert.Equal(t, 1, contents.TotalFiles)
	assert.Equal(t, "1.0", contents.IndexVersion)
}

func TestRestoreEngine(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tempDir, err := os.MkdirTemp("", "restore_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	testLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	engine := &RestoreEngine{
		indexer:      indexing.NewIndexer(tempDir, testLogger),
		searchEngine: indexing.NewSearchEngine(indexing.NewIndexer(tempDir, testLogger), testLogger),
		logger:       testLogger,
	}

	assert.NotNil(t, engine.indexer)
	assert.NotNil(t, engine.searchEngine)
	assert.NotNil(t, engine.logger)
}

// Integration test that tests restore command help and flag parsing
func TestRestoreCommandIntegration(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cmd := NewRestoreCmd()

	// Test help output
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "preview")
	assert.Contains(t, helpOutput, "--preview")
	assert.Contains(t, helpOutput, "--estimate-cost")
	assert.Contains(t, helpOutput, "--list-contents")

	// Test invalid arguments
	buf.Reset()
	cmd = NewRestoreCmd() // Create new command to avoid state from previous test
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{}) // No arguments should fail
	err = cmd.Execute()
	assert.Error(t, err) // Should require at least one argument
}

// Test the actual restore command execution (preview mode)
func TestRestoreCommandWithPreview(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "restore_preview_test")
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
  - path: /test/data/sequence.fastq.gz
    destination: data/sequence.fastq.gz
    name: sequence.fastq.gz
    size: 52428800
    suitcase_index: 1
    suitcase_name: test-restore-01-of-01.tar.zst
  - path: /test/results/analysis.json
    destination: results/analysis.json
    name: analysis.json
    size: 8192
    suitcase_index: 1
    suitcase_name: test-restore-01-of-01.tar.zst
  - path: /test/docs/README.md
    destination: docs/README.md
    name: README.md
    size: 4096
    suitcase_index: 1
    suitcase_name: test-restore-01-of-01.tar.zst
total_indexes: 1
options:
  user: testuser
  prefix: test-restore
  max_suitcase_size: 1073741824
  suitcase_format: tar.zst
`

	err = os.WriteFile(inventoryFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Test restore preview command structure (don't actually execute due to dependencies)
	cmd := NewRestoreCmd()

	// Set up command with test parameters
	_ = cmd.Flags().Set("inventory-directory", tempDir)
	_ = cmd.Flags().Set("index-cache-dir", filepath.Join(tempDir, "cache"))
	_ = cmd.Flags().Set("preview", "true")
	_ = cmd.Flags().Set("format", "table")

	// Verify flags are set correctly
	inventoryDir, _ := cmd.Flags().GetStringArray("inventory-directory")
	assert.Contains(t, inventoryDir, tempDir)

	preview, _ := cmd.Flags().GetBool("preview")
	assert.True(t, preview)

	format, _ := cmd.Flags().GetString("format")
	assert.Equal(t, "table", format)
}

// Test helper functions used in restore.go
func TestParseSizeRestore(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{
			name:     "bytes",
			input:    "2048",
			expected: 2048,
		},
		{
			name:     "megabytes",
			input:    "50MB",
			expected: 50 * 1024 * 1024,
		},
		{
			name:     "gigabytes",
			input:    "5GB",
			expected: 5 * 1024 * 1024 * 1024,
		},
		{
			name:     "fractional gigabytes",
			input:    "2.5GB",
			expected: int64(2.5 * 1024 * 1024 * 1024),
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "unknown unit",
			input:   "5PB",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSize(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestHumanizeBytesRestore(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "bytes",
			bytes:    512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			bytes:    1024,
			expected: "1.0 KB",
		},
		{
			name:     "megabytes",
			bytes:    1024 * 1024,
			expected: "1.0 MB",
		},
		{
			name:     "gigabytes",
			bytes:    2 * 1024 * 1024 * 1024,
			expected: "2.0 GB",
		},
		{
			name:     "fractional",
			bytes:    1536, // 1.5 KB
			expected: "1.5 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := humanizeBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
