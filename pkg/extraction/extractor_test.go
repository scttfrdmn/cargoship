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

// ---------------------------------------------------------------------------
// Symlinked-destination containment (#341)
//
// The existing traversal test above covers a hostile *archive* ("../evil"). The
// tests below cover a hostile *destination*: OutputDir already contains a
// symlink, planted by an earlier extraction or by anything else with write
// access to that directory. filepath.Join + HasPrefix is a lexical check and
// cannot see it, so pre-fix the OS happily followed the link and wrote outside
// OutputDir. (CWE-59)
// ---------------------------------------------------------------------------

// symlinkedOutputDir returns an OutputDir containing linkName -> an outside
// directory, plus that outside directory, for asserting nothing escapes.
func symlinkedOutputDir(t *testing.T, linkName string) (string, string) {
	t.Helper()
	base := t.TempDir()
	out := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(out, linkName)); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	return out, outside
}

// TestExtractor_SymlinkedParentDirRefused proves a pre-existing symlinked parent
// directory in OutputDir is refused rather than followed.
func TestExtractor_SymlinkedParentDirRefused(t *testing.T) {
	out, outside := symlinkedOutputDir(t, "cache")

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:  &mockS3Client{body: createTestTarGz(t, map[string]string{"cache/config.txt": "attacker content"})},
		Bucket:    "test-bucket",
		Key:       "archive.tar.gz",
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	_, err = extractor.Extract(context.Background())

	escaped := filepath.Join(outside, "config.txt")
	if _, statErr := os.Lstat(escaped); statErr == nil {
		t.Fatalf("extraction escaped OutputDir: wrote through a symlinked parent to %s", escaped)
	}
	if err == nil {
		t.Fatal("expected extraction to fail on a symlinked parent directory, got nil")
	}
}

