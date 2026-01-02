package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/detection"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
)

// ScannerStage discovers files and creates chunks
type ScannerStage struct {
	config   *ScannerConfig
	output   chan<- *Job
	pool     *WorkerPool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
	stats    StageStats
	strategy chunking.ChunkingStrategy

	// Phase 3.3: Compressed-aware chunker for optimal chunk sizing
	compressedChunker *chunking.CompressedAwareChunker

	// Magika detector for AI file type detection (Issue #30)
	magikaDetector *detection.MagikaDetector

	// Issue #34 Phase 4.1: Worker pool for parallel batch processing (4 workers)
	batchWorkerPool *WorkerPool

	// Atomic counters
	jobsProcessed  int64
	bytesProcessed int64

	// Error from run() method
	runError error

	// Manifest tracking (Issue #97)
	pipeline *Pipeline // Reference to parent pipeline for manifest tracking
}

// NewScannerStage creates a new scanner stage
func NewScannerStage(config *ScannerConfig, output chan<- *Job, pipeline *Pipeline) (*ScannerStage, error) {
	if config == nil {
		return nil, fmt.Errorf("scanner config cannot be nil")
	}

	// Create chunking strategy - use provided config or fall back to defaults
	chunkingConfig := config.ChunkingConfig
	if chunkingConfig == nil {
		// Default configuration (Phase 5: file splitting disabled by default for backward compatibility)
		chunkingConfig = &chunking.ChunkingConfig{
			Workers:             8,
			AvailableMemory:     4 * 1024 * 1024 * 1024, // 4GB
			GroupingStrategy:    "mixed",
			CostSavingsTarget:   1000,
			EnableFileSplitting: false,             // Disabled by default
			MaxFileChunkSize:    200 * 1024 * 1024, // 200MB chunks for split files
		}
	}
	// Create base chunking strategy
	var strategy chunking.ChunkingStrategy
	strategy = chunking.NewAdaptiveChunkingStrategy(chunkingConfig)

	// Issue #164: Wrap strategy with TierAwareChunker if tier-aware chunking is enabled
	if config.TierChunkingStrategy == "tier-aware" && config.TierSelector != nil {
		bufferSize := chunkingConfig.TierGroupBufferSize
		if bufferSize <= 0 {
			bufferSize = 100000 // Default: 100k files per tier
		}
		strategy = chunking.NewTierAwareChunker(strategy, config.TierSelector, bufferSize)
	}

	// Phase 3.3: Initialize compressed-aware chunker if enabled
	var compressedChunker *chunking.CompressedAwareChunker
	if config.UseCompressedAwareChunking {
		chunker, err := chunking.NewCompressedAwareChunker()
		if err != nil {
			return nil, fmt.Errorf("failed to create compressed-aware chunker: %w", err)
		}
		compressedChunker = chunker
	}

	// Issue #30: Initialize Magika detector if enabled
	var magikaDetector *detection.MagikaDetector
	if config.MagikaConfig != nil && config.MagikaConfig.Enabled {
		detector, err := detection.NewMagikaDetector(*config.MagikaConfig)
		if err != nil {
			// Log warning but don't fail - graceful degradation
			fmt.Fprintf(os.Stderr, "⚠️  Magika initialization failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Falling back to extension-based detection\n")
		} else {
			magikaDetector = detector
		}
	}

	return &ScannerStage{
		config:            config,
		output:            output,
		strategy:          strategy,
		compressedChunker: compressedChunker,
		magikaDetector:    magikaDetector,
		stats: StageStats{
			Name: "scanner",
		},
		pipeline: pipeline, // Store reference for manifest tracking
	}, nil
}

// Name returns the stage name
func (s *ScannerStage) Name() string {
	return "scanner"
}

// Start starts the scanner stage
func (s *ScannerStage) Start(ctx context.Context) error {
	// Create child context from parent (inherits trace context for Issue #155)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Initialize worker pool with inherited context
	s.pool = NewWorkerPool(s.ctx, s.config.Workers)

	// Issue #34 Phase 4.1: Initialize batch processing pool (4 workers for parallel batching)
	s.batchWorkerPool = NewWorkerPool(s.ctx, 4)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.run(s.ctx); err != nil {
			// Store error for pipeline to check
			s.mu.Lock()
			s.runError = err
			s.mu.Unlock()
		}
	}()
	return nil
}

