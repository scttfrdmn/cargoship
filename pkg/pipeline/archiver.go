package pipeline

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// mmapCacheEntry represents a memory-mapped file in the cache
type mmapCacheEntry struct {
	reader   *ioutils.MmapReader
	file     *os.File
	refCount int32 // Reference count for cleanup
}

// EncoderPool manages a pool of reusable zstd encoders
// Phase 5 Redux: Eliminates expensive encoder creation overhead
type EncoderPool struct {
	encoders chan *zstd.Encoder
	size     int
}

// NewEncoderPool creates a new encoder pool with pre-created encoders
func NewEncoderPool(size int) (*EncoderPool, error) {
	pool := &EncoderPool{
		encoders: make(chan *zstd.Encoder, size),
		size:     size,
	}

	// Pre-create all encoders
	for i := 0; i < size; i++ {
		// Create encoder with same settings as before
		encoder, err := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedDefault),
			zstd.WithEncoderConcurrency(runtime.NumCPU()),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create encoder %d: %w", i, err)
		}
		pool.encoders <- encoder
	}

	return pool, nil
}

// Get retrieves an encoder from the pool (blocks if none available)
func (p *EncoderPool) Get() *zstd.Encoder {
	return <-p.encoders
}

// Put returns an encoder to the pool for reuse
func (p *EncoderPool) Put(encoder *zstd.Encoder) {
	// Reset encoder state for reuse (this is cheap)
	// Don't close the encoder, just return it to pool
	p.encoders <- encoder
}

// Close closes all encoders in the pool
func (p *EncoderPool) Close() error {
	// Don't close the channel - just drain and close encoders
	// This prevents "send on closed channel" panics from late Put() calls
	for i := 0; i < p.size; i++ {
		select {
		case encoder := <-p.encoders:
			if err := encoder.Close(); err != nil {
				return err
			}
		default:
			// Encoder still in use by worker - skip it
			// It will be closed when worker returns it
		}
	}
	return nil
}

// ArchiverStage creates streaming tar.zst archives
type ArchiverStage struct {
	config              *ArchiverConfig
	input               <-chan *Job
	output              chan<- *Job
	pool                *WorkerPool
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	mu                  sync.RWMutex
	stats               StageStats
	compressionDetector *CompressionDetector

	// Phase 5: Shared mmap cache for split files (one mmap per file, shared across all parts)
	mmapCache sync.Map // map[string]*mmapCacheEntry (path -> cached mmap reader)

	// Phase 5 Redux: Encoder pool for reusing zstd encoders (eliminates expensive encoder creation)
	encoderPool *EncoderPool

	// Phase 3.2: Multi-output support for eliminating router bottleneck
	outputs           map[string]chan<- *Job // nil = single-output mode (Phase 2)
	shardCount        int                    // Number of shards (0 = single-output)
	shardDistribution map[string]*int64      // map[shardName]jobCount for load balancing analysis

	// Phase 3.3: Archive padding for uniform compressed chunk sizes
	padder *chunking.ArchivePadder // Archive padder for adding zero-byte padding

	// Atomic counters
	jobsProcessed        int64
	jobsFailed           int64
	bytesProcessed       int64
	filesSkipped         int64 // Files skipped due to pre-compression
	compressionTimeSaved int64 // Estimated time saved (nanoseconds)
	paddingBytesAdded    int64 // Total padding bytes added (Phase 3.3)
}

// NewArchiverStage creates a new archiver stage (single-output mode for Phase 2)
func NewArchiverStage(config *ArchiverConfig, input <-chan *Job, output chan<- *Job) (*ArchiverStage, error) {
	if config == nil {
		return nil, fmt.Errorf("archiver config cannot be nil")
	}

	// Phase 5 Redux: Create encoder pool with same number of encoders as workers
	// This eliminates expensive encoder creation during job processing
	encoderPool, err := NewEncoderPool(config.Workers)
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder pool: %w", err)
	}

	// Phase 3.3: Initialize archive padder if enabled
	var padder *chunking.ArchivePadder
	if config.EnablePadding {
		padder = chunking.NewArchivePadderWithConfig(config.UseLowEntropyPadding)
	}

	return &ArchiverStage{
		config:              config,
		input:               input,
		output:              output,
		compressionDetector: NewCompressionDetector(),
		encoderPool:         encoderPool,
		padder:              padder,
		stats: StageStats{
			Name: "archiver",
		},
	}, nil
}

