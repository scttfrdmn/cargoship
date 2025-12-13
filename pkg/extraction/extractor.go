// Package extraction provides streaming decompression and extraction from S3 archives
package extraction

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

// S3GetObjectAPI defines the interface for S3 GetObject operation
// This interface allows for easier testing by enabling mock implementations
type S3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// CompressionFormat represents the compression format of the archive
type CompressionFormat int

const (
	// CompressionAuto auto-detects compression format from file extension
	CompressionAuto CompressionFormat = iota
	// CompressionGzip for .tar.gz, .tgz files
	CompressionGzip
	// CompressionZstd for .tar.zst files
	CompressionZstd
	// CompressionNone for uncompressed .tar files
	CompressionNone
)

// ExtractorConfig configures the archive extractor
type ExtractorConfig struct {
	// S3 configuration
	S3Client S3GetObjectAPI // AWS S3 client (can be *s3.Client or mock)
	Bucket   string         // S3 bucket name
	Key      string         // S3 object key

	// Extraction configuration
	OutputDir           string            // Output directory for extracted files
	CompressionFormat   CompressionFormat // Compression format (auto-detect if not specified)
	OverwriteExisting   bool              // Overwrite existing files (default: false)
	PreservePermissions bool              // Preserve file permissions from archive (default: true)

	// Progress reporting
	ProgressCallback func(ExtractProgress) // Optional callback for progress updates
}

// ExtractProgress represents extraction progress information
type ExtractProgress struct {
	CurrentFile    string        // Currently extracting file
	FilesExtracted int           // Total files extracted so far
	BytesExtracted int64         // Total bytes extracted so far
	TotalFiles     int           // Total files in archive (if known)
	ElapsedTime    time.Duration // Elapsed time since extraction started
}

// Extractor handles streaming extraction from S3 archives
type Extractor struct {
	config    *ExtractorConfig
	startTime time.Time
	stats     ExtractionStats
}

// ExtractionStats contains statistics about the extraction operation
type ExtractionStats struct {
	FilesExtracted int           // Total files extracted
	BytesExtracted int64         // Total bytes extracted
	Duration       time.Duration // Total extraction duration
	SkippedFiles   int           // Files skipped (already exist)
	ErrorCount     int           // Number of errors encountered
}

// NewExtractor creates a new archive extractor
func NewExtractor(config *ExtractorConfig) (*Extractor, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}
	if config.Key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}
	if config.OutputDir == "" {
		return nil, fmt.Errorf("output directory cannot be empty")
	}

	// Auto-detect compression format from file extension if not specified
	if config.CompressionFormat == CompressionAuto {
		config.CompressionFormat = detectCompressionFormat(config.Key)
	}

	// Default: preserve permissions
	if !config.OverwriteExisting {
		config.PreservePermissions = true
	}

	return &Extractor{
		config: config,
	}, nil
}

// Extract performs streaming extraction from S3 to local filesystem
func (e *Extractor) Extract(ctx context.Context) (*ExtractionStats, error) {
	e.startTime = time.Now()

	// Download from S3
	result, err := e.config.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &e.config.Bucket,
		Key:    &e.config.Key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 object: %w", err)
	}
	defer func() { _ = result.Body.Close() }()

	// Create decompressor based on format
	reader, err := e.createDecompressor(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}
	defer e.closeReader(reader)

	// Create tar reader
	tarReader := tar.NewReader(reader)

	// Extract files from tar archive
	if err := e.extractFiles(ctx, tarReader); err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	e.stats.Duration = time.Since(e.startTime)
	return &e.stats, nil
}

// createDecompressor creates the appropriate decompressor based on format
func (e *Extractor) createDecompressor(r io.Reader) (io.ReadCloser, error) {
	switch e.config.CompressionFormat {
	case CompressionGzip:
		return gzip.NewReader(r)
	case CompressionZstd:
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		return io.NopCloser(decoder), nil
	case CompressionNone:
		return io.NopCloser(r), nil
	default:
		return nil, fmt.Errorf("unsupported compression format: %v", e.config.CompressionFormat)
	}
}