// Stop stops the scanner stage
func (s *ScannerStage) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.pool != nil {
		s.pool.Stop()
	}
	s.wg.Wait()
	return nil
}

// Process is not used for scanner (it's the source stage)
func (s *ScannerStage) Process(ctx context.Context, job *Job) error {
	return fmt.Errorf("scanner is a source stage and does not process jobs")
}

// Stats returns stage statistics
func (s *ScannerStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.pool.AvailableWorkers()

	return stats
}

// run executes the scanner logic with streaming file discovery
func (s *ScannerStage) run(ctx context.Context) error {
	startTime := time.Now()
	defer close(s.output) // Close output when done

	// Create stage span if tracing enabled (Issue #155)
	if s.pipeline != nil && s.pipeline.tracer != nil {
		tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
		var stageSpan interface{} // trace.Span
		ctx, stageSpan = tracer.StartStageSpan(ctx, "scanner")
		defer func() {
			if span, ok := stageSpan.(interface{ End() }); ok {
				span.End()
			}
		}()
	}

	// Stream files instead of loading all at once
	fileChan, errChan := s.streamFiles(ctx, s.config.RootPath)

	// Collect files in batches for chunking
	var batch []chunking.File
	var totalSize int64
	const batchSize = 1000 // Process 1000 files at a time

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("failed to stream files: %w", err)
			}
		case file, ok := <-fileChan:
			if !ok {
				// Channel closed, process final batch
				// Issue #34 Phase 4.1: Submit final batch to pool
				if len(batch) > 0 {
					batchCopy := make([]chunking.File, len(batch))
					copy(batchCopy, batch)
					batchSizeCopy := totalSize

					if err := s.batchWorkerPool.Submit(func(ctx context.Context) error {
						return s.processBatch(ctx, batchCopy, batchSizeCopy)
					}); err != nil {
						return err
					}
				}
				goto done
			}

			batch = append(batch, file)
			totalSize += file.Size

			// Issue #34 Phase 4.1: Process batch when full (parallel submission)
			if len(batch) >= batchSize {
				// Create a copy of the batch for async processing
				batchCopy := make([]chunking.File, len(batch))
				copy(batchCopy, batch)
				batchSizeCopy := totalSize

				// Submit to batch worker pool (non-blocking)
				if err := s.batchWorkerPool.Submit(func(ctx context.Context) error {
					return s.processBatch(ctx, batchCopy, batchSizeCopy)
				}); err != nil {
					return err
				}

				batch = batch[:0] // Clear batch but keep capacity
				totalSize = 0
			}
		}
	}

done:
	// Issue #34 Phase 4.1: Wait for all batch processing jobs to complete
	s.batchWorkerPool.Wait()

	s.mu.Lock()
	s.stats.TotalTime = time.Since(startTime)
	if s.stats.JobsProcessed > 0 {
		s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
	}
	s.mu.Unlock()

	return nil
}

// shouldExclude checks if a path should be excluded
func (s *ScannerStage) shouldExclude(path string) bool {
	if len(s.config.ExcludePatterns) == 0 {
		return false
	}

	for _, pattern := range s.config.ExcludePatterns {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}

	return false
}

// shouldInclude checks if a file should be included based on IncludeOnlyFiles filter (Issue #148)
// Returns true if file should be included:
// - If IncludeOnlyFiles is empty, all files are included
// - If IncludeOnlyFiles is set, only files in the list are included
func (s *ScannerStage) shouldInclude(path string, rootPath string) bool {
	// If no filter is set, include all files
	if len(s.config.IncludeOnlyFiles) == 0 {
		return true
	}

	// Get relative path from root for comparison
	relPath, err := filepath.Rel(rootPath, path)
	if err != nil {
		// If we can't compute relative path, exclude it to be safe
		return false
	}

	// Check if this file is in the include list
	for _, includePath := range s.config.IncludeOnlyFiles {
		if relPath == includePath {
			return true
		}
	}

	return false
}

