package pipeline

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func init() {
	// Configure GC for optimal pipeline performance
	// Default: GOGC=100 (GC runs when heap grows 100% from previous GC)
	//
	// For CargoShip pipeline:
	// - Memory is dominated by AWS SDK (256MB), zstd (226MB), I/O (320MB) = 92% unavoidable
	// - Setting GOGC=150 trades ~50-100MB more memory for 10-15% better throughput
	// - Users can override with GOGC environment variable
	//
	// Override priority:
	// 1. GOGC env var (user control)
	// 2. GOMEMLIMIT env var (Go 1.19+, soft memory target)
	// 3. Default: 150 (optimized for throughput)
	if os.Getenv("GOGC") == "" && os.Getenv("GOMEMLIMIT") == "" {
		// Only set if user hasn't specified their own tuning
		debug.SetGCPercent(150)
	}

	// Respect user's GOGC setting if explicitly set to "off"
	if gogc := os.Getenv("GOGC"); gogc == "off" {
		debug.SetGCPercent(-1) // Disable automatic GC
	} else if gogc != "" {
		// User specified custom GOGC value
		if val, err := strconv.Atoi(gogc); err == nil {
			debug.SetGCPercent(val)
		}
	}
}

// NewPipeline creates a new streaming pipeline
func NewPipeline(config *PipelineConfig) (*Pipeline, error) {
	if config == nil {
		return nil, fmt.Errorf("pipeline config cannot be nil")
	}

	// Apply defaults
	if config.ScannerWorkers == 0 {
		config.ScannerWorkers = 4
	}
	if config.ArchiverWorkers == 0 {
		config.ArchiverWorkers = 8
	}
	if config.UploaderWorkers == 0 {
		config.UploaderWorkers = 4
	}
	if config.ChunkBufferSize == 0 {
		config.ChunkBufferSize = 100
	}
	if config.ArchiveBufferSize == 0 {
		config.ArchiveBufferSize = 100 // Increased from 50 for better parallelism
	}
	if config.ResultBufferSize == 0 {
		config.ResultBufferSize = 200 // Increased from 100 for better flow
	}
	if config.ProgressInterval == 0 {
		config.ProgressInterval = time.Second
	}

	// Multi-prefix optimization defaults (Phase 3)
	if config.ShardCount == 0 {
		config.ShardCount = 8 // Default: 8 shards for parallel S3 uploads
	}
	if config.WorkersPerPrefix == 0 {
		config.WorkersPerPrefix = 2 // Default: 2 workers per prefix
	}
	if config.UploadID == "" {
		// Generate unique upload ID: {timestamp}-{random}
		// Format: 20251126-a3f9c2d1
		timestamp := time.Now().UTC().Format("20060102")
		randomBytes := make([]byte, 4)
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, fmt.Errorf("failed to generate upload ID: %w", err)
		}
		config.UploadID = fmt.Sprintf("%s-%s", timestamp, hex.EncodeToString(randomBytes))
	}
	// Multi-prefix is opt-in, not automatic
	// Users must explicitly enable it via config.EnableMultiPrefix = true
	// This allows benchmarks and tests to properly compare Phase 2 vs Phase 3.1

	// Phase 3.3: Compressed-aware chunking defaults (opt-in for now)
	if config.MaxPaddingRatio == 0 {
		config.MaxPaddingRatio = 0.25 // Default: 25% max padding overhead
	}
	// EnableCompressedAwareChunking and EnableArchivePadding are false by default (opt-in)
	// ForceChunkSizeMB defaults to 0 (use adaptive sizing)

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pipeline{
		ctx:    ctx,
		cancel: cancel,
		config: config,

		// Create channels
		chunkChan:   make(chan *Job, config.ChunkBufferSize),
		archiveChan: make(chan *Job, config.ArchiveBufferSize),
		resultChan:  make(chan *Job, config.ResultBufferSize),

		// Progress tracking
		progress: &ProgressTracker{
			progress: Progress{
				StartTime: time.Now(),
			},
		},

		errors: []error{},
	}

	// Initialize manifest builder if enabled and using real S3
	if config.EnableManifest && config.UseRealS3 {
		var builder *manifest.Builder
		var err error

		// Issue #157: Resume mode - load existing partial manifest
		if config.ResumeMode && config.ResumeUploadID != "" {
			// Download partial manifest from S3
			partialManifest, downloadErr := manifest.DownloadPartialManifestFromS3(
				ctx,
				config.S3Client.(*s3.Client),
				config.S3Bucket,
				config.S3Prefix,
				config.ResumeUploadID,
			)
			if downloadErr != nil {
				return nil, fmt.Errorf("failed to download partial manifest for resume: %w", downloadErr)
			}

			// Create builder from existing manifest
			builder, err = manifest.NewBuilderFromExisting(partialManifest)
			if err != nil {
				return nil, fmt.Errorf("failed to create builder from existing manifest: %w", err)
			}

			// Override UploadID with the resumed upload ID
			config.UploadID = config.ResumeUploadID

			fmt.Printf("📦 Resuming upload %s (%d files, %d chunks already uploaded)\n",
				config.ResumeUploadID, len(partialManifest.Files), len(partialManifest.Chunks))
		} else {
			// Normal mode - create new manifest builder
			builder, err = manifest.NewBuilder(
				config.UploadID,
				config.SourcePath,
				config.S3Bucket,
				config.S3Prefix,
				config.S3Region,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create manifest builder: %w", err)
			}

			// Set compression info (will be updated by archiver with actual compression ratio)
			builder.SetCompression("zstd", 3, 1.0) // Default values

			// Set shard count for manifest
			builder.SetShardCount(config.ShardCount)

			// Set sync info for incremental sync (Issue #148)
			if config.SyncType != "" {
				builder.SetSyncInfo(config.SyncType, config.PreviousUploadID)
			}
		}

		p.manifestBuilder = builder
	}

	return p, nil
}