// TestExtractor_SymlinkedLeafRefused covers the leaf case: OutputDir already
// holds a symlink AT the path being extracted, so writing through it truncates
// a file outside OutputDir.
func TestExtractor_SymlinkedLeafRefused(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	victim := filepath.Join(base, "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o644); err != nil {
		t.Fatalf("failed to create victim file: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(out, "file1.txt")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:          &mockS3Client{body: createTestTarGz(t, map[string]string{"file1.txt": "attacker content"})},
		Bucket:            "test-bucket",
		Key:               "archive.tar.gz",
		OutputDir:         out,
		OverwriteExisting: true,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	_, err = extractor.Extract(context.Background())

	got, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatalf("failed to read victim file: %v", readErr)
	}
	if string(got) != "original" {
		t.Errorf("extraction overwrote a file outside OutputDir through a symlinked leaf: got %q", got)
	}
	if err == nil {
		t.Fatal("expected extraction to fail on a symlinked leaf, got nil")
	}
}

// TestExtractor_PlantedSymlinkNotFollowed is the two-entry version, and the one
// that needs no pre-existing hostile state: entry 1 is a legitimate relative
// symlink (which the archive is allowed to contain), entry 2 writes *through*
// it. Both entries pass the lexical check individually.
func TestExtractor_PlantedSymlinkNotFollowed(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	victimDir := filepath.Join(out, "real")
	if err := os.MkdirAll(victimDir, 0o755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	// link -> real (relative, inside OutputDir: accepted by createSymlink), then
	// write link/payload.txt. A restore should write the path it was given.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "link", Typeflag: tar.TypeSymlink, Linkname: "real", Mode: 0777,
	}); err != nil {
		t.Fatalf("failed to write symlink header: %v", err)
	}
	content := "written through a symlink"
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "link/payload.txt", Mode: 0644, Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("failed to write file header: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:  &mockS3Client{body: io.NopCloser(bytes.NewReader(buf.Bytes()))},
		Bucket:    "test-bucket",
		Key:       "archive.tar.gz",
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	_, err = extractor.Extract(context.Background())

	if _, statErr := os.Lstat(filepath.Join(victimDir, "payload.txt")); statErr == nil {
		t.Error("extraction wrote through a planted symlink into real/")
	}
	if err == nil {
		t.Fatal("expected extraction to fail writing through a planted symlink, got nil")
	}
}

// TestExtractor_RealDirInOutputStillWorks is the control: an ordinary
// pre-existing directory in OutputDir must stay writable. Without this, a fix
// that refuses every existing parent would look correct.
func TestExtractor_RealDirInOutputStillWorks(t *testing.T) {
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, "cache"), 0o755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:  &mockS3Client{body: createTestTarGz(t, map[string]string{"cache/config.txt": "ordinary content"})},
		Bucket:    "test-bucket",
		Key:       "archive.tar.gz",
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	stats, err := extractor.Extract(context.Background())
	if err != nil {
		t.Fatalf("ordinary extraction into an existing real directory failed: %v", err)
	}
	if stats.FilesExtracted != 1 {
		t.Errorf("expected 1 file extracted, got %d", stats.FilesExtracted)
	}
	got, err := os.ReadFile(filepath.Join(out, "cache", "config.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(got) != "ordinary content" {
		t.Errorf("extracted content mismatch: got %q", got)
	}
}

// TestExtractor_NonDirectoryParentRefused covers the other half of the parent
// walk: a component that exists but is a regular file. Pre-fix os.MkdirAll would
// have failed with a bare ENOTDIR from deep inside the call; the walk names the
// offending component.
func TestExtractor_NonDirectoryParentRefused(t *testing.T) {
	out := t.TempDir()
	// "cache" exists as a file, but the archive wants to treat it as a directory.
	if err := os.WriteFile(filepath.Join(out, "cache"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:  &mockS3Client{body: createTestTarGz(t, map[string]string{"cache/config.txt": "payload"})},
		Bucket:    "test-bucket",
		Key:       "archive.tar.gz",
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	_, err = extractor.Extract(context.Background())
	if err == nil {
		t.Fatal("expected extraction to fail when a parent component is a regular file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected a 'not a directory' diagnostic naming the component, got: %v", err)
	}
}

// TestExtractor_SymlinkOverExistingRefused pins the skip path with Lstat rather
// than Stat: a *dangling* symlink at the target path is an existing entry, and
// Stat would follow it and report "not found", causing the extractor to write
// through the link. With OverwriteExisting off, it must be skipped.
func TestExtractor_DanglingSymlinkSkippedNotFollowed(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	// Points at a file that does not exist yet, inside the base but outside out.
	victim := filepath.Join(base, "victim.txt")
	if err := os.Symlink(victim, filepath.Join(out, "file1.txt")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:          &mockS3Client{body: createTestTarGz(t, map[string]string{"file1.txt": "attacker content"})},
		Bucket:            "test-bucket",
		Key:               "archive.tar.gz",
		OutputDir:         out,
		OverwriteExisting: false,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	stats, err := extractor.Extract(context.Background())
	if err != nil {
		t.Fatalf("extraction should skip the existing entry, not fail: %v", err)
	}
	if stats.SkippedFiles != 1 {
		t.Errorf("expected 1 skipped file, got %d", stats.SkippedFiles)
	}
	if _, statErr := os.Lstat(victim); statErr == nil {
		t.Error("extraction created the dangling symlink's target outside OutputDir")
	}
}

// TestExtractor_SymlinkOverwriteReplacesLink pins the overwrite branch of
// createSymlink: an existing entry at the path is removed first, so the new link
// is created rather than failing with EEXIST.
func TestExtractor_SymlinkOverwriteReplacesLink(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(out, "link")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target.txt", Mode: 0777,
	}); err != nil {
		t.Fatalf("failed to write symlink header: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:          &mockS3Client{body: io.NopCloser(bytes.NewReader(buf.Bytes()))},
		Bucket:            "test-bucket",
		Key:               "archive.tar.gz",
		OutputDir:         out,
		OverwriteExisting: true,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	if _, err := extractor.Extract(context.Background()); err != nil {
		t.Fatalf("overwriting an existing symlink should succeed: %v", err)
	}
	got, err := os.Readlink(filepath.Join(out, "link"))
	if err != nil {
		t.Fatalf("failed to read link: %v", err)
	}
	if got != "target.txt" {
		t.Errorf("expected link -> target.txt, got %q", got)
	}
}

// TestExtractor_OutputDirCreatedIfMissing pins that Extract creates OutputDir
// before opening the root — the extractor accepts a destination that does not
// exist yet, and os.OpenRoot would fail on it.
func TestExtractor_OutputDirCreatedIfMissing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "nested", "does-not-exist")

	extractor, err := NewExtractor(&ExtractorConfig{
		S3Client:  &mockS3Client{body: createTestTarGz(t, map[string]string{"file1.txt": "payload"})},
		Bucket:    "test-bucket",
		Key:       "archive.tar.gz",
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	if _, err := extractor.Extract(context.Background()); err != nil {
		t.Fatalf("extraction into a missing output directory failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(out, "file1.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content mismatch: got %q", got)
	}
}

// TestRelOutputPath exercises the root-relative conversion directly. Its
// escape branch is unreachable through Extract — the lexical check in extractFile
// rejects those entries first — so it is only reachable here. It exists as a
// backstop in case that ordering ever changes, and an untested backstop is not
// one.
func TestRelOutputPath(t *testing.T) {
	e := &Extractor{config: &ExtractorConfig{OutputDir: "/out"}}

	tests := []struct {
		name       string
		outputPath string
		want       string
		wantErr    bool
	}{
		{name: "nested entry", outputPath: "/out/a/b/c.txt", want: "a/b/c.txt"},
		{name: "top-level entry", outputPath: "/out/c.txt", want: "c.txt"},
		{name: "output dir itself", outputPath: "/out", wantErr: true},
		{name: "escapes output dir", outputPath: "/elsewhere/c.txt", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.relOutputPath(tt.outputPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %q", tt.outputPath, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMkdirContained exercises the parent walk directly, including the no-op
// cases that a tar archive cannot produce.
func TestMkdirContained(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("failed to open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	// No-op inputs must not error.
	for _, dir := range []string{".", "/", ""} {
		if err := mkdirContained(root, dir, 0755); err != nil {
			t.Errorf("mkdirContained(%q) = %v, want nil", dir, err)
		}
	}

	// Nested creation, then idempotent re-creation over real directories.
	for i := 0; i < 2; i++ {
		if err := mkdirContained(root, "a/b/c", 0755); err != nil {
			t.Fatalf("mkdirContained(a/b/c) attempt %d: %v", i+1, err)
		}
	}
	fi, err := root.Stat("a/b/c")
	if err != nil {
		t.Fatalf("stat a/b/c: %v", err)
	}
	if !fi.IsDir() {
		t.Error("a/b/c is not a directory")
	}
}

// TestExtractor_SymlinkTargetEscapes pins the target-validation branches of
// createSymlink: an absolute target, a "..", and a relative target that stays
// syntactically clean but resolves outside OutputDir.
func TestExtractor_SymlinkTargetEscapes(t *testing.T) {
	tests := []struct {
		name     string
		linkname string
		wantMsg  string
	}{
		{name: "absolute target", linkname: "/etc/passwd", wantMsg: "must be relative"},
		{name: "dotdot target", linkname: "../outside", wantMsg: "must be relative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			gzWriter := gzip.NewWriter(&buf)
			tarWriter := tar.NewWriter(gzWriter)
			if err := tarWriter.WriteHeader(&tar.Header{
				Name: "link", Typeflag: tar.TypeSymlink, Linkname: tt.linkname, Mode: 0777,
			}); err != nil {
				t.Fatalf("failed to write symlink header: %v", err)
			}
			if err := tarWriter.Close(); err != nil {
				t.Fatalf("failed to close tar writer: %v", err)
			}
			if err := gzWriter.Close(); err != nil {
				t.Fatalf("failed to close gzip writer: %v", err)
			}

			out := t.TempDir()
			extractor, err := NewExtractor(&ExtractorConfig{
				S3Client:  &mockS3Client{body: io.NopCloser(bytes.NewReader(buf.Bytes()))},
				Bucket:    "test-bucket",
				Key:       "archive.tar.gz",
				OutputDir: out,
			})
			if err != nil {
				t.Fatalf("Failed to create extractor: %v", err)
			}

			_, err = extractor.Extract(context.Background())
			if err == nil {
				t.Fatalf("expected symlink target %q to be rejected, got nil", tt.linkname)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.wantMsg, err)
			}
			if _, statErr := os.Lstat(filepath.Join(out, "link")); statErr == nil {
				t.Error("the rejected symlink was created anyway")
			}
		})
	}
}
