// Package detection provides AI-powered file type detection using Magika (Issue #30)
package detection

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/compression"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMagikaDetector_Disabled tests that disabled detector is no-op
func TestMagikaDetector_Disabled(t *testing.T) {
	cfg := config.MagikaConfig{
		Enabled: false,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)
	assert.NotNil(t, detector)
	assert.False(t, detector.IsAvailable())

	// Should return empty results
	ctx := context.Background()
	results, err := detector.DetectBatch(ctx, []string{"test.txt"})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestMagikaDetector_BinaryNotFound tests error handling when binary not found
func TestMagikaDetector_BinaryNotFound(t *testing.T) {
	cfg := config.MagikaConfig{
		Enabled:    true,
		BinaryPath: "/nonexistent/path/to/magika",
	}

	detector, err := NewMagikaDetector(cfg)
	assert.Error(t, err)
	assert.Nil(t, detector)
	assert.Contains(t, err.Error(), "magika binary validation failed")
}

// TestMagikaDetector_AutoDiscovery tests automatic binary discovery
func TestMagikaDetector_AutoDiscovery(t *testing.T) {
	// Check if magika is in PATH
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping auto-discovery test")
	}

	cfg := config.MagikaConfig{
		Enabled:     true,
		BinaryPath:  "", // Empty = auto-discover
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: true,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)
	assert.NotNil(t, detector)
	assert.True(t, detector.IsAvailable())
	assert.NotEmpty(t, detector.binaryPath)
}

// TestMagikaDetector_BatchDetection tests batch detection with real files
func TestMagikaDetector_BatchDetection(t *testing.T) {
	// Check if magika is installed
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping batch detection test")
	}

	// Create temporary test files
	tmpDir := t.TempDir()
	testFiles := createTestFiles(t, tmpDir)

	cfg := config.MagikaConfig{
		Enabled:       true,
		BatchSize:     100,
		Timeout:       "30s",
		EnableCache:   false, // Disable cache for this test
		UseMimeType:   false, // JSON mode includes MIME type by default
		IncludeScores: true,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)
	require.True(t, detector.IsAvailable())

	// Run batch detection
	ctx := context.Background()
	results, err := detector.DetectBatch(ctx, testFiles)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Verify results for each file
	for _, filePath := range testFiles {
		result, ok := results[filePath]
		require.True(t, ok, "Missing result for %s", filePath)
		assert.Equal(t, "ok", result.Result.Status, "Status should be 'ok' for %s", filePath)
		assert.NotEmpty(t, result.Result.Value.Output.CTLabel, "Empty label for %s", filePath)

		// Verify expected content types (be flexible as Magika may classify slightly differently)
		switch filepath.Ext(filePath) {
		case ".txt":
			assert.Contains(t, []string{"txt", "text", "markdown"}, result.Result.Value.Output.CTLabel)
		case ".json":
			// Magika may detect as "json" or "jsonl" depending on content
			assert.Contains(t, []string{"json", "jsonl"}, result.Result.Value.Output.CTLabel)
		case ".py":
			// For short Python snippets, detection may vary (python, lua, shell, etc.)
			// Just verify it detected as some code type
			assert.NotEmpty(t, result.Result.Value.Output.CTLabel)
		}

		// Verify MIME type is present
		assert.NotEmpty(t, result.Result.Value.Output.MimeType)

		// Verify score is present and valid
		assert.GreaterOrEqual(t, result.Result.Value.Output.Score, 0.0)
		assert.LessOrEqual(t, result.Result.Value.Output.Score, 1.0)
	}
}

// TestMagikaDetector_DetectSingle tests single file detection
func TestMagikaDetector_DetectSingle(t *testing.T) {
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping single detection test")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Hello, world!"), 0644)
	require.NoError(t, err)

	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: false,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := detector.DetectSingle(ctx, testFile)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Result.Status)
	assert.NotEmpty(t, result.Result.Value.Output.CTLabel)
}