// closeReader safely closes the reader if it implements io.Closer
func (e *Extractor) closeReader(r io.Reader) {
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
}

// extractFiles extracts all files from the tar archive
func (e *Extractor) extractFiles(ctx context.Context, tarReader *tar.Reader) error {
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read next tar entry
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			e.stats.ErrorCount++
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Extract the file
		if err := e.extractFile(tarReader, header); err != nil {
			e.stats.ErrorCount++
			return fmt.Errorf("failed to extract %s: %w", header.Name, err)
		}

		// Report progress
		e.reportProgress(header.Name)
	}

	return nil
}

// extractFile extracts a single file from the tar archive
func (e *Extractor) extractFile(tarReader *tar.Reader, header *tar.Header) error {
	// Construct output path
	outputPath := filepath.Join(e.config.OutputDir, header.Name)

	// Security: prevent path traversal attacks
	if !strings.HasPrefix(outputPath, filepath.Clean(e.config.OutputDir)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid path: %s (possible path traversal)", header.Name)
	}

	// Handle different file types
	switch header.Typeflag {
	case tar.TypeDir:
		return e.createDirectory(outputPath, header)
	case tar.TypeReg:
		return e.createRegularFile(tarReader, outputPath, header)
	case tar.TypeSymlink:
		return e.createSymlink(outputPath, header)
	default:
		// Skip unsupported file types (devices, FIFOs, etc.)
		return nil
	}
}

// createDirectory creates a directory from tar header
func (e *Extractor) createDirectory(path string, header *tar.Header) error {
	// Create directory with permissions
	mode := os.FileMode(0755)
	if e.config.PreservePermissions {
		mode = header.FileInfo().Mode()
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// createRegularFile creates a regular file from tar archive
func (e *Extractor) createRegularFile(tarReader *tar.Reader, path string, header *tar.Header) error {
	// Check if file exists
	if !e.config.OverwriteExisting {
		if _, err := os.Stat(path); err == nil {
			e.stats.SkippedFiles++
			return nil // Skip existing file
		}
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create file
	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Copy file content (bounded memory)
	n, err := io.Copy(outFile, tarReader)
	if err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Set permissions
	if e.config.PreservePermissions {
		if err := outFile.Chmod(header.FileInfo().Mode()); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}
	}

	// Update statistics
	e.stats.FilesExtracted++
	e.stats.BytesExtracted += n

	return nil
}

// createSymlink creates a symbolic link from tar header
func (e *Extractor) createSymlink(path string, header *tar.Header) error {
	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Remove existing symlink if overwrite is enabled
	if e.config.OverwriteExisting {
		_ = os.Remove(path)
	}

	// Create symlink
	if err := os.Symlink(header.Linkname, path); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// reportProgress reports extraction progress via callback
func (e *Extractor) reportProgress(currentFile string) {
	if e.config.ProgressCallback == nil {
		return
	}

	progress := ExtractProgress{
		CurrentFile:    currentFile,
		FilesExtracted: e.stats.FilesExtracted,
		BytesExtracted: e.stats.BytesExtracted,
		ElapsedTime:    time.Since(e.startTime),
	}

	e.config.ProgressCallback(progress)
}

// detectCompressionFormat detects compression format from file extension
func detectCompressionFormat(key string) CompressionFormat {
	lower := strings.ToLower(key)

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return CompressionGzip
	}
	if strings.HasSuffix(lower, ".tar.zst") {
		return CompressionZstd
	}
	if strings.HasSuffix(lower, ".tar") {
		return CompressionNone
	}

	// Default to gzip for unknown extensions
	return CompressionGzip
}

// GetStats returns extraction statistics
func (e *Extractor) GetStats() ExtractionStats {
	return e.stats
}