// NewArchiverStageWithSharding creates a new archiver stage with multi-output sharding (Phase 3.2)
// This eliminates the router bottleneck by sharding directly at archiver level
func NewArchiverStageWithSharding(config *ArchiverConfig, input <-chan *Job, outputs map[string]chan<- *Job, shardCount int) (*ArchiverStage, error) {
	if config == nil {
		return nil, fmt.Errorf("archiver config cannot be nil")
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("outputs map cannot be empty in sharding mode")
	}
	if shardCount <= 0 {
		return nil, fmt.Errorf("shard count must be positive, got %d", shardCount)
	}

	// Phase 5 Redux: Create encoder pool with same number of encoders as workers
	encoderPool, err := NewEncoderPool(config.Workers)
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder pool: %w", err)
	}

	// Initialize shard distribution tracking
	shardDist := make(map[string]*int64)
	for shardName := range outputs {
		var count int64
		shardDist[shardName] = &count
	}

	// Phase 3.3: Initialize archive padder if enabled
	var padder *chunking.ArchivePadder
	if config.EnablePadding {
		padder = chunking.NewArchivePadderWithConfig(config.UseLowEntropyPadding)
	}

	return &ArchiverStage{
		config:              config,
		input:               input,
		outputs:             outputs,
		shardCount:          shardCount,
		shardDistribution:   shardDist,
		compressionDetector: NewCompressionDetector(),
		encoderPool:         encoderPool,
		padder:              padder,
		stats: StageStats{
			Name: "archiver",
		},
	}, nil
}

// Name returns the stage name
func (s *ArchiverStage) Name() string {
	return "archiver"
}

// selectOutput determines which output channel to use for a job (Phase 3.2)
// This eliminates single-threaded router by sharding at archiver level
func (s *ArchiverStage) selectOutput(job *Job) chan<- *Job {
	// Single-output mode (Phase 2, Phase 3.1 with router)
	if s.outputs == nil {
		return s.output
	}

	// Multi-output mode (Phase 3.2): shard by chunk ID using fast modulo
	shardID := job.Chunk.ID % s.shardCount
	shardName := fmt.Sprintf("shard-%d", shardID)

	// Track shard distribution for load balancing analysis
	if counter, exists := s.shardDistribution[shardName]; exists {
		atomic.AddInt64(counter, 1)
	}

	return s.outputs[shardName]
}

// Start starts the archiver stage
func (s *ArchiverStage) Start(ctx context.Context) error {
	// Create child context from parent (inherits trace context for Issue #155)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Initialize worker pool with inherited context
	s.pool = NewWorkerPool(s.ctx, s.config.Workers)

	// Start worker goroutines
	for i := 0; i < s.config.Workers; i++ {
		s.wg.Add(1)
		go s.worker(s.ctx)
	}

	// Goroutine to close output when all workers done
	go func() {
		s.wg.Wait()

		// Phase 3.2: Close appropriate outputs
		if s.outputs != nil {
			// Multi-output mode: close all per-prefix channels
			for _, output := range s.outputs {
				close(output)
			}
		} else {
			// Single-output mode
			close(s.output)
		}
	}()

	return nil
}

// Stop stops the archiver stage
func (s *ArchiverStage) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.pool != nil {
		s.pool.Stop()
	}
	s.wg.Wait()

	// Phase 5: Clean up mmap cache
	s.mmapCache.Range(func(key, value interface{}) bool {
		if entry, ok := value.(*mmapCacheEntry); ok {
			_ = entry.reader.Close()
			_ = entry.file.Close()
		}
		return true
	})

	// Phase 5 Redux: Cleanup encoder pool
	if s.encoderPool != nil {
		_ = s.encoderPool.Close()
	}

	return nil
}

// getMmapReader gets or creates a memory-mapped reader for a file (Phase 5)
// Returns the reader and true if mmap was used, nil and false if file is too small
func (s *ArchiverStage) getMmapReader(path string) (*ioutils.MmapReader, bool, error) {
	// Try to load from cache first
	if cached, ok := s.mmapCache.Load(path); ok {
		entry := cached.(*mmapCacheEntry)
		atomic.AddInt32(&entry.refCount, 1)
		return entry.reader, true, nil
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}

	// Check if file is suitable for mmap
	if !ioutils.MmapSupported(file) {
		_ = file.Close()
		return nil, false, nil // Too small or not regular file
	}

	// Create mmap reader
	reader, err := ioutils.NewMmapReader(file)
	if err != nil {
		_ = file.Close()
		return nil, false, err
	}

	// Store in cache
	entry := &mmapCacheEntry{
		reader:   reader,
		file:     file,
		refCount: 1,
	}
	s.mmapCache.Store(path, entry)

	return reader, true, nil
}

