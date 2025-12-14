package migration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/extraction"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// Migrator handles converting traditional archives to CargoHold format
type Migrator struct {
	s3Client *s3.Client
	config   *MigratorConfig
}

// MigratorConfig contains configuration for the migrator
type MigratorConfig struct {
	// S3Client is the AWS S3 client for operations
	S3Client *s3.Client

	// ProgressCallback is called with progress updates (optional)
	ProgressCallback func(ProgressUpdate)
}

// NewMigrator creates a new migrator instance
func NewMigrator(config *MigratorConfig) *Migrator {
	return &Migrator{
		s3Client: config.S3Client,
		config:   config,
	}
}

// Migrate performs a complete migration from traditional archive to CargoHold format
func (m *Migrator) Migrate(ctx context.Context, req *MigrateRequest) (*MigrateResult, error) {
	startTime := time.Now()

	result := &MigrateResult{
		Success: false,
	}

	// Phase 1: Validation
	m.sendProgress(ProgressUpdate{
		Phase:   PhaseValidation,
		Message: "Validating migration parameters",
	})

	if err := m.validate(ctx, req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Parse S3 locations
	source, err := ParseS3URL(req.SourceArchive)
	if err != nil {
		return nil, fmt.Errorf("invalid source archive URL: %w", err)
	}

	dest, err := ParseS3URL(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("invalid destination URL: %w", err)
	}

	// Phase 2: Dry Run (if requested)
	if req.DryRun {
		estimate, err := m.EstimateMigration(ctx, req, source)
		if err != nil {
			return nil, fmt.Errorf("dry run estimation failed: %w", err)
		}
		return m.buildDryRunResult(estimate), nil
	}

	// Phase 3: Download & Extract
	m.sendProgress(ProgressUpdate{
		Phase:   PhaseExtraction,
		Message: "Downloading and extracting archive",
	})

	extractStart := time.Now()
	tempDir, extractStats, err := m.extractArchive(ctx, req, source)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}
	extractDuration := time.Since(extractStart)

	result.TempDirectory = tempDir
	result.FilesExtracted = extractStats.FilesExtracted
	result.BytesExtracted = extractStats.BytesExtracted

	// Ensure cleanup of temp directory
	defer func() {
		if !req.KeepTemp {
			if err := os.RemoveAll(tempDir); err != nil {
				slog.Warn("failed to cleanup temp directory",
					"path", tempDir,
					"error", err)
				result.Errors = append(result.Errors, fmt.Errorf("cleanup: %w", err))
			}
		}
	}()

	// Phase 4: Re-upload via Pipeline
	m.sendProgress(ProgressUpdate{
		Phase:   PhaseUpload,
		Message: "Uploading to CargoHold format",
	})

	uploadStart := time.Now()
	uploadResult, err := m.uploadCargoHold(ctx, req, dest, tempDir)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	uploadDuration := time.Since(uploadStart)

	result.UploadID = uploadResult.UploadID
	result.FilesUploaded = uploadResult.FilesUploaded
	result.BytesUploaded = uploadResult.BytesUploaded
	result.ShardsCreated = uploadResult.ShardsCreated
	result.ChunksCreated = uploadResult.ChunksCreated
	result.CompressionRatio = uploadResult.CompressionRatio
	result.ManifestLocation = uploadResult.ManifestLocation

	// Phase 5: Cleanup & Finalization
	m.sendProgress(ProgressUpdate{
		Phase:   PhaseCleanup,
		Message: "Finalizing migration",
	})

	// Delete original archive if requested and upload succeeded
	if req.DeleteOriginal && uploadResult.Success {
		if err := m.deleteOriginalArchive(ctx, source); err != nil {
			slog.Warn("failed to delete original archive",
				"bucket", source.Bucket,
				"key", source.Key,
				"error", err)
			result.Errors = append(result.Errors, fmt.Errorf("delete original: %w", err))
		} else {
			result.OriginalArchiveDeleted = true
		}
	}

	// Success!
	result.Success = true
	result.Duration = time.Since(startTime)
	result.ExtractionTime = extractDuration
	result.UploadTime = uploadDuration

	m.sendProgress(ProgressUpdate{
		Phase:   PhaseComplete,
		Message: "Migration complete",
	})

	return result, nil
}

