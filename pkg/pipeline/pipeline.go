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
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"github.com/scttfrdmn/cargoship/pkg/resume"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	// Issue #34 Phase 4.2: Optimized GC tuning for reduced pause times
	// Configure GC for optimal pipeline performance
	// Default: GOGC=100 (GC runs when heap grows 100% from previous GC)
	//
	// For CargoShip pipeline:
	// - Memory is dominated by AWS SDK (256MB), zstd (226MB), I/O (320MB) = 92% unavoidable
	// - Setting GOGC=200 (was 150) reduces GC frequency, improving throughput with minimal memory increase
	// - GOMEMLIMIT provides soft memory ceiling for more predictable behavior
	// - Users can override with GOGC/GOMEMLIMIT environment variables
	//
	// Override priority:
	// 1. GOGC env var (user control)
	// 2. GOMEMLIMIT env var (Go 1.19+, soft memory target)
	// 3. Default: GOGC=200, GOMEMLIMIT=6GB (optimized for throughput)

	// Set GOMEMLIMIT if not already set (Go 1.19+)
	// Default to 6GB soft limit (suitable for systems with 8GB+ RAM)
	if os.Getenv("GOMEMLIMIT") == "" {
		const defaultMemLimit = 6 * 1024 * 1024 * 1024 // 6GB
		debug.SetMemoryLimit(defaultMemLimit)
	}

	// Set GOGC if not already set
	if os.Getenv("GOGC") == "" {
		// Issue #34 Phase 4.2: Increased from 150 to 200 for better throughput
		debug.SetGCPercent(200)
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

	// Apply defaults if not specified
	// Note: CLI now uses adaptive scaling to set these values explicitly
	// These defaults only apply when using the library directly
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

	// Local state persistence defaults (Issue #119)
	if config.LocalStateSaveInterval == 0 {
		config.LocalStateSaveInterval = 30 * time.Second
	}
	// EnableLocalState defaults to true if using real S3
	if config.UseRealS3 && !config.ResumeMode {
		config.EnableLocalState = true
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

	// Issue #166: Direct upload optimization defaults
	if config.DirectUploadThresholdMB == 0 {
		config.DirectUploadThresholdMB = 500 // Default: 500MB max for direct upload
	}
	if config.DirectUploadMaxFiles == 0 {
		config.DirectUploadMaxFiles = 50000 // Default: 50k files max for direct upload
	}
	if config.DirectUploadAvgSizeMB == 0 {
		config.DirectUploadAvgSizeMB = 5.0 // Default: 5MB average file size threshold
	}
	if config.DirectUploadWorkers == 0 {
		config.DirectUploadWorkers = 256 // Default: 256 workers (matches s5cmd)
	}
	// EnableAutoDirectUpload defaults to true for automatic optimization
	if !config.EnableDirectUpload && !config.ForceDirectUpload {
		config.EnableAutoDirectUpload = true
	}

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

	// Initialize tracer if enabled (Issue #155)
	if config.EnableTracing {
		if config.Tracer != nil {
			p.tracer = config.Tracer
		} else {
			p.tracer = tracing.NewPipelineTracer()
		}
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

			// Set encryption info (Issue #163)
			if config.KMSKeyID != "" || config.EncryptManifest {
				builder.SetEncryption(config.KMSKeyID, config.EncryptManifest)
			}
		}

		p.manifestBuilder = builder
	}

	// Initialize local upload state if enabled (Issue #119: Enhanced resume)
	if config.EnableLocalState && config.UseRealS3 {
		uploadState := &resume.UploadState{
			UploadID:        config.UploadID,
			StartTime:       time.Now(),
			LastSave:        time.Now(),
			SourceDir:       config.SourcePath,
			Bucket:          config.S3Bucket,
			Prefix:          config.S3Prefix,
			Region:          config.S3Region,
			StorageClass:    config.S3StorageClass,
			KMSKeyID:        config.KMSKeyID,
			EncryptManifest: config.EncryptManifest,
			ChunkSizeMB:     config.ForceChunkSizeMB,
			ShardCount:      config.ShardCount,
			Shards:          make([]resume.ShardState, config.ShardCount),
		}

		// Initialize shard states
		for i := 0; i < config.ShardCount; i++ {
			uploadState.Shards[i] = resume.ShardState{
				ShardID: i,
			}
		}

		p.uploadState = uploadState
	}

	return p, nil
}