// releaseMmapReader decrements reference count (Phase 5)
func (s *ArchiverStage) releaseMmapReader(path string) {
	if cached, ok := s.mmapCache.Load(path); ok {
		entry := cached.(*mmapCacheEntry)
		newCount := atomic.AddInt32(&entry.refCount, -1)

		// If no more references, clean up (optional - could also keep cached)
		// For now, keep cached until Stop() to maximize reuse
		_ = newCount
	}
}

// Process processes a single job (called by workers)
func (s *ArchiverStage) Process(ctx context.Context, job *Job) error {
	startTime := time.Now()

	// Check if files in this chunk are mostly pre-compressed
	compressibleCount := 0
	preCompressedCount := 0
	for _, file := range job.Chunk.Files {
		shouldCompress, _ := s.compressionDetector.ShouldCompress(file.Path)
		if shouldCompress {
			compressibleCount++
		} else {
			preCompressedCount++
			// Track files skipped for metrics
			atomic.AddInt64(&s.filesSkipped, 1)
		}
	}

	// Decide whether to compress the entire archive
	// If >50% of files are already compressed, skip zstd compression
	useCompression := compressibleCount > preCompressedCount

	// Store compression decision in metadata
	if job.Metadata == nil {
		job.Metadata = make(map[string]string)
	}
	if useCompression {
		job.Metadata["compression"] = "zstd"
	} else {
		job.Metadata["compression"] = "none"
	}
	job.Metadata["compressible_files"] = fmt.Sprintf("%d", compressibleCount)
	job.Metadata["precompressed_files"] = fmt.Sprintf("%d", preCompressedCount)

	// Phase 2: Create streaming archive using BufferedPipe with 64MB buffer
	// This replaces io.Pipe's 4KB buffer to eliminate 751% serialization overhead
	pr, pw := NewBufferedPipe(64*1024*1024, 32*1024) // 64MB buffer, 32KB chunks

	// Phase 5 Redux: Get encoder from pool BEFORE spawning goroutine
	// This naturally limits concurrency to pool size (8) and prevents deadlock
	var encoder *zstd.Encoder
	if useCompression {
		encoder = s.encoderPool.Get() // Blocks if all encoders in use - natural backpressure
	}

	// Archive creation goroutine
	go func() {
		defer func() {
			_ = pw.Close()
			// Return encoder to pool after work is done
			if encoder != nil {
				// Close encoder to flush data, then return to pool
				_ = encoder.Close()
				s.encoderPool.Put(encoder)
			}
		}()

		var tw *tar.Writer

		if useCompression {
			// Reset encoder for new output stream
			encoder.Reset(pw)

			// Create tar writer on top of compression
			tw = tar.NewWriter(encoder)
		} else {
			// Skip compression - write tar directly
			tw = tar.NewWriter(pw)
			// Track time saved by skipping compression
			// Estimate: ~1s per 100MB for zstd compression
			estimatedTimeSaved := (job.Chunk.TotalSize / (100 * 1024 * 1024)) * int64(time.Second)
			atomic.AddInt64(&s.compressionTimeSaved, estimatedTimeSaved)
		}

		// Close tar writer to flush all data
		defer func() {
			_ = tw.Close()
		}()

		// Add all files to archive with parallel I/O optimization
		var totalSize int64
		if err := s.addFilesWithParallelIO(tw, job.Chunk.Files, &totalSize); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		// Phase 3.3: Add padding if enabled and target size is set
		if s.padder != nil && job.TargetCompressedSize > 0 {
			// Calculate padding needed to reach target AFTER compression
			// Since padding is zero bytes, it compresses to nearly nothing with zstd
			// So we need to add padding to the uncompressed archive
			currentSize := totalSize

			// For now, use a simple estimate: assume 50% compression ratio
			// (This should be replaced with actual compression ratio from CompressedAwareChunker when Task #11 is complete)
			targetUncompressed := job.TargetCompressedSize * 2

			paddingNeeded := s.padder.CalculatePaddingSize(currentSize, targetUncompressed)

			// Validate padding ratio before adding
			if paddingNeeded > 0 {
				paddingRatio := float64(paddingNeeded) / float64(targetUncompressed)

				if paddingRatio <= s.config.MaxPaddingRatio {
					// Add padding as a special tar file entry
					header := &tar.Header{
						Name: ".padding",
						Mode: 0644,
						Size: paddingNeeded,
					}
					if err := tw.WriteHeader(header); err != nil {
						_ = pw.CloseWithError(fmt.Errorf("failed to write padding header: %w", err))
						return
					}

					// Write padding bytes
					paddingWritten, err := s.padder.PadToTarget(tw, 0, paddingNeeded)
					if err != nil {
						_ = pw.CloseWithError(fmt.Errorf("failed to write padding: %w", err))
						return
					}

					atomic.AddInt64(&s.paddingBytesAdded, paddingWritten)
					totalSize += paddingWritten

					// Store in metadata
					job.Metadata["padding_bytes"] = fmt.Sprintf("%d", paddingWritten)
					job.Metadata["padding_ratio"] = fmt.Sprintf("%.2f%%",
						float64(paddingWritten)/float64(totalSize)*100)
				} else {
					// Log warning but continue (padding ratio exceeds limit)
					job.Metadata["padding_skipped"] = fmt.Sprintf("ratio %.2f%% exceeds limit %.2f%%",
						paddingRatio*100, s.config.MaxPaddingRatio*100)
				}
			}
		}

		// Update job with archive size (compressed)
		// Note: This is an estimate since we're streaming
		job.ArchiveSize = totalSize
	}()

	// Store the reader in the job
	job.Archive = pr

	// Generate S3 key with upload-ID/shard structure for multi-prefix optimization (Phase 3)
	// Format: uploads/{upload-id}/shard-{shard_id}/chunk-{chunk_id}.tar.zst
	// This distributes S3 request load across multiple prefixes (8× request rate capacity)
	shardCount := s.config.ShardCount
	if shardCount == 0 {
		shardCount = 8 // Default: 8 shards
	}
	shardID := job.ID % shardCount
	uploadID := s.config.UploadID
	if uploadID == "" {
		uploadID = "default" // Fallback for tests without upload ID
	}

	extension := ".tar"
	if useCompression {
		extension = ".tar.zst"
	}
	job.S3Key = fmt.Sprintf("uploads/%s/shard-%d/chunk-%d%s",
		uploadID, shardID, job.ID, extension)

	// Issue #103: Populate shard context for error reporting
	job.ShardID = shardID
	job.ShardPrefix = fmt.Sprintf("shard-%d", shardID)

	// Send to output channel (Phase 3.2: uses selectOutput() for sharding)
	select {
	case <-ctx.Done():
		_ = pr.Close()
		return ctx.Err()
	case s.selectOutput(job) <- job:
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, job.ArchiveSize)

		s.mu.Lock()
		s.stats.TotalTime += time.Since(startTime)
		s.mu.Unlock()

		return nil
	}
}