// Run executes the pipeline
func (p *Pipeline) Run(ctx context.Context, rootPath string) (*Result, error) {
	// Merge contexts
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start progress tracking if enabled
	if p.config.EnableProgress {
		go p.trackProgress(ctx)
	}

	// Start partial manifest saving if enabled (Issue #157: Resume capability)
	if p.config.EnablePartialManifest && p.manifestBuilder != nil && p.config.UseRealS3 {
		go p.savePartialManifestPeriodically(ctx)
	}

	// Start all stages
	if err := p.startStages(ctx, rootPath); err != nil {
		return nil, fmt.Errorf("failed to start stages: %w", err)
	}

	// Wait for completion
	result := p.waitForCompletion(ctx)

	// Copy uploaded keys to result (Issue #158)
	p.uploadedKeysMu.Lock()
	result.UploadedKeys = append([]string(nil), p.uploadedKeys...)
	result.UploadID = p.config.UploadID
	p.uploadedKeysMu.Unlock()

	// Check for scanner errors
	if scannerErr := p.scanner.Error(); scannerErr != nil {
		result.Success = false
		result.Errors = append(result.Errors, scannerErr)

		// Issue #158: Cleanup partial upload on failure
		if err := p.cleanupPartialUpload(ctx); err != nil {
			// Log but don't override the original error
			fmt.Printf("Warning: Cleanup failed: %v\n", err)
		}

		return result, scannerErr
	}

	// Issue #158: Check if upload failed and cleanup if needed
	if !result.Success {
		if err := p.cleanupPartialUpload(ctx); err != nil {
			fmt.Printf("Warning: Cleanup failed: %v\n", err)
		}
	}

	return result, nil
}

