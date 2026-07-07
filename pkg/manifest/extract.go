// Package manifest provides selective extraction from CargoHold archives (Issue #93)
package manifest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

// ExtractRequest specifies files to extract from a CargoHold archive
type ExtractRequest struct {
	// Target files to extract (supports glob patterns)
	FilePaths []string

	// Output directory for extracted files
	OutputDir string

	// Preserve directory structure (true) or flatten (false)
	PreserveStructure bool

	// Overwrite existing files
	Overwrite bool

	// Concurrency for parallel shard downloads
	Concurrency int

	// Progress callback (optional)
	OnProgress func(extracted, total int64)

	// Per-file progress callback (optional)
	OnFileExtracted func(path string, size int64)
}

// ExtractResult contains statistics from an extraction operation
type ExtractResult struct {
	FilesExtracted   int
	BytesExtracted   int64
	ShardsDownloaded int
	Duration         int64 // milliseconds
	Errors           []error
}

// S3Downloader is an interface for downloading objects from S3
type S3Downloader interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Extractor provides selective extraction from CargoHold archives
type Extractor struct {
	manifest *Manifest
	s3Client S3Downloader
	query    *ManifestQuery
}

// NewExtractor creates a new extractor for a manifest
func NewExtractor(manifest *Manifest, s3Client S3Downloader) *Extractor {
	return &Extractor{
		manifest: manifest,
		s3Client: s3Client,
		query:    NewManifestQuery(manifest),
	}
}

// Extract extracts requested files from the CargoHold archive
// This is the main entry point for selective extraction
func (e *Extractor) Extract(ctx context.Context, req *ExtractRequest) (*ExtractResult, error) {
	if err := e.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid extract request: %w", err)
	}

	// Resolve glob patterns to actual file entries
	filesToExtract, err := e.resolveFiles(req.FilePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file patterns: %w", err)
	}

	if len(filesToExtract) == 0 {
		return &ExtractResult{}, fmt.Errorf("no files matched the specified patterns")
	}

	// Group files by shard for efficient download
	shardMap := e.groupFilesByShard(filesToExtract)

	// Create output directory
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Extract files in parallel by shard
	result, err := e.extractParallel(ctx, req, shardMap)
	if err != nil {
		return result, fmt.Errorf("extraction failed: %w", err)
	}

	return result, nil
}

// validateRequest validates the extract request
func (e *Extractor) validateRequest(req *ExtractRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if len(req.FilePaths) == 0 {
		return fmt.Errorf("no file paths specified")
	}

	if req.OutputDir == "" {
		return fmt.Errorf("output directory not specified")
	}

	if req.Concurrency <= 0 {
		req.Concurrency = 4 // Default concurrency
	}

	return nil
}

// resolveFiles resolves glob patterns to actual file entries
func (e *Extractor) resolveFiles(patterns []string) ([]FileEntry, error) {
	seenPaths := make(map[string]bool)
	var files []FileEntry

	for _, pattern := range patterns {
		// Check for exact match first
		if file := e.query.FindFile(pattern); file != nil {
			if !seenPaths[file.Path] {
				files = append(files, *file)
				seenPaths[file.Path] = true
			}
			continue
		}

		// Try glob pattern matching
		matches := e.query.ListFiles(pattern)
		for _, match := range matches {
			if !seenPaths[match.Path] {
				files = append(files, match)
				seenPaths[match.Path] = true
			}
		}
	}

	return files, nil
}

// groupFilesByShard groups files by their shard ID for efficient extraction
func (e *Extractor) groupFilesByShard(files []FileEntry) map[int][]FileEntry {
	shardMap := make(map[int][]FileEntry)

	for _, file := range files {
		shardMap[file.ShardID] = append(shardMap[file.ShardID], file)
	}

	return shardMap
}

// extractParallel extracts files in parallel by processing shards concurrently
func (e *Extractor) extractParallel(ctx context.Context, req *ExtractRequest, shardMap map[int][]FileEntry) (*ExtractResult, error) {
	result := &ExtractResult{}

	// Create work queue
	type shardWork struct {
		shardID int
		files   []FileEntry
	}

	workChan := make(chan shardWork, len(shardMap))
	resultChan := make(chan *ExtractResult, len(shardMap))
	errorChan := make(chan error, len(shardMap))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < req.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range workChan {
				shardResult, err := e.extractShard(ctx, req, work.shardID, work.files)
				if err != nil {
					errorChan <- fmt.Errorf("shard %d: %w", work.shardID, err)
					continue
				}
				resultChan <- shardResult
			}
		}()
	}

	// Queue work
	for shardID, files := range shardMap {
		workChan <- shardWork{shardID: shardID, files: files}
	}
	close(workChan)

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()

	// Collect results
	for shardResult := range resultChan {
		result.FilesExtracted += shardResult.FilesExtracted
		result.BytesExtracted += shardResult.BytesExtracted
		result.ShardsDownloaded++
	}

	// Collect errors
	for err := range errorChan {
		result.Errors = append(result.Errors, err)
	}

	if len(result.Errors) > 0 {
		return result, fmt.Errorf("extraction completed with %d errors", len(result.Errors))
	}

	return result, nil
}