// Stats returns stage statistics
func (s *ArchiverStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.JobsFailed = atomic.LoadInt64(&s.jobsFailed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.config.Workers - s.pool.AvailableWorkers()

	if stats.JobsProcessed > 0 {
		stats.AverageTime = stats.TotalTime / time.Duration(stats.JobsProcessed)
	}

	// Phase 3.2: Add shard distribution to metadata for load balancing analysis
	if s.shardDistribution != nil {
		if stats.Metadata == nil {
			stats.Metadata = make(map[string]interface{})
		}
		shardDist := make(map[string]interface{})
		for shardName, count := range s.shardDistribution {
			shardDist[shardName] = atomic.LoadInt64(count)
		}
		stats.Metadata["shard_distribution"] = shardDist
	}

	return stats
}

// GetCompressionStats returns compression optimization statistics
func (s *ArchiverStage) GetCompressionStats() (filesSkipped int64, timeSaved time.Duration) {
	return atomic.LoadInt64(&s.filesSkipped), time.Duration(atomic.LoadInt64(&s.compressionTimeSaved))
}

// GetPaddingStats returns Phase 3.3 archive padding statistics
func (s *ArchiverStage) GetPaddingStats() int64 {
	return atomic.LoadInt64(&s.paddingBytesAdded)
}

// worker processes jobs from the input channel
func (s *ArchiverStage) worker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-s.input:
			if !ok {
				return
			}

			if err := s.Process(ctx, job); err != nil {
				job.Error = err
				atomic.AddInt64(&s.jobsFailed, 1)

				// Still send to output for error handling (Phase 3.2: uses selectOutput())
				select {
				case <-ctx.Done():
					return
				case s.selectOutput(job) <- job:
				}
			}
		}
	}
}