// Run executes the pipeline
func (p *Pipeline) Run(ctx context.Context, rootPath string) (*Result, error) {
	// Create root upload span if tracing enabled (Issue #155)
	var uploadSpan trace.Span
	if p.tracer != nil {
		tracer := p.tracer.(*tracing.PipelineTracer)
		ctx, uploadSpan = tracer.StartUploadSpan(ctx, p.config.UploadID)
		defer func() {
			if uploadSpan != nil {
				if result := recover(); result != nil {
					tracer.RecordError(uploadSpan, fmt.Errorf("panic: %v", result))
					uploadSpan.End()
					panic(result) // Re-panic after recording
				}
				uploadSpan.End()
			}
		}()
	}

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

	// Start local state saving if enabled (Issue #119: Enhanced resume)
	if p.config.EnableLocalState && p.uploadState != nil {
		go p.saveLocalStatePeriodically(ctx)
	}

	// Start all stages
	if err := p.startStages(ctx, rootPath); err != nil {
		if uploadSpan != nil && p.tracer != nil {
			tracer := p.tracer.(*tracing.PipelineTracer)
			tracer.RecordError(uploadSpan, err)
		}
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

	// Record final span status (Issue #155)
	if uploadSpan != nil && p.tracer != nil {
		tracer := p.tracer.(*tracing.PipelineTracer)
		if result.Success {
			tracer.RecordSuccess(uploadSpan)
			tracer.AddFileAttributes(uploadSpan, "", result.TotalBytes, int(result.TotalFiles))
		} else if len(result.Errors) > 0 {
			tracer.RecordError(uploadSpan, result.Errors[0])
		}
	}

	return result, nil
}

// shouldUseDirectUpload determines if direct upload mode should be used based on workload characteristics
// Direct upload bypasses archiving/compression for better performance on small files
func (p *Pipeline) shouldUseDirectUpload(fileCount int64, totalSize int64) bool {
	// Force direct upload if explicitly requested (for testing/benchmarking)
	if p.config.ForceDirectUpload {
		return true
	}

	// Check if direct upload is explicitly enabled
	if p.config.EnableDirectUpload {
		return true
	}

	// Auto-detect if enabled
	if !p.config.EnableAutoDirectUpload {
		return false
	}

	// Don't use direct upload if no files
	if fileCount == 0 {
		return false
	}

	// Calculate average file size in MB
	avgFileSizeMB := float64(totalSize) / float64(fileCount) / (1024 * 1024)

	// Calculate total size in MB
	totalSizeMB := totalSize / (1024 * 1024)

	// Use direct upload if:
	// 1. Total size is under threshold AND
	// 2. Average file size is small (under threshold) OR file count is high
	underSizeThreshold := totalSizeMB < int64(p.config.DirectUploadThresholdMB)
	smallAvgSize := avgFileSizeMB < p.config.DirectUploadAvgSizeMB
	manySmallFiles := fileCount > 1000 && avgFileSizeMB < 10.0 // Many files under 10MB each

	return underSizeThreshold && (smallAvgSize || manySmallFiles)
}

// startStages initializes and starts all pipeline stages
func (p *Pipeline) startStages(ctx context.Context, rootPath string) error {
	var err error

	// Create scanner stage
	scannerConfig := &ScannerConfig{
		RootPath:                   rootPath,
		Workers:                    p.config.ScannerWorkers,
		IncludeOnlyFiles:           p.config.IncludeOnlyFiles,              // Issue #148: Incremental sync file filtering
		UseCompressedAwareChunking: p.config.EnableCompressedAwareChunking, // Phase 3.3
		ChunkTargetSizeMB:          p.config.ForceChunkSizeMB,              // Phase 3.3
		ChunkingConfig:             p.config.ChunkingConfig,                // Phase 5: Pass chunking config
		TierSelector:               p.config.TierSelector,                  // Issue #164: Tier-aware chunking
		TierChunkingStrategy:       p.config.TierChunkingStrategy,          // Issue #164: Tier chunking strategy
	}
	p.scanner, err = NewScannerStage(scannerConfig, p.chunkChan, p) // Pass pipeline reference for manifest tracking
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Issue #166: Check if we should use direct upload (fast path for small files)
	// Only check when using real S3 (not simulation mode)
	useDirectUpload := false
	if p.config.UseRealS3 {
		// Estimate workload to determine upload strategy
		fileCount, totalSize, err := EstimateWorkload(ctx, rootPath)
		if err != nil {
			// Log warning but continue with standard path
			fmt.Printf("Warning: Failed to estimate workload, using standard upload path: %v\n", err)
		} else {
			useDirectUpload = p.shouldUseDirectUpload(fileCount, totalSize)
			if useDirectUpload {
				avgSizeMB := float64(totalSize) / float64(fileCount) / (1024 * 1024)
				fmt.Printf("📦 Using direct upload mode (fast path): %d files, %.2f MB total, %.2f MB avg\n",
					fileCount, float64(totalSize)/(1024*1024), avgSizeMB)
			}
		}
	}

	// Issue #166: Direct upload path (bypasses archiving/compression for small files)
	if useDirectUpload {
		// Create direct uploader stage
		directConfig := &DirectUploaderConfig{
			S3Client:   p.config.S3Client.(S3Uploader),
			Bucket:     p.config.S3Bucket,
			Prefix:     p.config.S3Prefix,
			Workers:    p.config.DirectUploadWorkers,
			MaxRetries: 3,
			RetryDelay: time.Second,
		}

		p.directUploader, err = NewDirectUploaderStage(directConfig, p.chunkChan, p.resultChan)
		if err != nil {
			return fmt.Errorf("failed to create direct uploader: %w", err)
		}

		// Start scanner and direct uploader
		if err := p.scanner.Start(ctx); err != nil {
			return fmt.Errorf("failed to start scanner: %w", err)
		}

		if err := p.directUploader.Start(ctx); err != nil {
			return fmt.Errorf("failed to start direct uploader: %w", err)
		}

		// Direct upload mode - skip archiver and standard uploader creation
		return nil
	}

	// Standard upload path: archiver + uploader
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
				PartSize:     p.config.S3PartSize,
				MaxRetries:   3,
				RetryDelay:   time.Second,
				S3Client:     p.config.S3Client.(*s3.Client),
				Bucket:       p.config.S3Bucket,
				Prefix:       p.config.S3Prefix,
				StorageClass: types.StorageClass(p.config.S3StorageClass),
				TierSelector: p.config.TierSelector,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024
			}

			// v0.6.2: Add transporter if configured
			if p.config.Transporter != nil {
				if transporter, ok := p.config.Transporter.(s3transport.BasicTransporter); ok {
					s3Config.Transporter = transporter
				}
			}

			p.multiPrefixUploader, err = NewS3MultiPrefixUploaderStage(
				s3Config,
				uploaderInputs,
				p.resultChan,
				p.config.WorkersPerPrefix,
				p,   // Pass pipeline reference for manifest tracking
				nil, // Use default logger
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
				PartSize:     p.config.S3PartSize,
				MaxRetries:   3,
				RetryDelay:   time.Second,
				S3Client:     p.config.S3Client.(*s3.Client),
				Bucket:       p.config.S3Bucket,
				Prefix:       p.config.S3Prefix,
				StorageClass: types.StorageClass(p.config.S3StorageClass),
				TierSelector: p.config.TierSelector,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024 // 64MB default
			}

			// v0.6.2: Add transporter if configured
			if p.config.Transporter != nil {
				if transporter, ok := p.config.Transporter.(s3transport.BasicTransporter); ok {
					s3Config.Transporter = transporter
				}
			}

			p.multiPrefixUploader, err = NewS3MultiPrefixUploaderStage(
				s3Config,
				uploaderInputs,
				p.resultChan,
				p.config.WorkersPerPrefix,
				p,   // Pass pipeline reference for manifest tracking
				nil, // Use default logger
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
				Workers:      p.config.UploaderWorkers,
				PartSize:     p.config.S3PartSize,
				MaxRetries:   3,
				RetryDelay:   time.Second,
				S3Client:     p.config.S3Client.(*s3.Client),
				Bucket:       p.config.S3Bucket,
				Prefix:       p.config.S3Prefix,
				StorageClass: types.StorageClass(p.config.S3StorageClass),
				TierSelector: p.config.TierSelector,
			}

			if p.config.S3PartSize == 0 {
				s3Config.PartSize = 64 * 1024 * 1024 // 64MB default
			}

			// v0.6.2: Add transporter if configured
			if p.config.Transporter != nil {
				if transporter, ok := p.config.Transporter.(s3transport.BasicTransporter); ok {
					s3Config.Transporter = transporter
				}
			}

			p.s3Uploader, err = NewS3UploaderStage(s3Config, p.archiveChan, p.resultChan, p, nil)
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
		p.progress.progress.BytesProcessed += atomic.LoadInt64(&job.ArchiveSize)
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
	manifestData := builder.Finalize()
	p.manifestMu.Unlock()

	// Get S3 client
	s3Client := p.config.S3Client.(*s3.Client)

	// Check if manifest encryption is enabled (Issue #163)
	if p.config.EncryptManifest && p.config.KMSKeyID != "" && p.config.KMSClient != nil {
		// Get KMS client from config (implements encryption.KMSClient interface)
		kmsClient := p.config.KMSClient.(*kms.Client)

		// Update manifest encryption metadata with KMS key ID for manifest
		if manifestData.Encryption != nil {
			manifestData.Encryption.ManifestKMSKeyID = p.config.KMSKeyID
		}

		// Upload with encryption
		err := manifestData.UploadToS3WithEncryption(ctx, s3Client, kmsClient, true)
		if err != nil {
			return fmt.Errorf("failed to upload encrypted manifest: %w", err)
		}

		fmt.Printf("✅ Encrypted manifest uploaded: s3://%s/%s/uploads/%s/manifest.encrypted.json.gz (%d files, %d chunks)\n",
			p.config.S3Bucket, p.config.S3Prefix, p.config.UploadID, len(manifestData.Files), len(manifestData.Chunks))
	} else {
		// Regular upload without encryption
		err := manifestData.UploadToS3(ctx, s3Client, true)
		if err != nil {
			return fmt.Errorf("failed to upload manifest: %w", err)
		}

		fmt.Printf("✅ Manifest uploaded: s3://%s/%s/uploads/%s/manifest.json.gz (%d files, %d chunks)\n",
			p.config.S3Bucket, p.config.S3Prefix, p.config.UploadID, len(manifestData.Files), len(manifestData.Chunks))
	}

	// Delete partial manifest after successful upload (Issue #157)
	p.deletePartialManifest(ctx)

	// Delete local state after successful upload (Issue #119)
	p.deleteLocalState()

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

// saveLocalStatePeriodically saves upload state to local disk periodically (Issue #119)
func (p *Pipeline) saveLocalStatePeriodically(ctx context.Context) {
	interval := p.config.LocalStateSaveInterval
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
			if err := p.saveLocalState(); err != nil {
				// Log warning but don't fail the upload
				fmt.Printf("Warning: Failed to save local state: %v\n", err)
			}
		}
	}
}