// extractShard extracts files from a single shard
func (e *Extractor) extractShard(ctx context.Context, req *ExtractRequest, shardID int, files []FileEntry) (*ExtractResult, error) {
	result := &ExtractResult{}

	// Get unique chunks for these files
	chunkMap := make(map[int][]FileEntry)
	for _, file := range files {
		chunkMap[file.ChunkID] = append(chunkMap[file.ChunkID], file)
	}

	// Extract from each chunk
	for chunkID, chunkFiles := range chunkMap {
		chunkResult, err := e.extractChunk(ctx, req, chunkID, chunkFiles)
		if err != nil {
			return result, fmt.Errorf("chunk %d: %w", chunkID, err)
		}

		result.FilesExtracted += chunkResult.FilesExtracted
		result.BytesExtracted += chunkResult.BytesExtracted
	}

	return result, nil
}

// extractChunk extracts specific files from a single chunk
func (e *Extractor) extractChunk(ctx context.Context, req *ExtractRequest, chunkID int, files []FileEntry) (*ExtractResult, error) {
	result := &ExtractResult{}

	// Find chunk metadata
	var chunk *ChunkEntry
	for i := range e.manifest.Chunks {
		if e.manifest.Chunks[i].ID == chunkID {
			chunk = &e.manifest.Chunks[i]
			break
		}
	}

	if chunk == nil {
		return nil, fmt.Errorf("chunk %d not found in manifest", chunkID)
	}

	// Download chunk from S3
	chunkData, err := e.downloadChunk(ctx, chunk.S3Key)
	if err != nil {
		return nil, fmt.Errorf("failed to download chunk: %w", err)
	}
	defer func() {
		if closeErr := chunkData.Close(); closeErr != nil {
			// Log but don't fail - extraction may have completed
			_ = closeErr
		}
	}()

	// Decompress and extract
	if err := e.extractFilesFromChunk(chunkData, req, files, result); err != nil {
		return nil, fmt.Errorf("failed to extract from chunk: %w", err)
	}

	return result, nil
}

// downloadChunk downloads a chunk from S3
func (e *Extractor) downloadChunk(ctx context.Context, s3Key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(e.manifest.Bucket),
		Key:    aws.String(s3Key),
	}

	output, err := e.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("S3 GetObject failed: %w", err)
	}

	return output.Body, nil
}

// extractFilesFromChunk extracts specific files from a chunk's tar archive
func (e *Extractor) extractFilesFromChunk(chunkData io.ReadCloser, req *ExtractRequest, files []FileEntry, result *ExtractResult) error {
	// Create file path map for quick lookup
	filesToExtract := make(map[string]FileEntry)
	for _, file := range files {
		filesToExtract[file.Path] = file
	}

	// Decompress based on compression type
	var decompressed io.Reader

	switch e.manifest.CompressionType {
	case "zstd":
		decoder, err := zstd.NewReader(chunkData)
		if err != nil {
			return fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		defer decoder.Close()
		decompressed = decoder

	case "gzip", "gz":
		gzReader, err := gzip.NewReader(chunkData)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			if closeErr := gzReader.Close(); closeErr != nil {
				_ = closeErr
			}
		}()
		decompressed = gzReader

	case "none", "":
		decompressed = chunkData

	default:
		return fmt.Errorf("unsupported compression type: %s", e.manifest.CompressionType)
	}

	// Read tar archive
	tarReader := tar.NewReader(decompressed)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		// Check if this file should be extracted
		fileEntry, shouldExtract := filesToExtract[header.Name]
		if !shouldExtract {
			// Skip this file
			continue
		}

		// Determine output path
		outputPath := e.getOutputPath(req, header.Name)

		// Check if file exists and handle overwrite
		if !req.Overwrite {
			if _, err := os.Stat(outputPath); err == nil {
				// File exists, skip
				continue
			}
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", outputPath, err)
		}

		// Extract file
		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", outputPath, err)
		}

		bytesWritten, err := io.Copy(outFile, tarReader) // nosemgrep: go.lang.security.decompression_bomb.potential-dos-via-decompression-bomb -- extracting cargoship's own archive tar stream
		if closeErr := outFile.Close(); closeErr != nil {
			return fmt.Errorf("failed to close file %s: %w", outputPath, closeErr)
		}

		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", outputPath, err)
		}

		// Update result
		result.FilesExtracted++
		result.BytesExtracted += bytesWritten

		// Call progress callback
		if req.OnFileExtracted != nil {
			req.OnFileExtracted(fileEntry.Path, bytesWritten)
		}

		if req.OnProgress != nil {
			req.OnProgress(result.BytesExtracted, e.calculateTotalSize(files))
		}
	}

	return nil
}

// getOutputPath determines the output path for an extracted file
func (e *Extractor) getOutputPath(req *ExtractRequest, filePath string) string {
	if req.PreserveStructure {
		return filepath.Join(req.OutputDir, filePath)
	}

	// Flatten: use only the basename
	return filepath.Join(req.OutputDir, filepath.Base(filePath))
}

// calculateTotalSize calculates total size of files to extract
func (e *Extractor) calculateTotalSize(files []FileEntry) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

// EstimateExtractCost estimates the extraction cost for given file patterns
// Returns number of shards and chunks that need to be downloaded
func (e *Extractor) EstimateExtractCost(patterns []string) (shards, chunks int, totalBytes int64, err error) {
	files, err := e.resolveFiles(patterns)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(files) == 0 {
		return 0, 0, 0, nil
	}

	shardSet := make(map[int]bool)
	chunkSet := make(map[int]bool)

	for _, file := range files {
		shardSet[file.ShardID] = true
		chunkSet[file.ChunkID] = true
		totalBytes += file.Size
	}

	return len(shardSet), len(chunkSet), totalBytes, nil
}

// ListExtractableFiles lists all files that match the given patterns
func (e *Extractor) ListExtractableFiles(patterns []string) ([]FileEntry, error) {
	return e.resolveFiles(patterns)
}