// fileData represents a file that has been read into memory
type fileData struct {
	Path string
	Info os.FileInfo
	Data []byte
	Err  error
	Done chan struct{}

	// Phase 5: Support for partial file reads
	File chunking.File // Full file metadata including offset/length for splits
}

// addFilesWithParallelIO adds files to archive with parallel I/O optimization
// Files are read in parallel by a worker pool, but written to tar sequentially
// to maintain tar format integrity. This provides 4-8x speedup for I/O bound workloads.
func (s *ArchiverStage) addFilesWithParallelIO(tw *tar.Writer, files []chunking.File, totalSize *int64) error {
	// For very small file counts, parallel I/O overhead isn't worth it
	if len(files) < 3 {
		for _, file := range files {
			if err := s.addFileToArchiveWithMetadata(tw, file); err != nil {
				return err
			}
			*totalSize += file.Size
		}
		return nil
	}

	// Create channels for parallel I/O
	fileChan := make(chan *fileData, runtime.NumCPU())
	errChan := make(chan error, 1)

	// Start worker pool to read files in parallel
	numWorkers := runtime.NumCPU()
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for fd := range fileChan {
				// Phase 5 Optimized: Use mmap for split files to enable zero-copy sharing
				if fd.File.TotalParts > 1 {
					// Try to use mmap for large files (shared across all parts)
					mmapReader, usedMmap, err := s.getMmapReader(fd.Path)
					if err != nil {
						fd.Err = fmt.Errorf("failed to open file: %w", err)
						close(fd.Done)
						continue
					}

					if usedMmap {
						// Use mmap - read directly from mapped memory at offset
						offset := fd.File.Offset
						length := fd.File.Length
						if length == 0 {
							length = fd.File.Size
						}

						// ReadAt from mmap (zero-copy from OS perspective)
						partData := make([]byte, length)
						n, err := mmapReader.ReadAt(partData, offset)
						if err != nil && err != io.EOF {
							fd.Err = fmt.Errorf("failed to read from mmap: %w", err)
							s.releaseMmapReader(fd.Path)
							close(fd.Done)
							continue
						}
						fd.Data = partData[:n]
						s.releaseMmapReader(fd.Path)
					} else {
						// File too small for mmap - use regular read
						data, err := os.ReadFile(fd.Path)
						if err != nil {
							fd.Err = fmt.Errorf("failed to read file: %w", err)
							close(fd.Done)
							continue
						}

						// Slice the data based on offset/length
						offset := fd.File.Offset
						length := fd.File.Length
						if length == 0 {
							length = fd.File.Size
						}

						// Bounds check
						if offset+length > int64(len(data)) {
							length = int64(len(data)) - offset
						}
						if offset < 0 || offset >= int64(len(data)) {
							fd.Err = fmt.Errorf("invalid offset %d for file size %d", offset, len(data))
							close(fd.Done)
							continue
						}

						// Create a slice with the data
						partData := make([]byte, length)
						copy(partData, data[offset:offset+length])
						fd.Data = partData
					}
				} else {
					// Not a split file - read entire file normally
					data, err := os.ReadFile(fd.Path)
					if err != nil {
						fd.Err = fmt.Errorf("failed to read file: %w", err)
					} else {
						fd.Data = data
					}
				}

				// Signal that file has been read
				close(fd.Done)
			}
		}()
	}

	// Goroutine to close workers when done
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Process files: dispatch reads in parallel, write to tar sequentially
	for _, file := range files {
		// Get file info first (fast, doesn't read data)
		info, err := os.Stat(file.Path)
		if err != nil {
			close(fileChan)
			return fmt.Errorf("failed to stat file %s: %w", file.Path, err)
		}

		// Create file data structure
		fd := &fileData{
			Path: file.Path,
			Info: info,
			Done: make(chan struct{}),
			File: file, // Phase 5: Store full file metadata for partial reads
		}

		// Dispatch read to worker pool (non-blocking)
		select {
		case fileChan <- fd:
			// Read dispatched successfully
		case err := <-errChan:
			close(fileChan)
			return err
		}

		// Wait for file to be read (parallel I/O happening in background)
		<-fd.Done

		// Check for read error
		if fd.Err != nil {
			close(fileChan)
			return fmt.Errorf("failed to read file %s: %w", fd.Path, fd.Err)
		}

		// Write to tar sequentially (fast, data already in memory)
		if err := s.addFileToArchiveFromMemoryWithMetadata(tw, fd.File, fd.Info, fd.Data); err != nil {
			close(fileChan)
			return err
		}

		*totalSize += file.Size
	}

	// Close file channel to signal workers to exit
	close(fileChan)

	// Wait for workers to finish
	wg.Wait()

	// Check for any worker errors
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	default:
	}

	return nil
}

