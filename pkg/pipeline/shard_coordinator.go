// Package pipeline provides streaming pipeline for CargoShip
package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// ShardCoordinatorConfig configures the shard coordinator
type ShardCoordinatorConfig struct {
	// Shard configuration
	ShardCount        int  // Number of shards (0 = auto-calculate based on workload size)
	EstimatedDataSize int64 // Estimated total data size for intelligent shard count (optional)
	Bucket            string // Target S3 bucket
	Prefix            string // S3 key prefix (optional)

	// Routing strategy
	Router *chunking.ShardRouter // File-to-shard router

	// S3 configuration
	S3Client S3Uploader // AWS S3 client interface

	// Memory management
	MemoryManager *MemoryManager // Global memory manager

	// Pipeline configuration (applied to all shards)
	CompressionLevel zstd.EncoderLevel // Zstd compression level
	MaxRetries       int               // Maximum upload retry attempts
	RetryDelay       time.Duration     // Delay between retries
}

// ShardCoordinator orchestrates multiple shard pipelines for parallel upload
// This enables 10x throughput improvement by uploading N shards concurrently
type ShardCoordinator struct {
	config    *ShardCoordinatorConfig
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	startOnce sync.Once
	closeOnce sync.Once

	// Shard pipelines
	pipelines []*ShardPipeline

	// Coordination
	wg        sync.WaitGroup
	errors    []error
	errorsMu  sync.Mutex
	startTime time.Time
	endTime   time.Time

	// Statistics
	filesAdded     int64 // Total files added across all shards
	bytesProcessed int64 // Total bytes processed across all shards

	// Memory management (Issue #83)
	ownsMemoryManager bool // True if we created the MemoryManager and should stop it
}

// CalculateIntelligentShardCount determines optimal shard count based on workload size
// Returns recommended shard count based on:
//   - Small workload (<1 GB): 4 shards (saves memory, still good parallelism)
//   - Medium workload (1-10 GB): 8 shards (hits S3 multi-prefix sweet spot)
//   - Large workload (>10 GB): 10 shards (maximum throughput)
func CalculateIntelligentShardCount(estimatedDataSize int64) int {
	const (
		smallWorkloadThreshold  = 1 * 1024 * 1024 * 1024  // 1 GB
		mediumWorkloadThreshold = 10 * 1024 * 1024 * 1024 // 10 GB

		smallShardCount  = 4  // For <1GB
		mediumShardCount = 8  // For 1-10GB
		largeShardCount  = 10 // For >10GB
	)

	if estimatedDataSize < smallWorkloadThreshold {
		return smallShardCount
	} else if estimatedDataSize <= mediumWorkloadThreshold {
		return mediumShardCount
	} else {
		return largeShardCount
	}
}

