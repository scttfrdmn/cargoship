package extraction

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

// mockS3Client mocks S3 GetObject for testing
type mockS3Client struct {
	body   io.ReadCloser
	getErr error
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return &s3.GetObjectOutput{
		Body: m.body,
	}, nil
}

// createTestTarGz creates a test tar.gz archive in memory
func createTestTarGz(t *testing.T, files map[string]string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

// createTestTarZst creates a test tar.zst archive in memory
func createTestTarZst(t *testing.T, files map[string]string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	zstWriter, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("Failed to create zstd writer: %v", err)
	}
	tarWriter := tar.NewWriter(zstWriter)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := zstWriter.Close(); err != nil {
		t.Fatalf("Failed to close zstd writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

// createTestTar creates a test uncompressed tar archive in memory
func createTestTar(t *testing.T, files map[string]string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	tarWriter := tar.NewWriter(&buf)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

// createTestTarWithDirectory creates a test tar.gz archive with directories
func createTestTarWithDirectory(t *testing.T) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add directory
	dirHeader := &tar.Header{
		Name:     "testdir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tarWriter.WriteHeader(dirHeader); err != nil {
		t.Fatalf("Failed to write directory header: %v", err)
	}

	// Add file in directory
	content := "file in directory"
	fileHeader := &tar.Header{
		Name: "testdir/file.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(fileHeader); err != nil {
		t.Fatalf("Failed to write file header: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write file content: %v", err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

// createTestTarWithSymlink creates a test tar.gz archive with a symlink
func createTestTarWithSymlink(t *testing.T) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add regular file
	content := "target file"
	fileHeader := &tar.Header{
		Name: "target.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(fileHeader); err != nil {
		t.Fatalf("Failed to write file header: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write file content: %v", err)
	}

	// Add symlink
	symlinkHeader := &tar.Header{
		Name:     "link.txt",
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: "target.txt",
	}
	if err := tarWriter.WriteHeader(symlinkHeader); err != nil {
		t.Fatalf("Failed to write symlink header: %v", err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

// createTestTarWithPathTraversal creates a malicious tar with path traversal attempt
func createTestTarWithPathTraversal(t *testing.T) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add file with path traversal
	content := "malicious content"
	header := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

func TestNewExtractor(t *testing.T) {
	tests := []struct {
		name        string
		config      *ExtractorConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "config cannot be nil",
		},
		{
			name: "nil S3 client",
			config: &ExtractorConfig{
				S3Client:  nil,
				Bucket:    "test-bucket",
				Key:       "test.tar.gz",
				OutputDir: "/tmp/output",
			},
			expectError: true,
			errorMsg:    "S3Client cannot be nil",
		},
		{
			name: "empty bucket",
			config: &ExtractorConfig{
				S3Client:  &mockS3Client{},
				Bucket:    "",
				Key:       "test.tar.gz",
				OutputDir: "/tmp/output",
			},
			expectError: true,
			errorMsg:    "bucket cannot be empty",
		},
		{
			name: "empty key",
			config: &ExtractorConfig{
				S3Client:  &mockS3Client{},
				Bucket:    "test-bucket",
				Key:       "",
				OutputDir: "/tmp/output",
			},
			expectError: true,
			errorMsg:    "key cannot be empty",
		},
		{
			name: "empty output directory",
			config: &ExtractorConfig{
				S3Client:  &mockS3Client{},
				Bucket:    "test-bucket",
				Key:       "test.tar.gz",
				OutputDir: "",
			},
			expectError: true,
			errorMsg:    "output directory cannot be empty",
		},
		{
			name: "valid config",
			config: &ExtractorConfig{
				S3Client:  &mockS3Client{},
				Bucket:    "test-bucket",
				Key:       "test.tar.gz",
				OutputDir: "/tmp/output",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor, err := NewExtractor(tt.config)
			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if extractor == nil {
					t.Fatal("Expected extractor, got nil")
				}
			}
		})
	}
}

func TestDetectCompressionFormat(t *testing.T) {
	tests := []struct {
		key      string
		expected CompressionFormat
	}{
		{"archive.tar.gz", CompressionGzip},
		{"archive.tgz", CompressionGzip},
		{"archive.tar.zst", CompressionZstd},
		{"archive.tar", CompressionNone},
		{"archive.TAR.GZ", CompressionGzip}, // Case insensitive
		{"archive.TGZ", CompressionGzip},
		{"archive.TAR.ZST", CompressionZstd},
		{"archive.TAR", CompressionNone},
		{"unknown.zip", CompressionGzip}, // Default to gzip
		{"no-extension", CompressionGzip},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := detectCompressionFormat(tt.key)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractor_ExtractGzip(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "content of file 1",
		"file2.txt": "content of file 2",
		"file3.txt": "content of file 3",
	}

	mockClient := &mockS3Client{
		body: createTestTarGz(t, files),
	}

	config := &ExtractorConfig{
		S3Client:          mockClient,
		Bucket:            "test-bucket",
		Key:               "test.tar.gz",
		OutputDir:         tmpDir,
		CompressionFormat: CompressionAuto,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	// Verify statistics
	if stats.FilesExtracted != len(files) {
		t.Errorf("Expected %d files extracted, got %d", len(files), stats.FilesExtracted)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", stats.ErrorCount)
	}
	if stats.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	// Verify files exist and have correct content
	for name, expectedContent := range files {
		path := filepath.Join(tmpDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", name, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("File %s: expected %q, got %q", name, expectedContent, string(content))
		}
	}
}

func TestExtractor_ExtractZstd(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "zstd content 1",
		"file2.txt": "zstd content 2",
	}

	mockClient := &mockS3Client{
		body: createTestTarZst(t, files),
	}

	config := &ExtractorConfig{
		S3Client:          mockClient,
		Bucket:            "test-bucket",
		Key:               "test.tar.zst",
		OutputDir:         tmpDir,
		CompressionFormat: CompressionZstd,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if stats.FilesExtracted != len(files) {
		t.Errorf("Expected %d files extracted, got %d", len(files), stats.FilesExtracted)
	}

	// Verify files exist
	for name, expectedContent := range files {
		path := filepath.Join(tmpDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", name, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("File %s: expected %q, got %q", name, expectedContent, string(content))
		}
	}
}

func TestExtractor_ExtractUncompressed(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "uncompressed content",
	}

	mockClient := &mockS3Client{
		body: createTestTar(t, files),
	}

	config := &ExtractorConfig{
		S3Client:          mockClient,
		Bucket:            "test-bucket",
		Key:               "test.tar",
		OutputDir:         tmpDir,
		CompressionFormat: CompressionNone,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if stats.FilesExtracted != len(files) {
		t.Errorf("Expected %d files extracted, got %d", len(files), stats.FilesExtracted)
	}
}

func TestExtractor_ExtractWithDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &mockS3Client{
		body: createTestTarWithDirectory(t),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "test.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if stats.FilesExtracted != 1 {
		t.Errorf("Expected 1 file extracted, got %d", stats.FilesExtracted)
	}

	// Verify directory exists
	dirPath := filepath.Join(tmpDir, "testdir")
	if info, err := os.Stat(dirPath); err != nil {
		t.Errorf("Directory not created: %v", err)
	} else if !info.IsDir() {
		t.Error("Expected directory, got file")
	}

	// Verify file in directory exists
	filePath := filepath.Join(tmpDir, "testdir", "file.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "file in directory" {
		t.Errorf("Expected %q, got %q", "file in directory", string(content))
	}
}

func TestExtractor_ExtractWithSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &mockS3Client{
		body: createTestTarWithSymlink(t),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "test.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if stats.FilesExtracted != 1 {
		t.Errorf("Expected 1 file extracted (symlink not counted), got %d", stats.FilesExtracted)
	}

	// Verify symlink exists
	linkPath := filepath.Join(tmpDir, "link.txt")
	if info, err := os.Lstat(linkPath); err != nil {
		t.Errorf("Symlink not created: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symlink, got regular file")
	}

	// Verify symlink target
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("Expected symlink to target.txt, got %s", target)
	}
}

func TestExtractor_PathTraversalPrevention(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &mockS3Client{
		body: createTestTarWithPathTraversal(t),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "malicious.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	_, err = extractor.Extract(ctx)

	// Should fail with path traversal error
	if err == nil {
		t.Fatal("Expected path traversal error, got nil")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("Expected path traversal error, got: %v", err)
	}
}

func TestExtractor_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingFile := filepath.Join(tmpDir, "file1.txt")
	existingContent := "existing content"
	if err := os.WriteFile(existingFile, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	t.Run("skip existing (default)", func(t *testing.T) {
		files := map[string]string{
			"file1.txt": "new content",
		}

		mockClient := &mockS3Client{
			body: createTestTarGz(t, files),
		}

		config := &ExtractorConfig{
			S3Client:          mockClient,
			Bucket:            "test-bucket",
			Key:               "test.tar.gz",
			OutputDir:         tmpDir,
			OverwriteExisting: false,
		}

		extractor, err := NewExtractor(config)
		if err != nil {
			t.Fatalf("Failed to create extractor: %v", err)
		}

		ctx := context.Background()
		stats, err := extractor.Extract(ctx)
		if err != nil {
			t.Fatalf("Failed to extract: %v", err)
		}

		if stats.SkippedFiles != 1 {
			t.Errorf("Expected 1 skipped file, got %d", stats.SkippedFiles)
		}

		// Verify file content unchanged
		content, err := os.ReadFile(existingFile)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		if string(content) != existingContent {
			t.Errorf("File was modified: expected %q, got %q", existingContent, string(content))
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		files := map[string]string{
			"file1.txt": "new content",
		}

		mockClient := &mockS3Client{
			body: createTestTarGz(t, files),
		}

		config := &ExtractorConfig{
			S3Client:          mockClient,
			Bucket:            "test-bucket",
			Key:               "test.tar.gz",
			OutputDir:         tmpDir,
			OverwriteExisting: true,
		}

		extractor, err := NewExtractor(config)
		if err != nil {
			t.Fatalf("Failed to create extractor: %v", err)
		}

		ctx := context.Background()
		stats, err := extractor.Extract(ctx)
		if err != nil {
			t.Fatalf("Failed to extract: %v", err)
		}

		if stats.SkippedFiles != 0 {
			t.Errorf("Expected 0 skipped files, got %d", stats.SkippedFiles)
		}

		// Verify file content changed
		content, err := os.ReadFile(existingFile)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		if string(content) != "new content" {
			t.Errorf("File not updated: expected %q, got %q", "new content", string(content))
		}
	})
}

func TestExtractor_ProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"file3.txt": "content 3",
	}

	mockClient := &mockS3Client{
		body: createTestTarGz(t, files),
	}

	var progressUpdates []ExtractProgress
	progressCallback := func(progress ExtractProgress) {
		progressUpdates = append(progressUpdates, progress)
	}

	config := &ExtractorConfig{
		S3Client:         mockClient,
		Bucket:           "test-bucket",
		Key:              "test.tar.gz",
		OutputDir:        tmpDir,
		ProgressCallback: progressCallback,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	_, err = extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	// Verify progress updates
	if len(progressUpdates) != len(files) {
		t.Errorf("Expected %d progress updates, got %d", len(files), len(progressUpdates))
	}

	// Verify progress is monotonically increasing
	for i := 0; i < len(progressUpdates); i++ {
		if progressUpdates[i].FilesExtracted != i+1 {
			t.Errorf("Progress update %d: expected %d files extracted, got %d",
				i, i+1, progressUpdates[i].FilesExtracted)
		}
		if progressUpdates[i].CurrentFile == "" {
			t.Errorf("Progress update %d: expected current file name, got empty", i)
		}
		if progressUpdates[i].ElapsedTime == 0 {
			t.Errorf("Progress update %d: expected non-zero elapsed time", i)
		}
	}
}

func TestExtractor_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create large archive to allow time for cancellation
	files := make(map[string]string)
	for i := 0; i < 100; i++ {
		files[filepath.Join("dir", fmt.Sprintf("subdir%d", i), "file.txt")] = strings.Repeat("x", 1000)
	}

	mockClient := &mockS3Client{
		body: createTestTarGz(t, files),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "test.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = extractor.Extract(ctx)

	// Should fail with context cancellation error
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

func TestExtractor_BytesCounting(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": strings.Repeat("a", 1000),
		"file2.txt": strings.Repeat("b", 2000),
		"file3.txt": strings.Repeat("c", 3000),
	}

	expectedBytes := int64(1000 + 2000 + 3000)

	mockClient := &mockS3Client{
		body: createTestTarGz(t, files),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "test.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	ctx := context.Background()
	stats, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if stats.BytesExtracted != expectedBytes {
		t.Errorf("Expected %d bytes extracted, got %d", expectedBytes, stats.BytesExtracted)
	}
}

func TestExtractor_GetStats(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
	}

	mockClient := &mockS3Client{
		body: createTestTarGz(t, files),
	}

	config := &ExtractorConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		Key:       "test.tar.gz",
		OutputDir: tmpDir,
	}

	extractor, err := NewExtractor(config)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	// Get stats before extraction
	statsBefore := extractor.GetStats()
	if statsBefore.FilesExtracted != 0 {
		t.Error("Expected 0 files extracted before extraction")
	}

	// Extract
	ctx := context.Background()
	statsAfter, err := extractor.Extract(ctx)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	// Verify stats match
	statsFromGetter := extractor.GetStats()
	if statsFromGetter.FilesExtracted != statsAfter.FilesExtracted {
		t.Errorf("GetStats mismatch: expected %d files, got %d",
			statsAfter.FilesExtracted, statsFromGetter.FilesExtracted)
	}
	if statsFromGetter.BytesExtracted != statsAfter.BytesExtracted {
		t.Errorf("GetStats mismatch: expected %d bytes, got %d",
			statsAfter.BytesExtracted, statsFromGetter.BytesExtracted)
	}
}