// streamFiles streams files from the root path via a channel instead of loading all into memory
func (s *ScannerStage) streamFiles(ctx context.Context, rootPath string) (<-chan chunking.File, <-chan error) {
	fileChan := make(chan chunking.File, 100) // Buffer for 100 files
	errChan := make(chan error, 1)

	go func() {
		defer close(fileChan)
		defer close(errChan)

		err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Skip directories
			if info.IsDir() {
				return nil
			}

			// Skip symlinks if not following
			if info.Mode()&os.ModeSymlink != 0 && !s.config.FollowSymlinks {
				return nil
			}

			// Check exclude patterns
			if s.shouldExclude(path) {
				return nil
			}

			// Check include filter (Issue #148: Incremental sync)
			if !s.shouldInclude(path, rootPath) {
				return nil
			}

			// Extract extended file times (atime, ctime) if available
			// Non-fatal: if extraction fails, continue with zero values
			atime, mtime, ctime, err := GetFileTimes(info)
			if err != nil {
				atime = time.Time{} // Zero value
				mtime = info.ModTime()
				ctime = time.Time{}
			}

			// Initialize metadata map
			metadata := make(map[string]string)

			// Store extended times in metadata (if available)
			if !atime.IsZero() {
				metadata["atime"] = atime.Format(time.RFC3339)
			}
			if !ctime.IsZero() {
				metadata["ctime"] = ctime.Format(time.RFC3339)
			}

			// Stream file to channel (no slice accumulation!)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case fileChan <- chunking.File{
				Path:      path,
				Size:      info.Size(),
				ModTime:   mtime,
				Directory: filepath.Dir(path),
				Metadata:  metadata,
			}:
			}

			return nil
		})

		if err != nil {
			errChan <- err
		}
	}()

	return fileChan, errChan
}

// processBatch processes a batch of files into chunks
func (s *ScannerStage) processBatch(ctx context.Context, files []chunking.File, totalSize int64) error {
	// Issue #30: Run Magika batch detection if enabled
	if s.magikaDetector != nil {
		files = s.enrichWithMagika(ctx, files)
	}

	// Issue #108: Check for duplicates if deduplication is enabled
	uniqueFiles := files
	if s.pipeline != nil && s.pipeline.dedupEnabled {
		uniqueFiles = s.filterDuplicates(ctx, files)
	}

	var chunks []chunking.Chunk
	var err error

	// Phase 3.3: Use compressed-aware chunking if enabled
	if s.compressedChunker != nil {
		result, err := s.compressedChunker.CreateChunksWithMetadata(uniqueFiles)
		if err != nil {
			return fmt.Errorf("failed to create compressed-aware chunks: %w", err)
		}
		chunks = result.Chunks

		// Log chunking decision
		fmt.Printf("Phase 3.3: Created %d chunks with %dMB target (total: %.2f GB compressed)\n",
			len(chunks), result.OptimalChunkSizeMB,
			float64(result.TotalEstimatedCompressedSize)/(1024*1024*1024))

		// Calculate target compressed size per chunk (average)
		targetCompressedSize := int64(0)
		if len(result.ChunkMetadata) > 0 {
			targetCompressedSize = result.TotalEstimatedCompressedSize / int64(len(result.ChunkMetadata))
		}

		// Send chunks with target sizes
		for i := range chunks {
			chunk := chunks[i]

			// Use metadata for this specific chunk if available
			estimatedCompressed := chunk.TotalSize / 2 // Default: 50% compression
			if i < len(result.ChunkMetadata) {
				estimatedCompressed = result.ChunkMetadata[i].EstimatedCompressedSize
			}

			// Track files in manifest (Issue #97)
			// Issue #34 Phase 1.4: Use AddFileBatch to eliminate double-locking
			if s.pipeline != nil && s.pipeline.manifestBuilder != nil {
				builder := s.pipeline.manifestBuilder.(*manifest.Builder)

				// Build entries array without holding any locks
				entries := make([]manifest.FileEntry, 0, len(chunk.Files))
				for _, file := range chunk.Files {
					entries = append(entries, manifest.FileEntry{
						Path:    file.Path,
						Size:    file.Size,
						ModTime: file.ModTime,
						ChunkID: chunk.ID,
						ShardID: -1, // Will be determined by archiver
						S3Key:   "", // Will be filled by uploader
					})
				}

				// Add all entries at once (Builder.AddFileBatch handles locking internally)
				builder.AddFileBatch(entries)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case s.output <- &Job{
				ID:                   chunk.ID,
				Chunk:                chunk,
				TargetCompressedSize: targetCompressedSize, // Phase 3.3
				EstimatedCompressed:  estimatedCompressed,  // Phase 3.3
				StartTime:            time.Now(),
			}:
				atomic.AddInt64(&s.jobsProcessed, 1)
				atomic.AddInt64(&s.bytesProcessed, chunk.TotalSize)
			}
		}
	} else {
		// Fallback: Use existing simple chunking strategy
		chunkSize, _ := s.strategy.CalculateOptimalChunkSize(
			totalSize,
			len(uniqueFiles),
			4*1024*1024*1024, // 4GB
			1000,             // 1000x cost savings
		)

		chunks, err = s.strategy.GroupFilesIntoChunks(uniqueFiles, chunkSize)
		if err != nil {
			return fmt.Errorf("failed to group files into chunks: %w", err)
		}

		// Send chunks without target sizes
		for i := range chunks {
			chunk := chunks[i]

			// Track files in manifest (Issue #97)
			// Issue #34 Phase 1.4: Use AddFileBatch to eliminate double-locking
			if s.pipeline != nil && s.pipeline.manifestBuilder != nil {
				builder := s.pipeline.manifestBuilder.(*manifest.Builder)

				// Build entries array without holding any locks
				entries := make([]manifest.FileEntry, 0, len(chunk.Files))
				for _, file := range chunk.Files {
					entries = append(entries, manifest.FileEntry{
						Path:    file.Path,
						Size:    file.Size,
						ModTime: file.ModTime,
						ChunkID: chunk.ID,
						ShardID: -1, // Will be determined by archiver
						S3Key:   "", // Will be filled by uploader
					})
				}

				// Add all entries at once (Builder.AddFileBatch handles locking internally)
				builder.AddFileBatch(entries)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case s.output <- &Job{
				ID:        chunk.ID,
				Chunk:     chunk,
				StartTime: time.Now(),
			}:
				atomic.AddInt64(&s.jobsProcessed, 1)
				atomic.AddInt64(&s.bytesProcessed, chunk.TotalSize)
			}
		}
	}

	return nil
}