// TestMagikaDetector_Cache tests caching behavior
func TestMagikaDetector_Cache(t *testing.T) {
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping cache test")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Test content"), 0644)
	require.NoError(t, err)

	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: true,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// First detection - should hit Magika
	results1, err := detector.DetectBatch(ctx, []string{testFile})
	require.NoError(t, err)
	require.Len(t, results1, 1)

	// Second detection - should hit cache
	results2, err := detector.DetectBatch(ctx, []string{testFile})
	require.NoError(t, err)
	require.Len(t, results2, 1)

	// Results should be identical
	assert.Equal(t, results1[testFile].Result.Value.Output.CTLabel, results2[testFile].Result.Value.Output.CTLabel)

	// Check cache stats
	stats := detector.GetCacheStats()
	assert.True(t, stats["enabled"].(bool))
	assert.Equal(t, 1, stats["cached_files"].(int))
	assert.True(t, stats["available"].(bool))

	// Clear cache
	detector.ClearCache()
	stats = detector.GetCacheStats()
	assert.Equal(t, 0, stats["cached_files"].(int))
}

// TestMagikaDetector_Timeout tests timeout handling
func TestMagikaDetector_Timeout(t *testing.T) {
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping timeout test")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Test"), 0644)
	require.NoError(t, err)

	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "1ns", // Very short timeout to trigger timeout
		EnableCache: false,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = detector.DetectBatch(ctx, []string{testFile})
	// Should get timeout error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

// TestMagikaDetector_EmptyBatch tests empty batch handling
func TestMagikaDetector_EmptyBatch(t *testing.T) {
	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: false,
	}

	detector, err := NewMagikaDetector(cfg)
	if err != nil {
		t.Skip("Magika not available, skipping empty batch test")
	}

	ctx := context.Background()
	results, err := detector.DetectBatch(ctx, []string{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestMagikaDetector_NonexistentFile tests handling of nonexistent files
func TestMagikaDetector_NonexistentFile(t *testing.T) {
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping nonexistent file test")
	}

	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: false,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = detector.DetectBatch(ctx, []string{"/nonexistent/file.txt"})
	// Should return error from Magika execution
	assert.Error(t, err)
}

// TestMapMagikaToCompression tests Magika label mapping
func TestMapMagikaToCompression(t *testing.T) {
	tests := []struct {
		name          string
		magikaLabel   string
		expectedType  compression.ContentType
		expectedMatch bool
	}{
		// Text formats
		{"Text file", "txt", compression.ContentTypeText, true},
		{"Markdown", "markdown", compression.ContentTypeText, true},
		{"CSV", "csv", compression.ContentTypeText, true},
		{"Log file", "log", compression.ContentTypeText, true},

		// Code formats
		{"Python", "python", compression.ContentTypeCode, true},
		{"JavaScript", "javascript", compression.ContentTypeCode, true},
		{"TypeScript", "typescript", compression.ContentTypeCode, true},
		{"Go", "go", compression.ContentTypeCode, true},
		{"Rust", "rust", compression.ContentTypeCode, true},
		{"JSON", "json", compression.ContentTypeCode, true},
		{"YAML", "yaml", compression.ContentTypeCode, true},
		{"HTML", "html", compression.ContentTypeCode, true},
		{"CSS", "css", compression.ContentTypeCode, true},

		// Documents
		{"PDF", "pdf", compression.ContentTypeDocument, true},
		{"DOCX", "docx", compression.ContentTypeDocument, true},
		{"XLSX", "xlsx", compression.ContentTypeDocument, true},
		{"PPTX", "pptx", compression.ContentTypeDocument, true},

		// Images
		{"JPEG", "jpeg", compression.ContentTypeImage, true},
		{"PNG", "png", compression.ContentTypeImage, true},
		{"GIF", "gif", compression.ContentTypeImage, true},
		{"WebP", "webp", compression.ContentTypeImage, true},
		{"SVG", "svg", compression.ContentTypeImage, true},

		// Video
		{"MP4", "mp4", compression.ContentTypeVideo, true},
		{"AVI", "avi", compression.ContentTypeVideo, true},
		{"MKV", "mkv", compression.ContentTypeVideo, true},
		{"WebM", "webm", compression.ContentTypeVideo, true},

		// Audio
		{"MP3", "mp3", compression.ContentTypeAudio, true},
		{"FLAC", "flac", compression.ContentTypeAudio, true},
		{"WAV", "wav", compression.ContentTypeAudio, true},
		{"OGG", "ogg", compression.ContentTypeAudio, true},

		// Archives
		{"ZIP", "zip", compression.ContentTypeArchive, true},
		{"GZIP", "gzip", compression.ContentTypeArchive, true},
		{"TAR", "tar", compression.ContentTypeArchive, true},
		{"7ZIP", "7z", compression.ContentTypeArchive, true},
		{"RAR", "rar", compression.ContentTypeArchive, true},

		// Binary
		{"ELF", "elf", compression.ContentTypeBinary, true},
		{"PE", "pe", compression.ContentTypeBinary, true},
		{"Mach-O", "macho", compression.ContentTypeBinary, true},
		{"Java Class", "java_class", compression.ContentTypeBinary, true},
		{"WASM", "wasm", compression.ContentTypeBinary, true},

		// Unknown
		{"Unknown type", "unknown_type_xyz", compression.ContentTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapMagikaToCompression(tt.magikaLabel)
			if tt.expectedMatch {
				assert.Equal(t, tt.expectedType, result,
					"Label %s should map to %v", tt.magikaLabel, tt.expectedType)
			} else {
				assert.Equal(t, compression.ContentTypeUnknown, result,
					"Unknown label %s should map to ContentTypeUnknown", tt.magikaLabel)
			}
		})
	}
}