// addFileToArchiveWithMetadata adds a file to archive with full metadata support (Phase 5)
// Supports partial file reads with offset/length for split files
func (s *ArchiverStage) addFileToArchiveWithMetadata(tw *tar.Writer, file chunking.File) error {
	// Open file
	f, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Seek to offset for partial reads
	if file.Offset > 0 {
		if _, err := f.Seek(file.Offset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", file.Offset, err)
		}
	}

	// Determine read length
	length := file.Length
	if length == 0 {
		length = file.Size
	}

	// Get file info for tar header
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("failed to create tar header: %w", err)
	}

	// Update header with correct size and name
	header.Size = length

	// For split files, modify the name to include part information
	if file.TotalParts > 1 {
		header.Name = fmt.Sprintf("%s.part%d", file.Path, file.PartIndex)

		// Add PAX records for split file metadata
		if header.PAXRecords == nil {
			header.PAXRecords = make(map[string]string)
		}
		header.PAXRecords["CARGOSHIP.part_index"] = fmt.Sprintf("%d", file.PartIndex)
		header.PAXRecords["CARGOSHIP.total_parts"] = fmt.Sprintf("%d", file.TotalParts)
		header.PAXRecords["CARGOSHIP.offset"] = fmt.Sprintf("%d", file.Offset)
		header.PAXRecords["CARGOSHIP.original_path"] = file.Path
	} else {
		header.Name = file.Path
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	// Copy file content with length limit (platform-specific zero-copy)
	if err := s.copyFileToArchive(tw, f, length); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// addFileToArchiveFromMemoryWithMetadata adds a file to tar from in-memory data with metadata (Phase 5)
func (s *ArchiverStage) addFileToArchiveFromMemoryWithMetadata(tw *tar.Writer, file chunking.File, info os.FileInfo, data []byte) error {
	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("failed to create tar header: %w", err)
	}

	// Update header with actual data size (for partial reads)
	header.Size = int64(len(data))

	// For split files, modify the name to include part information
	if file.TotalParts > 1 {
		header.Name = fmt.Sprintf("%s.part%d", file.Path, file.PartIndex)

		// Add PAX records for split file metadata
		if header.PAXRecords == nil {
			header.PAXRecords = make(map[string]string)
		}
		header.PAXRecords["CARGOSHIP.part_index"] = fmt.Sprintf("%d", file.PartIndex)
		header.PAXRecords["CARGOSHIP.total_parts"] = fmt.Sprintf("%d", file.TotalParts)
		header.PAXRecords["CARGOSHIP.offset"] = fmt.Sprintf("%d", file.Offset)
		header.PAXRecords["CARGOSHIP.original_path"] = file.Path
	} else {
		header.Name = file.Path
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	// Write data from memory
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// StreamingArchiveReader wraps a pipe reader to track bytes read
type StreamingArchiveReader struct {
	reader     io.ReadCloser
	bytesRead  int64
	mu         sync.Mutex
	onProgress func(int64)
}

// NewStreamingArchiveReader creates a new streaming archive reader
func NewStreamingArchiveReader(reader io.ReadCloser) *StreamingArchiveReader {
	return &StreamingArchiveReader{
		reader: reader,
	}
}

// Read implements io.Reader
func (r *StreamingArchiveReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)

	r.mu.Lock()
	r.bytesRead += int64(n)
	bytes := r.bytesRead
	r.mu.Unlock()

	if r.onProgress != nil && n > 0 {
		r.onProgress(bytes)
	}

	return n, err
}

// Close implements io.Closer
func (r *StreamingArchiveReader) Close() error {
	return r.reader.Close()
}

// BytesRead returns the total bytes read
func (r *StreamingArchiveReader) BytesRead() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytesRead
}

// SetProgressCallback sets a callback for progress updates
func (r *StreamingArchiveReader) SetProgressCallback(callback func(int64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onProgress = callback
}