// isObviousFileType checks if a file has an obvious type based on extension
// Issue #34 Phase 3.1: Pre-filter to skip expensive Magika detection for common types
func isObviousFileType(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	// Common obvious types that don't need AI detection (60-70% of typical files)
	obviousExtensions := map[string]bool{
		// Archives (always obvious)
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".7z": true, ".rar": true, ".tgz": true, ".tbz2": true,

		// Images (obvious formats)
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true,
		".svg": true, ".webp": true, ".ico": true, ".tiff": true, ".tif": true,

		// Video (obvious formats)
		".mp4": true, ".avi": true, ".mov": true, ".mkv": true, ".webm": true,
		".flv": true, ".wmv": true, ".m4v": true, ".mpg": true, ".mpeg": true,

		// Audio (obvious formats)
		".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true,
		".m4a": true, ".wma": true, ".opus": true,

		// Documents (common formats)
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,

		// Code/Text (obvious types)
		".txt": true, ".md": true, ".json": true, ".xml": true, ".yaml": true,
		".yml": true, ".csv": true, ".tsv": true, ".log": true, ".ini": true,
		".conf": true, ".cfg": true, ".properties": true,

		// Source code (programming languages)
		".go": true, ".py": true, ".js": true, ".ts": true, ".java": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
		".rb": true, ".php": true, ".sh": true, ".bash": true, ".zsh": true,
		".pl": true, ".r": true, ".sql": true, ".html": true, ".css": true,
		".scss": true, ".sass": true, ".less": true, ".vue": true, ".jsx": true,
		".tsx": true, ".swift": true, ".kt": true, ".scala": true, ".clj": true,

		// Binaries/Executables (obvious)
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".app": true,
		".deb": true, ".rpm": true, ".dmg": true, ".pkg": true, ".msi": true,

		// Database files
		".db": true, ".sqlite": true, ".sqlite3": true, ".mdb": true,
	}

	return obviousExtensions[ext]
}