// NewShardCoordinator creates a new shard coordinator
func NewShardCoordinator(ctx context.Context, config *ShardCoordinatorConfig) (*ShardCoordinator, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.Router == nil {
		return nil, fmt.Errorf("router cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}

	// Set defaults
	if config.ShardCount <= 0 {
		if config.EstimatedDataSize > 0 {
			// Use intelligent shard count based on workload size
			config.ShardCount = CalculateIntelligentShardCount(config.EstimatedDataSize)
		} else {
			// Fall back to default (medium workload assumption)
			config.ShardCount = 8
		}
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	sc := &ShardCoordinator{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// Create default MemoryManager if not provided (Issue #83)
	if config.MemoryManager == nil {
		// Calculate part size for memory estimation (default 50MB per shard)
		partSize := int64(50 << 20)
		// Total memory budget: shard_count × 4 × partSize
		// Use 50% of available memory by default, with proactive GC for large chunks
		memConfig := &MemoryManagerConfig{
			MemoryBudgetPercent:  0.5,                 // Use 50% of available memory
			MinMemoryBuffer:      512 << 20,           // Keep 512MB free
			ProactiveGCThreshold: 50 << 20,            // Proactive GC for >50MB chunks
			PartSize:             partSize,            // Part size for estimation
			MonitorInterval:      time.Second,         // Monitor every second
		}
		config.MemoryManager = NewMemoryManager(ctx, memConfig)
		sc.ownsMemoryManager = true
	}

	// Calculate intelligent compression concurrency per shard (Issue #80)
	// Distribute CPU cores across shards for optimal compression parallelism
	cpuCores := runtime.GOMAXPROCS(0)
	compressionConcurrency := cpuCores / config.ShardCount
	if compressionConcurrency < 1 {
		compressionConcurrency = 1 // Minimum 1 thread per shard
	}

	// Create shard pipelines
	sc.pipelines = make([]*ShardPipeline, config.ShardCount)
	for i := 0; i < config.ShardCount; i++ {
		shardName := fmt.Sprintf("shard-%05d", i)
		pipeConfig := &ShardPipelineConfig{
			ShardID:                i,
			ShardName:              shardName,
			S3Client:               config.S3Client,
			Bucket:                 config.Bucket,
			Prefix:                 config.Prefix,
			CompressionLevel:       config.CompressionLevel,
			CompressionConcurrency: compressionConcurrency, // Issue #80
			MemoryManager:          config.MemoryManager,
			MaxRetries:             config.MaxRetries,
			RetryDelay:             config.RetryDelay,
		}

		pipeline, err := NewShardPipeline(ctx, pipeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create pipeline for shard %d: %w", i, err)
		}
		sc.pipelines[i] = pipeline
	}

	return sc, nil
}

// Start starts all shard pipelines in parallel
func (sc *ShardCoordinator) Start() error {
	var startErr error
	sc.startOnce.Do(func() {
		sc.startTime = time.Now()

		// Start all pipelines
		for i, pipeline := range sc.pipelines {
			if err := pipeline.Start(); err != nil {
				startErr = fmt.Errorf("failed to start pipeline for shard %d: %w", i, err)
				return
			}
		}
	})
	return startErr
}

// AddFile routes a file to the appropriate shard and queues it for upload
func (sc *ShardCoordinator) AddFile(file chunking.File) error {
	// Route file to shard
	shardID := sc.config.Router.Route(file)
	if shardID < 0 || shardID >= len(sc.pipelines) {
		return fmt.Errorf("invalid shard ID %d (must be 0-%d)", shardID, len(sc.pipelines)-1)
	}

	// Add to shard pipeline
	if err := sc.pipelines[shardID].AddFile(file); err != nil {
		return fmt.Errorf("failed to add file to shard %d: %w", shardID, err)
	}

	// Update statistics
	atomic.AddInt64(&sc.filesAdded, 1)
	atomic.AddInt64(&sc.bytesProcessed, file.Size)

	return nil
}

// AddFiles adds multiple files in batch
func (sc *ShardCoordinator) AddFiles(files []chunking.File) error {
	for _, file := range files {
		if err := sc.AddFile(file); err != nil {
			return err
		}
	}
	return nil
}

// Close closes all shard pipelines and waits for upload completion
func (sc *ShardCoordinator) Close() error {
	var closeErr error
	sc.closeOnce.Do(func() {
		sc.endTime = time.Now()

		// Close all pipelines concurrently
		sc.wg.Add(len(sc.pipelines))
		for i, pipeline := range sc.pipelines {
			go func(idx int, p *ShardPipeline) {
				defer sc.wg.Done()
				if err := p.Close(); err != nil {
					sc.addError(fmt.Errorf("shard %d close error: %w", idx, err))
				}
			}(i, pipeline)
		}

		// Wait for all pipelines to close
		sc.wg.Wait()

		// Stop memory manager if we own it (Issue #83)
		if sc.ownsMemoryManager && sc.config.MemoryManager != nil {
			sc.config.MemoryManager.Stop()
		}

		// Aggregate errors
		sc.errorsMu.Lock()
		if len(sc.errors) > 0 {
			closeErr = fmt.Errorf("coordinator close failed with %d errors: %v", len(sc.errors), sc.errors[0])
		}
		sc.errorsMu.Unlock()
	})
	return closeErr
}

// addError adds an error to the error list (thread-safe)
func (sc *ShardCoordinator) addError(err error) {
	sc.errorsMu.Lock()
	defer sc.errorsMu.Unlock()
	sc.errors = append(sc.errors, err)
}

// GetStats returns aggregated statistics across all shards
func (sc *ShardCoordinator) GetStats() ShardCoordinatorStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	duration := time.Since(sc.startTime)
	if !sc.endTime.IsZero() {
		duration = sc.endTime.Sub(sc.startTime)
	}

	// Aggregate per-shard statistics
	shardStats := make([]ShardPipelineStats, len(sc.pipelines))
	var totalUploadSize int64
	var completedShards int
	var failedShards int

	for i, pipeline := range sc.pipelines {
		stats := pipeline.GetStats()
		shardStats[i] = stats
		totalUploadSize += stats.UploadSize

		if stats.Completed {
			completedShards++
		}
		if stats.Error != nil {
			failedShards++
		}
	}

	sc.errorsMu.Lock()
	errorCount := len(sc.errors)
	var firstError error
	if errorCount > 0 {
		firstError = sc.errors[0]
	}
	sc.errorsMu.Unlock()

	// Get memory manager stats if available (Issue #83)
	var memStats *MemoryManagerStats
	if sc.config.MemoryManager != nil {
		stats := sc.config.MemoryManager.GetStats()
		memStats = &stats
	}

	return ShardCoordinatorStats{
		ShardCount:       sc.config.ShardCount,
		FilesAdded:       atomic.LoadInt64(&sc.filesAdded),
		BytesProcessed:   atomic.LoadInt64(&sc.bytesProcessed),
		TotalUploadSize:  totalUploadSize,
		Duration:         duration,
		CompletedShards:  completedShards,
		FailedShards:     failedShards,
		ShardStats:       shardStats,
		ErrorCount:       errorCount,
		FirstError:       firstError,
		MemoryStats:      memStats,
	}
}

// GetShardStats returns statistics for a specific shard
func (sc *ShardCoordinator) GetShardStats(shardID int) (ShardPipelineStats, error) {
	if shardID < 0 || shardID >= len(sc.pipelines) {
		return ShardPipelineStats{}, fmt.Errorf("invalid shard ID %d (must be 0-%d)", shardID, len(sc.pipelines)-1)
	}
	return sc.pipelines[shardID].GetStats(), nil
}

// ShardCoordinatorStats contains aggregated statistics across all shards
type ShardCoordinatorStats struct {
	ShardCount      int                   // Total number of shards
	FilesAdded      int64                 // Total files added across all shards
	BytesProcessed  int64                 // Total bytes processed (uncompressed)
	TotalUploadSize int64                 // Total uploaded size (compressed)
	Duration        time.Duration         // Total processing time
	CompletedShards int                   // Number of completed shards
	FailedShards    int                   // Number of failed shards
	ShardStats      []ShardPipelineStats  // Per-shard statistics
	ErrorCount      int                   // Total number of errors
	FirstError      error                 // First error encountered (if any)
	MemoryStats     *MemoryManagerStats   // Memory manager statistics (Issue #83)
}

// String returns a formatted string representation of coordinator stats
func (s ShardCoordinatorStats) String() string {
	status := "in-progress"
	if s.CompletedShards == s.ShardCount {
		if s.FailedShards > 0 {
			status = fmt.Sprintf("completed with %d failures", s.FailedShards)
		} else {
			status = "completed"
		}
	} else if s.FailedShards > 0 {
		status = fmt.Sprintf("in-progress (%d failed)", s.FailedShards)
	}

	compressionRatio := 0.0
	if s.BytesProcessed > 0 && s.TotalUploadSize > 0 {
		compressionRatio = float64(s.TotalUploadSize) / float64(s.BytesProcessed)
	}

	processingThroughput := s.ThroughputMBps()
	networkThroughput := s.NetworkThroughputMBps()

	baseStats := fmt.Sprintf("Coordinator: %d shards, %d files, %d MB → %d MB (%.1f%% compression)\nThroughput: %.2f MB/s processing | %.2f MB/s network\nDuration: %s, %s",
		s.ShardCount,
		s.FilesAdded,
		s.BytesProcessed/(1<<20),
		s.TotalUploadSize/(1<<20),
		(1-compressionRatio)*100,
		processingThroughput,
		networkThroughput,
		s.Duration.Round(time.Millisecond),
		status,
	)

	// Include memory stats if available (Issue #83)
	if s.MemoryStats != nil {
		return fmt.Sprintf("%s\n%s", baseStats, s.MemoryStats.String())
	}

	return baseStats
}

// CompressionRatio returns the compression ratio (0-1, where 0 = perfect compression, 1 = no compression)
func (s ShardCoordinatorStats) CompressionRatio() float64 {
	if s.BytesProcessed == 0 || s.TotalUploadSize == 0 {
		return 0.0
	}
	return float64(s.TotalUploadSize) / float64(s.BytesProcessed)
}

// ThroughputMBps returns the processing throughput in MB/s (uncompressed data rate)
func (s ShardCoordinatorStats) ThroughputMBps() float64 {
	if s.Duration.Seconds() == 0 {
		return 0.0
	}
	return float64(s.BytesProcessed) / (1 << 20) / s.Duration.Seconds()
}

// NetworkThroughputMBps returns the network throughput in MB/s (compressed data uploaded to S3)
func (s ShardCoordinatorStats) NetworkThroughputMBps() float64 {
	if s.Duration.Seconds() == 0 {
		return 0.0
	}
	return float64(s.TotalUploadSize) / (1 << 20) / s.Duration.Seconds()
}

// IsComplete returns true if all shards have completed
func (s ShardCoordinatorStats) IsComplete() bool {
	return s.CompletedShards == s.ShardCount
}

// HasErrors returns true if any errors were encountered
func (s ShardCoordinatorStats) HasErrors() bool {
	return s.ErrorCount > 0 || s.FailedShards > 0
}