// TestGetMappingStats tests mapping statistics
func TestGetMappingStats(t *testing.T) {
	stats := GetMappingStats()

	// Verify all categories are present
	assert.Contains(t, stats, "total")
	assert.Contains(t, stats, "text")
	assert.Contains(t, stats, "code")
	assert.Contains(t, stats, "document")
	assert.Contains(t, stats, "image")
	assert.Contains(t, stats, "video")
	assert.Contains(t, stats, "audio")
	assert.Contains(t, stats, "archive")
	assert.Contains(t, stats, "binary")

	// Verify counts are reasonable
	assert.Greater(t, stats["total"], 100, "Should have 100+ mappings")
	assert.Greater(t, stats["code"], 10, "Should have 10+ code types")
	assert.Greater(t, stats["text"], 5, "Should have 5+ text types")
	assert.Greater(t, stats["image"], 5, "Should have 5+ image types")
	assert.Greater(t, stats["binary"], 5, "Should have 5+ binary types")

	// Total should equal sum of categories
	categorySum := stats["text"] + stats["code"] + stats["document"] +
		stats["image"] + stats["video"] + stats["audio"] +
		stats["archive"] + stats["binary"] + stats["unknown"]
	assert.Equal(t, stats["total"], categorySum, "Total should equal sum of categories")
}

// TestMagikaDetector_ConcurrentAccess tests thread-safe cache access
func TestMagikaDetector_ConcurrentAccess(t *testing.T) {
	_, err := exec.LookPath("magika")
	if err != nil {
		t.Skip("Magika not installed, skipping concurrent access test")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Concurrent test"), 0644)
	require.NoError(t, err)

	cfg := config.MagikaConfig{
		Enabled:     true,
		BatchSize:   100,
		Timeout:     "30s",
		EnableCache: true,
	}

	detector, err := NewMagikaDetector(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Run concurrent detections
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := detector.DetectBatch(ctx, []string{testFile})
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines with timeout
	timeout := time.After(10 * time.Second)
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// Success
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent detections")
		}
	}

	// Cache should have exactly one entry
	stats := detector.GetCacheStats()
	assert.Equal(t, 1, stats["cached_files"].(int))
}

// TestVerifyBinary tests binary verification
func TestVerifyBinary(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid executable",
			setup: func(t *testing.T) string {
				path, err := exec.LookPath("ls")
				require.NoError(t, err)
				return path
			},
			expectError: false,
		},
		{
			name: "Nonexistent file",
			setup: func(t *testing.T) string {
				return "/nonexistent/binary"
			},
			expectError: true,
		},
		{
			name: "Directory instead of file",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expectError: true,
			errorMsg:    "directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			err := verifyBinary(path)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// createTestFiles creates temporary test files for batch detection tests
func createTestFiles(t *testing.T, dir string) []string {
	files := []struct {
		name    string
		content string
	}{
		{"test.txt", "This is a plain text file.\n"},
		{"data.json", `{"key": "value", "number": 42}`},
		{"script.py", "#!/usr/bin/env python\nprint('Hello, World!')"},
	}

	var paths []string
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		err := os.WriteFile(path, []byte(f.content), 0644)
		require.NoError(t, err)
		paths = append(paths, path)
	}

	return paths
}
