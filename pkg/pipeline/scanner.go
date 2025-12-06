package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// ScannerStage discovers files and creates chunks
type ScannerStage struct {
	config  *ScannerConfig
	output  chan<- *Job
	pool    *WorkerPool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
	stats   StageStats
	strategy chunking.ChunkingStrategy

	// Phase 3.3: Compressed-aware chunker for optimal chunk sizing
	compressedChunker *chunking.CompressedAwareChunker

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

	ctx, cancel := context.WithCancel(context.Background())

	// Create chunking strategy
	chunkingConfig := &chunking.ChunkingConfig{
		Workers:            8,
		AvailableMemory:    4 * 1024 * 1024 * 1024, // 4GB
		GroupingStrategy:   "mixed",
		CostSavingsTarget:  1000,
		EnableFileSplitting: false,                  // Phase 5 Redux: Disable file splitting, use encoder pooling instead
		MaxFileChunkSize:   200 * 1024 * 1024,      // 200MB chunks for split files
	}
	strategy := chunking.NewAdaptiveChunkingStrategy(chunkingConfig)

	// Phase 3.3: Initialize compressed-aware chunker if enabled
	var compressedChunker *chunking.CompressedAwareChunker
	if config.UseCompressedAwareChunking {
		chunker, err := chunking.NewCompressedAwareChunker()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create compressed-aware chunker: %w", err)
		}
		compressedChunker = chunker
	}

	return &ScannerStage{
		config:            config,
		output:            output,
		pool:              NewWorkerPool(ctx, config.Workers),
		ctx:               ctx,
		cancel:            cancel,
		strategy:          strategy,
		compressedChunker: compressedChunker,
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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.run(ctx); err != nil {
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
	s.cancel()
	s.pool.Stop()
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
				if len(batch) > 0 {
					if err := s.processBatch(ctx, batch, totalSize); err != nil {
						return err
					}
				}
				goto done
			}

			batch = append(batch, file)
			totalSize += file.Size

			// Process batch when full
			if len(batch) >= batchSize {
				if err := s.processBatch(ctx, batch, totalSize); err != nil {
					return err
				}
				batch = batch[:0] // Clear batch but keep capacity
			}
		}
	}

done:
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

			// Stream file to channel (no slice accumulation!)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case fileChan <- chunking.File{
				Path:      path,
				Size:      info.Size(),
				ModTime:   info.ModTime(),
				Directory: filepath.Dir(path),
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
	var chunks []chunking.Chunk
	var err error

	// Phase 3.3: Use compressed-aware chunking if enabled
	if s.compressedChunker != nil {
		result, err := s.compressedChunker.CreateChunksWithMetadata(files)
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
			if s.pipeline != nil && s.pipeline.manifestBuilder != nil {
				builder := s.pipeline.manifestBuilder.(*manifest.Builder)
				s.pipeline.manifestMu.Lock()
				for _, file := range chunk.Files {
					builder.AddFile(manifest.FileEntry{
						Path:    file.Path,
						Size:    file.Size,
						ModTime: file.ModTime,
						ChunkID: chunk.ID,
						ShardID: -1,      // Will be determined by archiver
						S3Key:   "",      // Will be filled by uploader
					})
				}
				s.pipeline.manifestMu.Unlock()
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case s.output <- &Job{
				ID:                   chunk.ID,
				Chunk:                chunk,
				TargetCompressedSize: targetCompressedSize,  // Phase 3.3
				EstimatedCompressed:  estimatedCompressed,   // Phase 3.3
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
			len(files),
			4*1024*1024*1024, // 4GB
			1000,             // 1000x cost savings
		)

		chunks, err = s.strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			return fmt.Errorf("failed to group files into chunks: %w", err)
		}

		// Send chunks without target sizes
		for i := range chunks {
			chunk := chunks[i]

			// Track files in manifest (Issue #97)
			if s.pipeline != nil && s.pipeline.manifestBuilder != nil {
				builder := s.pipeline.manifestBuilder.(*manifest.Builder)
				s.pipeline.manifestMu.Lock()
				for _, file := range chunk.Files {
					builder.AddFile(manifest.FileEntry{
						Path:    file.Path,
						Size:    file.Size,
						ModTime: file.ModTime,
						ChunkID: chunk.ID,
						ShardID: -1,      // Will be determined by archiver
						S3Key:   "",      // Will be filled by uploader
					})
				}
				s.pipeline.manifestMu.Unlock()
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

// Error returns any error that occurred during scanning
func (s *ScannerStage) Error() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runError
}