// validate performs pre-flight validation checks
func (m *Migrator) validate(ctx context.Context, req *MigrateRequest) error {
	if req.SkipValidation {
		return nil
	}

	// Parse source and destination
	source, err := ParseS3URL(req.SourceArchive)
	if err != nil {
		return fmt.Errorf("invalid source archive URL: %w", err)
	}

	dest, err := ParseS3URL(req.Destination)
	if err != nil {
		return fmt.Errorf("invalid destination URL: %w", err)
	}

	// Check source archive exists
	headOutput, err := m.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(source.Bucket),
		Key:    aws.String(source.Key),
	})
	if err != nil {
		return fmt.Errorf("source archive not accessible: %w", err)
	}

	// Check destination bucket is writable
	_, err = m.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(dest.Bucket),
	})
	if err != nil {
		return fmt.Errorf("destination bucket not accessible: %w", err)
	}

	// Check disk space if temp directory is specified
	if req.TempDir != "" {
		archiveSize := aws.ToInt64(headOutput.ContentLength)
		requiredSpace := int64(float64(archiveSize) * 1.5) // Estimate 1.5x for safety

		available, err := getAvailableDiskSpace(req.TempDir)
		if err != nil {
			slog.Warn("could not check disk space", "error", err)
		} else if available < requiredSpace {
			return fmt.Errorf("insufficient disk space: need %d bytes, have %d bytes", requiredSpace, available)
		}
	}

	// Validate shard count
	if req.ShardCount < 1 || req.ShardCount > 100 {
		return fmt.Errorf("shard count must be between 1 and 100, got %d", req.ShardCount)
	}

	// Validate compression level
	if req.CompressionLevel < 1 || req.CompressionLevel > 22 {
		return fmt.Errorf("compression level must be between 1 and 22, got %d", req.CompressionLevel)
	}

	return nil
}

// extractArchive downloads and extracts the archive to a temporary directory
func (m *Migrator) extractArchive(ctx context.Context, req *MigrateRequest, source *S3Location) (string, *extractionStats, error) {
	// Create temp directory
	tempDirBase := req.TempDir
	if tempDirBase == "" {
		tempDirBase = os.TempDir()
	}

	tempDir, err := os.MkdirTemp(tempDirBase, "cargoship-migrate-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	slog.Info("extracting archive to temp directory",
		"source", req.SourceArchive,
		"temp_dir", tempDir)

	// Configure extractor
	extractorConfig := &extraction.ExtractorConfig{
		S3Client:            m.s3Client,
		Bucket:              source.Bucket,
		Key:                 source.Key,
		OutputDir:           tempDir,
		CompressionFormat:   extraction.CompressionAuto, // Auto-detect
		OverwriteExisting:   true,
		PreservePermissions: true,
		ProgressCallback: func(p extraction.ExtractProgress) {
			if !req.Quiet && m.config.ProgressCallback != nil {
				m.config.ProgressCallback(ProgressUpdate{
					Phase:                 PhaseExtraction,
					Message:               fmt.Sprintf("Extracting: %s", p.CurrentFile),
					FilesProcessed:        int64(p.FilesExtracted),
					BytesProcessed:        p.BytesExtracted,
					ThroughputBytesPerSec: float64(p.BytesExtracted) / p.ElapsedTime.Seconds(),
					Elapsed:               p.ElapsedTime,
				})
			}
		},
	}

	extractor, err := extraction.NewExtractor(extractorConfig)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to create extractor: %w", err)
	}

	// Extract the archive
	stats, err := extractor.Extract(ctx)
	if err != nil {
		// Cleanup temp directory on failure
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("extraction failed: %w", err)
	}

	extractStats := &extractionStats{
		FilesExtracted: int64(stats.FilesExtracted),
		BytesExtracted: stats.BytesExtracted,
	}

	slog.Info("extraction complete",
		"files", extractStats.FilesExtracted,
		"bytes", extractStats.BytesExtracted,
		"temp_dir", tempDir)

	return tempDir, extractStats, nil
}