// startStages initializes and starts all pipeline stages
func (p *Pipeline) startStages(ctx context.Context, rootPath string) error {
	var err error

	// Create scanner stage
	scannerConfig := &ScannerConfig{
		RootPath:                   rootPath,
		Workers:                    p.config.ScannerWorkers,
		IncludeOnlyFiles:           p.config.IncludeOnlyFiles,             // Issue #148: Incremental sync file filtering
		UseCompressedAwareChunking: p.config.EnableCompressedAwareChunking, // Phase 3.3
		ChunkTargetSizeMB:          p.config.ForceChunkSizeMB,              // Phase 3.3
		ChunkingConfig:             p.config.ChunkingConfig,                // Phase 5: Pass chunking config
	}
	p.scanner, err = NewScannerStage(scannerConfig, p.chunkChan, p) // Pass pipeline reference for manifest tracking
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Create archiver stage based on configuration
	archiverConfig := &ArchiverConfig{
		Workers:         p.config.ArchiverWorkers,
		CompressionType: "zstd",
		BufferSize:      64 * 1024, // 64KB
		UploadID:        p.config.UploadID,
		ShardCount:      p.config.ShardCount,
		// Phase 3.3: Archive padding configuration
		EnablePadding:        p.config.EnableArchivePadding,
		MaxPaddingRatio:      p.config.MaxPaddingRatio,
		UseLowEntropyPadding: true, // S3-optimized zero-byte padding
	}

	// Create uploader stage (real S3 or simulated)
	if p.config.UseRealS3 {
		// Use real AWS S3 uploader
		if p.config.S3Client == nil {
			return fmt.Errorf("S3Client required when UseRealS3 is true")
		}

		// Phase 3.2: Direct archiver-to-uploader with built-in sharding (NO ROUTER)
		if p.config.EnableMultiPrefix && p.config.EnableArchiverSharding {
			// Create per-prefix channels for archiver to write to directly
			p.prefixChans = make(map[string]chan *Job)
			for i := 0; i < p.config.ShardCount; i++ {
				shardName := fmt.Sprintf("shard-%d", i)
				p.prefixChans[shardName] = make(chan *Job, p.config.ArchiveBufferSize/p.config.ShardCount)
			}

			// Create write-only map for archiver, read-only map for uploader
			archiverOutputs := make(map[string]chan<- *Job)
			uploaderInputs := make(map[string]<-chan *Job)
			for name, ch := range p.prefixChans {
				archiverOutputs[name] = ch
				uploaderInputs[name] = ch
			}

			// Create archiver with built-in sharding (NO p.archiveChan!)
			p.archiver, err = NewArchiverStageWithSharding(archiverConfig, p.chunkChan, archiverOutputs, p.config.ShardCount)
			if err != nil {
				return fmt.Errorf("failed to create archiver: %w", err)
			}

			// Create S3MultiPrefixUploader (same as Phase 3.1)
			s3Config := &S3UploaderConfig{
				PartSize:   p.config.S3PartSize,
				MaxRetries: 3,
				RetryDelay: time.Second,
				S3Client:   p.config.S3Client.(*s3.Client),
				Bucket:     p.config.S3Bucket,
				Prefix:     p.config.S3Prefix,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024
			}

			p.multiPrefixUploader, err = NewS3MultiPrefixUploaderStage(
				s3Config,
				uploaderInputs,
				p.resultChan,
				p.config.WorkersPerPrefix,
				p, // Pass pipeline reference for manifest tracking
			)
			if err != nil {
				return fmt.Errorf("failed to create multi-prefix S3 uploader: %w", err)
			}

			// NO ROUTER CREATED - archiver shards directly! ✅

		} else if p.config.EnableMultiPrefix {
			// Phase 3.1: Multi-prefix with router (original implementation)

			// Create archiver with single output (Phase 3.1 uses router)
			p.archiver, err = NewArchiverStage(archiverConfig, p.chunkChan, p.archiveChan)
			if err != nil {
				return fmt.Errorf("failed to create archiver: %w", err)
			}

			// Create per-prefix channels for parallel uploads
			p.prefixChans = make(map[string]chan *Job)
			for i := 0; i < p.config.ShardCount; i++ {
				shardName := fmt.Sprintf("shard-%d", i)
				// Buffered channels to allow some queueing per prefix
				p.prefixChans[shardName] = make(chan *Job, p.config.ArchiveBufferSize/p.config.ShardCount)
			}

			// Create directional channel maps for router (write-only) and uploader (read-only)
			routerOutputs := make(map[string]chan<- *Job)
			uploaderInputs := make(map[string]<-chan *Job)
			for name, ch := range p.prefixChans {
				routerOutputs[name] = ch  // Write-only for router
				uploaderInputs[name] = ch // Read-only for uploader
			}

			// Create PrefixRouter to distribute jobs to per-prefix channels
			p.router = NewPrefixRouter(p.archiveChan, routerOutputs)

			// Create S3MultiPrefixUploader with per-prefix worker pools
			s3Config := &S3UploaderConfig{
				PartSize:   p.config.S3PartSize,
				MaxRetries: 3,
				RetryDelay: time.Second,
				S3Client:   p.config.S3Client.(*s3.Client),
				Bucket:     p.config.S3Bucket,
				Prefix:     p.config.S3Prefix,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024 // 64MB default
			}

			p.multiPrefixUploader, err = NewS3MultiPrefixUploaderStage(
				s3Config,
				uploaderInputs,
				p.resultChan,
				p.config.WorkersPerPrefix,
				p, // Pass pipeline reference for manifest tracking
			)
			if err != nil {
				return fmt.Errorf("failed to create multi-prefix S3 uploader: %w", err)
			}
		} else {
			// Phase 2: Single-prefix legacy uploader

			// Create archiver with single output (Phase 2)
			p.archiver, err = NewArchiverStage(archiverConfig, p.chunkChan, p.archiveChan)
			if err != nil {
				return fmt.Errorf("failed to create archiver: %w", err)
			}

			s3Config := &S3UploaderConfig{
				Workers:    p.config.UploaderWorkers,
				PartSize:   p.config.S3PartSize,
				MaxRetries: 3,
				RetryDelay: time.Second,
				S3Client:   p.config.S3Client.(*s3.Client),
				Bucket:     p.config.S3Bucket,
				Prefix:     p.config.S3Prefix,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024 // 64MB default
			}

			p.s3Uploader, err = NewS3UploaderStage(s3Config, p.archiveChan, p.resultChan, p)
			if err != nil {
				return fmt.Errorf("failed to create S3 uploader: %w", err)
			}
		}
	} else {
		// Use simulated uploader for testing

		// Create archiver with single output (simulated uploader mode)
		p.archiver, err = NewArchiverStage(archiverConfig, p.chunkChan, p.archiveChan)
		if err != nil {
			return fmt.Errorf("failed to create archiver: %w", err)
		}

		uploaderConfig := &UploaderConfig{
			Workers:     p.config.UploaderWorkers,
			PartSize:    5 * 1024 * 1024, // 5MB parts
			Concurrency: 4,
			MaxRetries:  3,
			RetryDelay:  time.Second,
		}
		p.uploader, err = NewUploaderStage(uploaderConfig, p.archiveChan, p.resultChan)
		if err != nil {
			return fmt.Errorf("failed to create uploader: %w", err)
		}
	}

	// Start all stages
	if err := p.scanner.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scanner: %w", err)
	}

	if err := p.archiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start archiver: %w", err)
	}

	// Start the appropriate uploader
	if p.config.UseRealS3 {
		if p.config.EnableMultiPrefix {
			// Phase 3.2: Direct archiver sharding (NO ROUTER)
			if p.config.EnableArchiverSharding {
				// Only start multi-prefix uploader (router skipped)
				if err := p.multiPrefixUploader.Start(ctx); err != nil {
					return fmt.Errorf("failed to start multi-prefix S3 uploader: %w", err)
				}
			} else {
				// Phase 3.1: Start PrefixRouter and MultiPrefixUploader
				if err := p.router.Start(ctx); err != nil {
					return fmt.Errorf("failed to start prefix router: %w", err)
				}

				if err := p.multiPrefixUploader.Start(ctx); err != nil {
					return fmt.Errorf("failed to start multi-prefix S3 uploader: %w", err)
				}
			}
		} else {
			// Phase 2: Single-prefix uploader
			if err := p.s3Uploader.Start(ctx); err != nil {
				return fmt.Errorf("failed to start S3 uploader: %w", err)
			}
		}
	} else {
		if err := p.uploader.Start(ctx); err != nil {
			return fmt.Errorf("failed to start uploader: %w", err)
		}
	}

	return nil
}

