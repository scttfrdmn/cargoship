// Package extraction provides streaming decompression and extraction from S3 archives
package extraction

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
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

	// #341: every filesystem write below goes through this root, so a symlinked
	// component inside OutputDir — pre-existing, or planted by an earlier entry in
	// this same archive — is refused rather than followed.
	if err := os.MkdirAll(e.config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}
	root, err := os.OpenRoot(e.config.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open output directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	// Extract files from tar archive
	if err := e.extractFiles(ctx, tarReader, root); err != nil {
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

// extractFiles extracts all files from the tar archive. All writes are performed
// relative to root, which confines them to OutputDir. (#341)
func (e *Extractor) extractFiles(ctx context.Context, tarReader *tar.Reader, root *os.Root) error {
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
		if err := e.extractFile(tarReader, header, root); err != nil {
			e.stats.ErrorCount++
			return fmt.Errorf("failed to extract %s: %w", header.Name, err)
		}

		// Report progress
		e.reportProgress(header.Name)
	}

	return nil
}

// extractFile extracts a single file from the tar archive.
//
// Containment is enforced in two layers. The lexical check below rejects an
// obviously hostile archive entry ("../evil") with a clear diagnostic naming the
// entry. The root passed to the creators is what actually guarantees containment:
// it re-validates every path component at open time, so a symlink in OutputDir
// cannot redirect a write out of it. The lexical check alone cannot see symlinks,
// which is the defect in #341.
func (e *Extractor) extractFile(tarReader *tar.Reader, header *tar.Header, root *os.Root) error {
	// Construct output path
	outputPath := filepath.Join(e.config.OutputDir, header.Name) // #nosec G305 -- containment is verified immediately below

	// Security: prevent path traversal attacks
	if !strings.HasPrefix(outputPath, filepath.Clean(e.config.OutputDir)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid path: %s (possible path traversal)", header.Name)
	}

	relPath, err := e.relOutputPath(outputPath)
	if err != nil {
		return err
	}

	// Handle different file types
	switch header.Typeflag {
	case tar.TypeDir:
		return e.createDirectory(root, relPath, header)
	case tar.TypeReg:
		return e.createRegularFile(tarReader, root, relPath, header)
	case tar.TypeSymlink:
		return e.createSymlink(root, relPath, outputPath, header)
	default:
		// Skip unsupported file types (devices, FIFOs, etc.)
		return nil
	}
}

// relOutputPath converts an absolute output path into a slash-separated path
// relative to OutputDir, for use with the *os.Root methods.
func (e *Extractor) relOutputPath(outputPath string) (string, error) {
	rel, err := filepath.Rel(e.config.OutputDir, outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to relativize %q against output directory: %w", outputPath, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path: %s (escapes output directory)", outputPath)
	}
	return filepath.ToSlash(rel), nil
}

// mkdirParentsContained creates the parent directories of relPath inside root.
func mkdirParentsContained(root *os.Root, relPath string) error {
	return mkdirContained(root, path.Dir(relPath), 0755)
}

// mkdirContained creates dir and each missing parent inside root, refusing to
// traverse a symlinked component.
//
// root.MkdirAll is not sufficient on its own: os.Root blocks a symlink that
// leaves the root, but a symlink pointing to another path *inside* it is still
// followed. An archive can exploit that in two entries — plant "link" -> "real"
// (a legal relative symlink), then write "link/payload.txt" — and both entries
// pass every lexical check individually. Walking component by component and
// rejecting any symlink closes that, so an extracted path means what it says.
func mkdirContained(root *os.Root, dir string, perm os.FileMode) error {
	if dir == "." || dir == "/" || dir == "" {
		return nil
	}
	var cur string
	for _, comp := range strings.Split(dir, "/") {
		if comp == "" || comp == "." {
			continue
		}
		if cur == "" {
			cur = comp
		} else {
			cur += "/" + comp
		}

		fi, err := root.Lstat(cur)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to write under %s: path component is a symlink", cur)
			}
			if !fi.IsDir() {
				return fmt.Errorf("refusing to write under %s: path component is not a directory", cur)
			}
			continue
		}
		if err := root.Mkdir(cur, perm); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create directory %s: %w", cur, err)
		}
	}
	return nil
}

// createDirectory creates a directory from tar header
func (e *Extractor) createDirectory(root *os.Root, relPath string, header *tar.Header) error {
	// Create directory with permissions
	mode := os.FileMode(0755)
	if e.config.PreservePermissions {
		mode = header.FileInfo().Mode()
	}

	// mode comes from a tar header and carries type bits that Mkdir rejects; keep
	// only the permission bits.
	if err := mkdirContained(root, relPath, mode.Perm()); err != nil {
		return err
	}

	return nil
}

// createRegularFile creates a regular file from tar archive
func (e *Extractor) createRegularFile(tarReader *tar.Reader, root *os.Root, relPath string, header *tar.Header) error {
	// Check if file exists. Lstat, not Stat: a dangling symlink at this path is
	// still an existing entry, and Stat would follow it. (#341)
	if !e.config.OverwriteExisting {
		if _, err := root.Lstat(relPath); err == nil {
			e.stats.SkippedFiles++
			return nil // Skip existing file
		}
	}

	// Create parent directories
	if err := mkdirParentsContained(root, relPath); err != nil {
		return err
	}

	// Refuse to write through a symlink at the leaf. root already blocks a link
	// pointing outside OutputDir; this also rejects one pointing back inside it,
	// because an extraction should write the path the archive named rather than
	// wherever a link happens to lead. (#341)
	if fi, err := root.Lstat(relPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to extract %s: destination path is a symlink", relPath)
	}

	// Create file
	outFile, err := root.Create(relPath)
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

// createSymlink creates a symbolic link from tar header.
// The symlink target is validated to prevent path traversal via crafted archives (CWE-22).
// outputPath is the absolute form of relPath, used only for target resolution.
func (e *Extractor) createSymlink(root *os.Root, relPath, outputPath string, header *tar.Header) error {
	// Validate symlink target: reject absolute paths and any ".." components.
	if filepath.IsAbs(header.Linkname) || strings.Contains(header.Linkname, "..") {
		return fmt.Errorf("invalid symlink target %q: must be relative and must not contain '..'", header.Linkname)
	}

	// Resolve what the symlink would point to and ensure it stays inside OutputDir.
	resolvedTarget := filepath.Join(filepath.Dir(outputPath), header.Linkname) // #nosec G305 -- containment is verified immediately below
	outputDir := filepath.Clean(e.config.OutputDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(resolvedTarget)+string(os.PathSeparator), outputDir) {
		return fmt.Errorf("symlink target %q escapes extraction directory", header.Linkname)
	}

	// Create parent directories
	if err := mkdirParentsContained(root, relPath); err != nil {
		return err
	}

	// Remove existing symlink if overwrite is enabled
	if e.config.OverwriteExisting {
		_ = root.Remove(relPath)
	}

	// Create symlink
	if err := root.Symlink(header.Linkname, relPath); err != nil {
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