// uploadCargoHold uploads the extracted files using the CargoHold pipeline
func (m *Migrator) uploadCargoHold(ctx context.Context, req *MigrateRequest, dest *S3Location, sourcePath string) (*uploadStats, error) {
	slog.Info("uploading to CargoHold format",
		"destination", req.Destination,
		"shards", req.ShardCount)

	// Configure pipeline
	pipelineConfig := &pipeline.PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 4,
		UploaderWorkers: 4,
		S3Bucket:        dest.Bucket,
		S3Prefix:        dest.Key,
		S3Region:        req.Region,
		UseRealS3:       true,
		S3Client:        m.s3Client,
		S3StorageClass:  req.StorageClass,
		S3PartSize:      64 * 1024 * 1024, // 64MB

		// CargoHold sharding
		EnableMultiPrefix:      true,
		EnableArchiverSharding: true,
		ShardCount:             req.ShardCount,
		WorkersPerPrefix:       2,

		// Manifest generation
		EnableManifest:        true,
		EnablePartialManifest: true,
		SourcePath:            sourcePath,

		// Cleanup on failure
		CleanupOnFailure: true,

		// Progress tracking
		EnableProgress:   !req.Quiet,
		ProgressInterval: 100 * time.Millisecond,
	}

	// Note: Compression level is handled by pipeline defaults (zstd level 3)
	// Progress reporting is handled via EnableProgress flag

	// Create and run pipeline
	pipe, err := pipeline.NewPipeline(pipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	result, err := pipe.Run(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("pipeline run failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("pipeline completed with errors: %d errors", len(result.Errors))
	}

	// Build upload stats
	stats := &uploadStats{
		Success:          result.Success,
		UploadID:         result.UploadID,
		FilesUploaded:    result.TotalFiles,
		BytesUploaded:    result.TotalBytes, // Uncompressed bytes
		ShardsCreated:    req.ShardCount,
		ChunksCreated:    result.TotalChunks,
		CompressionRatio: 0.0, // Not directly available from Result
		ManifestLocation: fmt.Sprintf("s3://%s/%s/uploads/%s/manifest.json.gz",
			dest.Bucket, dest.Key, result.UploadID),
	}

	slog.Info("upload complete",
		"upload_id", stats.UploadID,
		"files", stats.FilesUploaded,
		"bytes", stats.BytesUploaded,
		"shards", stats.ShardsCreated,
		"chunks", stats.ChunksCreated)

	return stats, nil
}

// deleteOriginalArchive deletes the source archive from S3
func (m *Migrator) deleteOriginalArchive(ctx context.Context, source *S3Location) error {
	slog.Info("deleting original archive",
		"bucket", source.Bucket,
		"key", source.Key)

	_, err := m.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(source.Bucket),
		Key:    aws.String(source.Key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete original archive: %w", err)
	}

	return nil
}

// EstimateMigration estimates the cost and resources for a migration
func (m *Migrator) EstimateMigration(ctx context.Context, req *MigrateRequest, source *S3Location) (*DryRunEstimate, error) {
	// Get source archive metadata
	headOutput, err := m.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(source.Bucket),
		Key:    aws.String(source.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get archive metadata: %w", err)
	}

	archiveSize := aws.ToInt64(headOutput.ContentLength)

	// Estimate uncompressed size (assume 3x compression ratio for zstd)
	estimatedUncompressed := archiveSize * 3

	// Calculate temp disk space requirement (1.5x safety margin)
	tempDiskRequired := int64(float64(estimatedUncompressed) * 1.5)

	// Check available disk space
	tempDirBase := req.TempDir
	if tempDirBase == "" {
		tempDirBase = os.TempDir()
	}
	availableDisk, _ := getAvailableDiskSpace(tempDirBase)

	// Estimate costs (rough approximations)
	downloadCost := float64(archiveSize) * 0.09 / (1024 * 1024 * 1024)       // $0.09/GB out
	uploadCost := float64(req.ShardCount) * 0.005                             // $0.005/1000 PUT requests
	storageCost := float64(estimatedUncompressed) * 0.023 / (1024 * 1024 * 1024) // $0.023/GB/month

	// Estimate duration (assume 10 MB/s download, 20 MB/s upload per shard)
	downloadTime := time.Duration(archiveSize/10/1024/1024) * time.Second
	uploadTime := time.Duration(estimatedUncompressed/(20*int64(req.ShardCount))/1024/1024) * time.Second
	estimatedDuration := downloadTime + uploadTime

	// Build estimate
	estimate := &DryRunEstimate{
		SourceArchiveSize:         archiveSize,
		EstimatedUncompressedSize: estimatedUncompressed,
		TempDiskSpaceRequired:     tempDiskRequired,
		AvailableDiskSpace:        availableDisk,
		EstimatedShards:           req.ShardCount,
		EstimatedChunks:           int(estimatedUncompressed / (100 * 1024 * 1024)), // 100MB per chunk estimate
		EstimatedDownloadCost:     downloadCost,
		EstimatedUploadCost:       uploadCost,
		EstimatedStorageCost:      storageCost,
		EstimatedDuration:         estimatedDuration,
		CanProceed:                availableDisk >= tempDiskRequired,
		Warnings:                  make([]string, 0),
	}

	// Add warnings
	if estimate.AvailableDiskSpace < estimate.TempDiskSpaceRequired {
		estimate.Warnings = append(estimate.Warnings,
			fmt.Sprintf("Insufficient disk space: need %d GB, have %d GB",
				estimate.TempDiskSpaceRequired/(1024*1024*1024),
				estimate.AvailableDiskSpace/(1024*1024*1024)))
	}

	if estimate.EstimatedUncompressedSize > 1024*1024*1024*1024 { // 1TB
		estimate.Warnings = append(estimate.Warnings,
			"Large archive (>1TB) may require significant time and resources")
	}

	return estimate, nil
}

// sendProgress sends a progress update to the callback if configured
func (m *Migrator) sendProgress(update ProgressUpdate) {
	if m.config != nil && m.config.ProgressCallback != nil {
		m.config.ProgressCallback(update)
	}
}

// buildDryRunResult builds a result for dry run mode
func (m *Migrator) buildDryRunResult(estimate *DryRunEstimate) *MigrateResult {
	return &MigrateResult{
		Success: estimate.CanProceed,
		// Other fields not applicable for dry run
	}
}

// ParseS3URL parses an S3 URL into components
func ParseS3URL(s3url string) (*S3Location, error) {
	if !strings.HasPrefix(s3url, "s3://") {
		return &S3Location{IsValid: false}, fmt.Errorf("URL must start with s3://")
	}

	// Remove s3:// prefix
	path := strings.TrimPrefix(s3url, "s3://")

	// Split into bucket and key
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return &S3Location{IsValid: false}, fmt.Errorf("invalid S3 URL: missing bucket")
	}

	location := &S3Location{
		Bucket:  parts[0],
		IsValid: true,
	}

	if len(parts) > 1 {
		location.Key = parts[1]
	}

	return location, nil
}

// getAvailableDiskSpace returns the available disk space for a path
func getAvailableDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Available blocks * block size
	available := int64(stat.Bavail) * int64(stat.Bsize)
	return available, nil
}

// extractionStats contains statistics from the extraction phase
type extractionStats struct {
	FilesExtracted int64
	BytesExtracted int64
}

// uploadStats contains statistics from the upload phase
type uploadStats struct {
	Success          bool
	UploadID         string
	FilesUploaded    int64
	BytesUploaded    int64
	ShardsCreated    int
	ChunksCreated    int
	CompressionRatio float64
	ManifestLocation string
}