// waitForCompletion waits for all stages to complete and collects results
func (p *Pipeline) waitForCompletion(ctx context.Context) *Result {
	result := &Result{
		Success: true,
	}

	// Wait for results
	for job := range p.resultChan {
		result.ChunksUploaded++

		if job.Error != nil {
			result.Success = false
			result.Errors = append(result.Errors, job.Error)
			result.FailedJobs = append(result.FailedJobs, job) // Issue #103: Track failed jobs
			p.mu.Lock()
			p.errors = append(p.errors, job.Error)
			p.mu.Unlock()
		}

		// Update progress
		p.progress.mu.Lock()
		p.progress.progress.ChunksCompleted++
		p.progress.progress.BytesProcessed += job.ArchiveSize
		p.progress.progress.FilesProcessed += int64(job.Chunk.FileCount)
		p.progress.mu.Unlock()
	}

	// Get final progress
	p.progress.mu.RLock()
	result.Progress = p.progress.progress
	result.TotalFiles = p.progress.progress.FilesProcessed
	result.TotalBytes = p.progress.progress.BytesProcessed
	result.ChunksCreated = p.progress.progress.ChunksCompleted
	p.progress.mu.RUnlock()

	result.TotalTime = time.Since(result.Progress.StartTime)

	// Step 5: Upload manifest to S3 after all chunks complete (Issue #97)
	if p.manifestBuilder != nil && p.config.UseRealS3 {
		if err := p.uploadManifest(ctx); err != nil {
			// Log warning but don't fail the upload
			fmt.Printf("Warning: Failed to upload manifest: %v\n", err)
		}
	}

	return result
}