// saveLocalState saves the current upload state to local disk (Issue #119)
func (p *Pipeline) saveLocalState() error {
	if p.uploadState == nil {
		return fmt.Errorf("upload state not initialized")
	}

	p.uploadStateMu.Lock()
	state := p.uploadState.(*resume.UploadState)

	// Update progress from pipeline tracker
	p.progress.mu.RLock()
	state.TotalFiles = p.progress.progress.TotalFiles
	state.TotalBytes = p.progress.progress.TotalBytes
	state.CompletedFiles = p.progress.progress.FilesProcessed
	state.CompletedBytes = p.progress.progress.BytesProcessed
	p.progress.mu.RUnlock()

	// Save to disk with atomic write
	err := resume.SaveState(state)
	p.uploadStateMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to save state to disk: %w", err)
	}

	return nil
}

// deleteLocalState removes the local state file after successful completion (Issue #119)
func (p *Pipeline) deleteLocalState() {
	if p.uploadState == nil {
		return
	}

	p.uploadStateMu.Lock()
	state := p.uploadState.(*resume.UploadState)
	uploadID := state.UploadID
	p.uploadStateMu.Unlock()

	if err := resume.DeleteState(uploadID); err != nil {
		// Log but don't fail - state cleanup is not critical
		fmt.Printf("Warning: Failed to delete local state for %s: %v\n", uploadID, err)
	}
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

	// Issue #166: Stop direct uploader if present
	if p.directUploader != nil {
		if err := p.directUploader.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("direct uploader stop error: %w", err))
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