// enrichWithMagika enriches file metadata with Magika AI detections (Issue #30)
// Issue #34 Phase 3.1: Lazy detection - skip Magika for files with obvious types
func (s *ScannerStage) enrichWithMagika(ctx context.Context, files []chunking.File) []chunking.File {
	if s.magikaDetector == nil || !s.magikaDetector.IsAvailable() {
		return files
	}

	// Issue #34 Phase 3.1: Pre-filter - only run Magika on files with unknown/ambiguous types
	var needsDetection []chunking.File

	for _, file := range files {
		if isObviousFileType(file.Path) {
			// Skip Magika for obvious types (60-70% of files typically)
			continue
		}
		needsDetection = append(needsDetection, file)
	}

	// If no files need detection, return early
	if len(needsDetection) == 0 {
		return files
	}

	// Extract paths for batch processing (only for files needing detection)
	paths := make([]string, len(needsDetection))
	for i, file := range needsDetection {
		paths[i] = file.Path
	}

	// Run Magika batch detection (only on subset)
	results, err := s.magikaDetector.DetectBatch(ctx, paths)
	if err != nil {
		// Log error but continue - non-fatal
		fmt.Fprintf(os.Stderr, "⚠️  Magika detection failed: %v\n", err)
		return files
	}

	// Enrich file metadata with detection results
	for i := range files {
		if result, ok := results[files[i].Path]; ok &&
			result.Result.Status == "ok" &&
			result.Result.Value.Output.CTLabel != "" {
			if files[i].Metadata == nil {
				files[i].Metadata = make(map[string]string)
			}

			// Store Magika content type label
			files[i].Metadata["magika_type"] = result.Result.Value.Output.CTLabel

			// Optionally store MIME type
			if result.Result.Value.Output.MimeType != "" {
				files[i].Metadata["magika_mime"] = result.Result.Value.Output.MimeType
			}

			// Optionally store confidence score
			if s.magikaDetector != nil && s.config.MagikaConfig != nil && s.config.MagikaConfig.IncludeScores {
				files[i].Metadata["magika_score"] = fmt.Sprintf("%.2f", result.Result.Value.Output.Score)
			}
		}
	}

	return files
}

// Error returns any error that occurred during scanning
func (s *ScannerStage) Error() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runError
}

// filterDuplicates checks files against the deduplication index and filters out duplicates (Issue #108)
// Returns only unique files that should be uploaded. Duplicate files are tracked in the manifest.
//
// Note: This is called during scanning, before we know the actual shard/chunk locations.
// The dedupIndex tracks hashes and will be updated with actual locations after upload.
func (s *ScannerStage) filterDuplicates(ctx context.Context, files []chunking.File) []chunking.File {
	if s.pipeline == nil || s.pipeline.dedupIndex == nil {
		return files
	}

	dedupIndex := s.pipeline.dedupIndex.(*manifest.FileDeduplicationIndex)
	var manifestBuilder *manifest.Builder
	if s.pipeline.manifestBuilder != nil {
		manifestBuilder = s.pipeline.manifestBuilder.(*manifest.Builder)
	}

	uniqueFiles := make([]chunking.File, 0, len(files))

	for _, file := range files {
		// Check if file with this content hash already exists
		// Use placeholder location since we don't know actual location yet
		isDuplicate, location, err := dedupIndex.AddFile(file.Path, -1, -1, "")

		if err != nil {
			// Error computing hash - treat as unique and upload anyway
			fmt.Fprintf(os.Stderr, "⚠️  Deduplication hash error for %s: %v (uploading anyway)\n", file.Path, err)
			uniqueFiles = append(uniqueFiles, file)
			continue
		}

		if isDuplicate && location != nil {
			// File is a duplicate of an already-processed file
			// Add to manifest with dedup reference, but don't upload
			if manifestBuilder != nil {
				manifestBuilder.AddFile(manifest.FileEntry{
					Path:              file.Path,
					Size:              file.Size,
					ModTime:           file.ModTime,
					ChunkID:           location.ChunkID,
					ShardID:           location.ShardID,
					S3Key:             location.S3Key,
					IsDuplicate:       true,
					DuplicateOfHash:   location.Hash,
					OriginalChunkID:   location.ChunkID,
					OriginalShardID:   location.ShardID,
					OriginalS3Key:     location.S3Key,
					Checksum:          location.Hash,
				})
			}

			// Skip adding to uniqueFiles - don't upload duplicates
			continue
		}

		// File is unique (first time seeing this content hash)
		// Add to list for upload - dedupIndex will be updated with actual location after upload
		uniqueFiles = append(uniqueFiles, file)

		// Store hash in file metadata for later location updates
		if file.Metadata == nil {
			file.Metadata = make(map[string]string)
		}
		if location != nil {
			file.Metadata["content_hash"] = location.Hash
		}
	}

	// Log deduplication stats
	duplicateCount := len(files) - len(uniqueFiles)
	if duplicateCount > 0 {
		dedupRatio := dedupIndex.DeduplicationRatio()
		fmt.Printf("🔍 Deduplication: %d/%d files are duplicates (%.1f%% space saved)\n",
			duplicateCount, len(files), dedupRatio*100)
	}

	return uniqueFiles
}