// uploadManifest finalizes and uploads the manifest to S3
func (p *Pipeline) uploadManifest(ctx context.Context) error {
	builder := p.manifestBuilder.(*manifest.Builder)

	// Finalize the manifest
	p.manifestMu.Lock()
	manifestData := builder.Build()
	p.manifestMu.Unlock()

	// Serialize to JSON and compress with gzip
	manifestBytes, err := manifestData.ToJSONCompressed()
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// Construct S3 key: prefix/uploads/{uploadID}/manifest.json.gz
	s3Key := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", p.config.S3Prefix, p.config.UploadID)

	// Upload to S3 using the same S3 client
	s3Client := p.config.S3Client.(*s3.Client)
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024 // 5MB parts (manifest should be <1MB)
	})

	input := &s3.PutObjectInput{
		Bucket:      aws.String(p.config.S3Bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(manifestBytes),
		ContentType: aws.String("application/gzip"),
		Metadata: map[string]string{
			"cargoship-manifest-version": "1.0",
			"cargoship-upload-id":        p.config.UploadID,
			"cargoship-file-count":       fmt.Sprintf("%d", len(manifestData.Files)),
			"cargoship-chunk-count":      fmt.Sprintf("%d", len(manifestData.Chunks)),
		},
	}

	_, err = uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload manifest to S3: %w", err)
	}

	fmt.Printf("✅ Manifest uploaded: s3://%s/%s (%d files, %d chunks)\n",
		p.config.S3Bucket, s3Key, len(manifestData.Files), len(manifestData.Chunks))

	// Delete partial manifest after successful upload (Issue #157)
	p.deletePartialManifest(ctx)

	return nil
}

// savePartialManifestPeriodically periodically saves the partial manifest to S3 (Issue #157: Resume capability)
func (p *Pipeline) savePartialManifestPeriodically(ctx context.Context) {
	interval := p.config.PartialManifestSaveInterval
	if interval == 0 {
		interval = 30 * time.Second // Default: 30s
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.savePartialManifest(ctx); err != nil {
				// Log warning but don't fail the upload
				fmt.Printf("Warning: Failed to save partial manifest: %v\n", err)
			}
		}
	}
}

// savePartialManifest saves the current manifest state to S3 as manifest.partial.json.gz (Issue #157)
func (p *Pipeline) savePartialManifest(ctx context.Context) error {
	if p.manifestBuilder == nil {
		return fmt.Errorf("manifest builder not initialized")
	}

	builder := p.manifestBuilder.(*manifest.Builder)

	// Build current manifest state (non-finalized)
	p.manifestMu.Lock()
	manifestData := builder.Build()
	p.manifestMu.Unlock()

	// Skip if no chunks yet
	if len(manifestData.Chunks) == 0 {
		return nil
	}

	// Serialize to JSON and compress with gzip
	manifestBytes, err := manifestData.ToJSONCompressed()
	if err != nil {
		return fmt.Errorf("failed to serialize partial manifest: %w", err)
	}

	// Construct S3 key: prefix/uploads/{uploadID}/manifest.partial.json.gz
	s3Key := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", p.config.S3Prefix, p.config.UploadID)

	// Upload to S3 using the same S3 client
	s3Client := p.config.S3Client.(*s3.Client)
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024 // 5MB parts
	})

	input := &s3.PutObjectInput{
		Bucket:      aws.String(p.config.S3Bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(manifestBytes),
		ContentType: aws.String("application/gzip"),
		Metadata: map[string]string{
			"cargoship-manifest-version": "1.0-partial",
			"cargoship-upload-id":        p.config.UploadID,
			"cargoship-file-count":       fmt.Sprintf("%d", len(manifestData.Files)),
			"cargoship-chunk-count":      fmt.Sprintf("%d", len(manifestData.Chunks)),
			"cargoship-last-updated":     time.Now().Format(time.RFC3339),
		},
	}

	_, err = uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload partial manifest to S3: %w", err)
	}

	return nil
}

// deletePartialManifest removes the partial manifest from S3 after successful completion (Issue #157)
func (p *Pipeline) deletePartialManifest(ctx context.Context) {
	s3Key := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", p.config.S3Prefix, p.config.UploadID)
	s3Client := p.config.S3Client.(*s3.Client)

	_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.config.S3Bucket),
		Key:    aws.String(s3Key),
	})

	if err != nil {
		// Log warning but don't fail - partial manifest cleanup is non-critical
		fmt.Printf("Warning: Failed to delete partial manifest: %v\n", err)
	}
}

// trackProgress periodically reports progress
func (p *Pipeline) trackProgress(ctx context.Context) {
	ticker := time.NewTicker(p.config.ProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.reportProgress()
		}
	}
}

// reportProgress calculates and reports current progress
func (p *Pipeline) reportProgress() {
	p.progress.mu.Lock()
	defer p.progress.mu.Unlock()

	elapsed := time.Since(p.progress.progress.StartTime)
	p.progress.progress.ElapsedTime = elapsed

	// Calculate throughput
	if elapsed.Seconds() > 0 {
		p.progress.progress.FilesPerSecond = float64(p.progress.progress.FilesProcessed) / elapsed.Seconds()
		p.progress.progress.BytesPerSecond = float64(p.progress.progress.BytesProcessed) / elapsed.Seconds()
	}

	// Calculate ETA if we know total
	if p.progress.progress.TotalChunks > 0 {
		remaining := p.progress.progress.TotalChunks - p.progress.progress.ChunksCompleted
		if p.progress.progress.ChunksCompleted > 0 {
			avgTimePerChunk := elapsed / time.Duration(p.progress.progress.ChunksCompleted)
			p.progress.progress.EstimatedETA = avgTimePerChunk * time.Duration(remaining)
		}
	}

	// Call callback if registered
	if p.progress.callback != nil {
		p.progress.callback(p.progress.progress)
	}
}

// Stop gracefully stops the pipeline
func (p *Pipeline) Stop() error {
	p.cancel()

	// Stop all stages
	var errs []error

	if p.scanner != nil {
		if err := p.scanner.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("scanner stop error: %w", err))
		}
	}

	if p.archiver != nil {
		if err := p.archiver.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("archiver stop error: %w", err))
		}
	}

	if p.uploader != nil {
		if err := p.uploader.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("uploader stop error: %w", err))
		}
	}

	if p.s3Uploader != nil {
		if err := p.s3Uploader.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("s3 uploader stop error: %w", err))
		}
	}

	// Channels are closed by stages when they finish
	// No need to close them here

	if len(errs) > 0 {
		return fmt.Errorf("pipeline stop errors: %v", errs)
	}

	return nil
}

// GetProgress returns current pipeline progress
func (p *Pipeline) GetProgress() Progress {
	p.progress.mu.RLock()
	defer p.progress.mu.RUnlock()
	return p.progress.progress
}

// SetProgressCallback sets a callback for progress updates
func (p *Pipeline) SetProgressCallback(callback func(Progress)) {
	p.progress.mu.Lock()
	defer p.progress.mu.Unlock()
	p.progress.callback = callback
}

// GetErrors returns all errors encountered during pipeline execution
func (p *Pipeline) GetErrors() []error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]error{}, p.errors...)
}

// GetStats returns statistics for all stages
func (p *Pipeline) GetStats() map[string]StageStats {
	stats := make(map[string]StageStats)

	if p.scanner != nil {
		stats["scanner"] = p.scanner.Stats()
	}

	if p.archiver != nil {
		stats["archiver"] = p.archiver.Stats()
	}

	if p.uploader != nil {
		stats["uploader"] = p.uploader.Stats()
	}

	if p.s3Uploader != nil {
		stats["s3_uploader"] = p.s3Uploader.Stats()
	}

	// Phase 3.1: Multi-prefix parallel upload stages
	if p.router != nil {
		stats["prefix_router"] = p.router.Stats()
	}

	if p.multiPrefixUploader != nil {
		stats["s3_multiprefix_uploader"] = p.multiPrefixUploader.Stats()
	}

	return stats
}

// trackUploadedKey adds an S3 key to the list of uploaded keys for cleanup (Issue #158)
func (p *Pipeline) trackUploadedKey(s3Key string) {
	p.uploadedKeysMu.Lock()
	defer p.uploadedKeysMu.Unlock()
	p.uploadedKeys = append(p.uploadedKeys, s3Key)
}

// cleanupPartialUpload deletes uploaded chunks and partial manifest on failure (Issue #158)
func (p *Pipeline) cleanupPartialUpload(ctx context.Context) error {
	if !p.config.CleanupOnFailure || !p.config.UseRealS3 {
		return nil
	}

	s3Client := p.config.S3Client.(*s3.Client)

	p.uploadedKeysMu.Lock()
	keys := append([]string(nil), p.uploadedKeys...)
	p.uploadedKeysMu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	fmt.Printf("\n🧹 Cleaning up %d partial chunks...\n", len(keys))

	// Add partial manifest to cleanup list
	partialManifestKey := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", p.config.S3Prefix, p.config.UploadID)
	keys = append(keys, partialManifestKey)

	// DeleteObjects supports up to 1000 keys per request
	var deletedCount int
	for i := 0; i < len(keys); i += 1000 {
		end := i + 1000
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]

		// Build delete request
		objects := make([]types.ObjectIdentifier, len(batch))
		for j, key := range batch {
			objects[j] = types.ObjectIdentifier{
				Key: aws.String(key),
			}
		}

		_, err := s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(p.config.S3Bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true), // Don't return successfully deleted objects
			},
		})

		if err != nil {
			// Log but don't fail - cleanup is best effort
			fmt.Printf("⚠️  Warning: Failed to delete some objects: %v\n", err)
		} else {
			deletedCount += len(batch)
		}
	}

	fmt.Printf("✅ Cleanup complete: %d objects deleted\n", deletedCount)
	return nil
}
