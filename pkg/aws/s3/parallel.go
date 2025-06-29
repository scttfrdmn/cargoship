// Package s3 provides parallel prefix optimization for CargoShip uploads
package s3

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"
)

// ParallelUploader manages parallel uploads across multiple S3 prefixes
type ParallelUploader struct {
	transporter *Transporter
	config      ParallelConfig
	metrics     *UploadMetrics
}

// ParallelConfig configures parallel upload behavior
type ParallelConfig struct {
	// MaxPrefixes is the maximum number of parallel prefixes to use
	MaxPrefixes int
	
	// PrefixPattern defines how prefixes are generated
	PrefixPattern string // "date", "hash", "sequential", "custom"
	
	// CustomPrefixes allows manual prefix specification
	CustomPrefixes []string
	
	// MaxConcurrentUploads per prefix
	MaxConcurrentUploads int
	
	// LoadBalancing strategy for distributing uploads
	LoadBalancing string // "round_robin", "least_loaded", "hash_based"
	
	// PrefixOptimization enables automatic prefix optimization
	PrefixOptimization bool
}

// UploadMetrics tracks performance across prefixes
type UploadMetrics struct {
	PrefixStats    map[string]*PrefixMetrics
	TotalUploaded  int64
	TotalErrors    int
	StartTime      time.Time
	mutex          sync.RWMutex
}

// PrefixMetrics tracks per-prefix performance
type PrefixMetrics struct {
	UploadCount   int64
	TotalBytes    int64
	ErrorCount    int
	LastUpload    time.Time
	AvgThroughput float64 // MB/s
	ActiveUploads int
}

// PrefixBatch groups archives for parallel upload
type PrefixBatch struct {
	Prefix   string
	Archives []Archive
	Priority int // Higher priority uploads first
}

// NewParallelUploader creates a new parallel uploader
func NewParallelUploader(transporter *Transporter, config ParallelConfig) *ParallelUploader {
	if config.MaxPrefixes <= 0 {
		config.MaxPrefixes = 4 // Default to 4 parallel prefixes
	}
	if config.MaxConcurrentUploads <= 0 {
		config.MaxConcurrentUploads = 3 // Default per-prefix concurrency
	}
	if config.PrefixPattern == "" {
		config.PrefixPattern = "hash" // Default to hash-based distribution
	}
	if config.LoadBalancing == "" {
		config.LoadBalancing = "least_loaded"
	}

	return &ParallelUploader{
		transporter: transporter,
		config:      config,
		metrics: &UploadMetrics{
			PrefixStats: make(map[string]*PrefixMetrics),
			StartTime:   time.Now(),
		},
	}
}

// UploadParallel uploads archives in parallel across multiple prefixes
func (p *ParallelUploader) UploadParallel(ctx context.Context, archives []Archive) (*ParallelUploadResult, error) {
	if len(archives) == 0 {
		return &ParallelUploadResult{}, nil
	}

	slog.Info("starting parallel upload", "archives", len(archives), "max_prefixes", p.config.MaxPrefixes)

	// Generate prefixes for parallel upload
	prefixes := p.generatePrefixes(len(archives))
	
	// Distribute archives across prefixes
	batches := p.distributeArchives(archives, prefixes)
	
	// Start parallel upload workers
	return p.executeParallelUpload(ctx, batches)
}

// generatePrefixes creates S3 prefixes for parallel upload
func (p *ParallelUploader) generatePrefixes(archiveCount int) []string {
	switch p.config.PrefixPattern {
	case "custom":
		if len(p.config.CustomPrefixes) > 0 {
			return p.config.CustomPrefixes[:min(len(p.config.CustomPrefixes), p.config.MaxPrefixes)]
		}
		fallthrough
	case "date":
		return p.generateDatePrefixes()
	case "sequential":
		return p.generateSequentialPrefixes()
	case "hash":
		fallthrough
	default:
		return p.generateHashPrefixes()
	}
}

// generateDatePrefixes creates date-based prefixes for better organization
func (p *ParallelUploader) generateDatePrefixes() []string {
	now := time.Now()
	prefixes := make([]string, p.config.MaxPrefixes)
	
	for i := 0; i < p.config.MaxPrefixes; i++ {
		// Create hourly prefixes for high parallelism
		hour := now.Add(time.Duration(i) * time.Hour)
		prefixes[i] = fmt.Sprintf("archives/%s/batch-%02d/",
			hour.Format("2006/01/02"), i)
	}
	
	return prefixes
}

// generateSequentialPrefixes creates simple sequential prefixes
func (p *ParallelUploader) generateSequentialPrefixes() []string {
	prefixes := make([]string, p.config.MaxPrefixes)
	
	for i := 0; i < p.config.MaxPrefixes; i++ {
		prefixes[i] = fmt.Sprintf("archives/batch-%04d/", i)
	}
	
	return prefixes
}

// generateHashPrefixes creates hash-based prefixes for optimal S3 performance
func (p *ParallelUploader) generateHashPrefixes() []string {
	prefixes := make([]string, p.config.MaxPrefixes)
	
	// Use hex characters for better S3 performance (avoids hotspotting)
	hexChars := "0123456789abcdef"
	
	for i := 0; i < p.config.MaxPrefixes; i++ {
		// Create diverse prefix distribution
		char1 := hexChars[i%16]
		char2 := hexChars[(i/16)%16]
		prefixes[i] = fmt.Sprintf("archives/%c%c/", char1, char2)
	}
	
	return prefixes
}

// distributeArchives assigns archives to prefixes using load balancing
func (p *ParallelUploader) distributeArchives(archives []Archive, prefixes []string) []PrefixBatch {
	batches := make([]PrefixBatch, len(prefixes))
	
	// Initialize batches
	for i, prefix := range prefixes {
		batches[i] = PrefixBatch{
			Prefix:   prefix,
			Archives: make([]Archive, 0),
			Priority: 0,
		}
	}
	
	// Distribute archives based on load balancing strategy
	switch p.config.LoadBalancing {
	case "round_robin":
		for i, archive := range archives {
			batchIdx := i % len(batches)
			batches[batchIdx].Archives = append(batches[batchIdx].Archives, archive)
		}
		
	case "hash_based":
		for _, archive := range archives {
			hash := p.hashArchiveKey(archive.Key)
			batchIdx := int(hash) % len(batches)
			batches[batchIdx].Archives = append(batches[batchIdx].Archives, archive)
		}
		
	case "least_loaded":
		fallthrough
	default:
		// Distribute by size to balance load
		p.distributeBySize(archives, batches)
	}
	
	// Set priorities based on batch size (larger batches get higher priority)
	for i := range batches {
		totalSize := int64(0)
		for _, archive := range batches[i].Archives {
			totalSize += archive.Size
		}
		batches[i].Priority = int(totalSize / (1024 * 1024)) // Priority based on MB
	}
	
	return batches
}

// distributeBySize distributes archives to balance total size across batches
func (p *ParallelUploader) distributeBySize(archives []Archive, batches []PrefixBatch) {
	// Track cumulative size per batch
	batchSizes := make([]int64, len(batches))
	
	for _, archive := range archives {
		// Find batch with smallest total size
		minIdx := 0
		minSize := batchSizes[0]
		
		for i, size := range batchSizes {
			if size < minSize {
				minSize = size
				minIdx = i
			}
		}
		
		// Add archive to least loaded batch
		batches[minIdx].Archives = append(batches[minIdx].Archives, archive)
		batchSizes[minIdx] += archive.Size
	}
}

// hashArchiveKey creates a hash for consistent prefix assignment
func (p *ParallelUploader) hashArchiveKey(key string) uint32 {
	hash := fnv.New32a()
	hash.Write([]byte(key))
	return hash.Sum32()
}

// executeParallelUpload performs the actual parallel upload
func (p *ParallelUploader) executeParallelUpload(ctx context.Context, batches []PrefixBatch) (*ParallelUploadResult, error) {
	result := &ParallelUploadResult{
		Results:     make([]*UploadResult, 0),
		PrefixStats: make(map[string]*PrefixMetrics),
		StartTime:   time.Now(),
	}
	
	// Create worker pool for prefix-level parallelism
	prefixPool := pool.New().WithErrors().WithContext(ctx)
	
	var resultMutex sync.Mutex
	
	// Launch upload workers for each prefix batch
	for _, batch := range batches {
		batch := batch // Capture for closure
		
		if len(batch.Archives) == 0 {
			continue // Skip empty batches
		}
		
		slog.Info("starting prefix batch", "prefix", batch.Prefix, "archives", len(batch.Archives))
		
		prefixPool.Go(func(ctx context.Context) error {
			prefixResult, err := p.uploadPrefixBatch(ctx, batch)
			
			resultMutex.Lock()
			if prefixResult != nil {
				result.Results = append(result.Results, prefixResult.Results...)
				result.PrefixStats[batch.Prefix] = prefixResult.Metrics
				result.TotalUploaded += prefixResult.TotalUploaded
				result.TotalErrors += prefixResult.TotalErrors
			}
			resultMutex.Unlock()
			
			return err
		})
	}
	
	// Wait for all prefix uploads to complete
	if err := prefixPool.Wait(); err != nil {
		slog.Error("parallel upload failed", "error", err)
		result.Duration = time.Since(result.StartTime)
		return result, err
	}
	
	result.Duration = time.Since(result.StartTime)
	result.CalculateMetrics()
	
	slog.Info("parallel upload completed", 
		"total_uploaded", result.TotalUploaded,
		"total_errors", result.TotalErrors,
		"duration", result.Duration,
		"avg_throughput_mbps", result.AverageThroughputMBps)
	
	return result, nil
}

// uploadPrefixBatch uploads all archives for a single prefix
func (p *ParallelUploader) uploadPrefixBatch(ctx context.Context, batch PrefixBatch) (*PrefixUploadResult, error) {
	prefixResult := &PrefixUploadResult{
		Prefix:  batch.Prefix,
		Results: make([]*UploadResult, 0, len(batch.Archives)),
		Metrics: &PrefixMetrics{
			LastUpload:    time.Now(),
			ActiveUploads: 0,
		},
		StartTime: time.Now(),
	}
	
	// Create worker pool for archive uploads within this prefix
	archivePool := pool.New().WithErrors().WithContext(ctx).WithMaxGoroutines(p.config.MaxConcurrentUploads)
	
	var resultMutex sync.Mutex
	
	// Upload each archive in the batch
	for _, archive := range batch.Archives {
		archive := archive // Capture for closure
		
		// Add prefix to archive key
		archive.Key = batch.Prefix + archive.Key
		
		archivePool.Go(func(ctx context.Context) error {
			// Update active uploads counter
			resultMutex.Lock()
			prefixResult.Metrics.ActiveUploads++
			resultMutex.Unlock()
			
			// Perform upload
			uploadResult, err := p.transporter.Upload(ctx, archive)
			
			// Update metrics
			resultMutex.Lock()
			prefixResult.Metrics.ActiveUploads--
			if err != nil {
				prefixResult.Metrics.ErrorCount++
				prefixResult.TotalErrors++
			} else {
				prefixResult.Results = append(prefixResult.Results, uploadResult)
				prefixResult.Metrics.UploadCount++
				prefixResult.Metrics.TotalBytes += archive.Size
				prefixResult.TotalUploaded++
			}
			resultMutex.Unlock()
			
			if err != nil {
				slog.Error("archive upload failed", "key", archive.Key, "error", err)
				return fmt.Errorf("upload failed for %s: %w", archive.Key, err)
			}
			
			slog.Debug("archive uploaded", "key", archive.Key, "size", archive.Size, "throughput", uploadResult.Throughput)
			return nil
		})
	}
	
	// Wait for all archive uploads in this prefix to complete
	err := archivePool.Wait()
	
	prefixResult.Duration = time.Since(prefixResult.StartTime)
	
	// Calculate throughput metrics
	if prefixResult.Duration.Seconds() > 0 {
		totalMB := float64(prefixResult.Metrics.TotalBytes) / (1024 * 1024)
		prefixResult.Metrics.AvgThroughput = totalMB / prefixResult.Duration.Seconds()
	}
	
	prefixResult.Metrics.LastUpload = time.Now()
	
	return prefixResult, err
}

// ParallelUploadResult contains results from parallel upload operation
type ParallelUploadResult struct {
	Results               []*UploadResult            `json:"results"`
	PrefixStats           map[string]*PrefixMetrics  `json:"prefix_stats"`
	TotalUploaded         int64                      `json:"total_uploaded"`
	TotalErrors           int                        `json:"total_errors"`
	Duration              time.Duration              `json:"duration"`
	StartTime             time.Time                  `json:"start_time"`
	AverageThroughputMBps float64                    `json:"avg_throughput_mbps"`
	TotalBytes            int64                      `json:"total_bytes"`
}

// PrefixUploadResult contains results for a single prefix batch
type PrefixUploadResult struct {
	Prefix        string           `json:"prefix"`
	Results       []*UploadResult  `json:"results"`
	Metrics       *PrefixMetrics   `json:"metrics"`
	TotalUploaded int64            `json:"total_uploaded"`
	TotalErrors   int              `json:"total_errors"`
	Duration      time.Duration    `json:"duration"`
	StartTime     time.Time        `json:"start_time"`
}

// CalculateMetrics computes aggregate metrics for the parallel upload
func (r *ParallelUploadResult) CalculateMetrics() {
	var totalBytes int64
	
	// Calculate total bytes from prefix stats
	for _, stats := range r.PrefixStats {
		totalBytes += stats.TotalBytes
	}
	
	r.TotalBytes = totalBytes
	
	if r.Duration.Seconds() > 0 {
		totalMB := float64(totalBytes) / (1024 * 1024)
		r.AverageThroughputMBps = totalMB / r.Duration.Seconds()
	}
}

// GetOptimalPrefixCount determines optimal prefix count based on data size and patterns
func GetOptimalPrefixCount(totalSize int64, archiveCount int) int {
	// Simple heuristic for optimal prefix count
	sizeGB := float64(totalSize) / (1024 * 1024 * 1024)
	
	switch {
	case sizeGB < 1:
		return 1 // Single prefix for small datasets
	case sizeGB < 10:
		return 2 // Two prefixes for medium datasets
	case sizeGB < 100:
		return 4 // Four prefixes for large datasets
	case sizeGB < 1000:
		return 8 // Eight prefixes for very large datasets
	default:
		return 16 // Maximum prefixes for massive datasets
	}
}

// OptimizePrefixDistribution analyzes and suggests optimal prefix configuration
func (p *ParallelUploader) OptimizePrefixDistribution(archives []Archive) *PrefixOptimization {
	totalSize := int64(0)
	maxSize := int64(0)
	minSize := int64(1<<63 - 1)
	
	for _, archive := range archives {
		totalSize += archive.Size
		if archive.Size > maxSize {
			maxSize = archive.Size
		}
		if archive.Size < minSize {
			minSize = archive.Size
		}
	}
	
	optimalPrefixes := GetOptimalPrefixCount(totalSize, len(archives))
	
	return &PrefixOptimization{
		RecommendedPrefixes:    optimalPrefixes,
		RecommendedConcurrency: min(optimalPrefixes*3, 16),
		TotalSize:              totalSize,
		ArchiveCount:           len(archives),
		SizeVariation:          float64(maxSize-minSize) / float64(totalSize),
		OptimalPattern:         p.selectOptimalPattern(totalSize, len(archives)),
	}
}

// PrefixOptimization contains optimization recommendations
type PrefixOptimization struct {
	RecommendedPrefixes    int     `json:"recommended_prefixes"`
	RecommendedConcurrency int     `json:"recommended_concurrency"`
	TotalSize              int64   `json:"total_size"`
	ArchiveCount           int     `json:"archive_count"`
	SizeVariation          float64 `json:"size_variation"`
	OptimalPattern         string  `json:"optimal_pattern"`
}

// selectOptimalPattern chooses the best prefix pattern for the dataset
func (p *ParallelUploader) selectOptimalPattern(totalSize int64, archiveCount int) string {
	sizeGB := float64(totalSize) / (1024 * 1024 * 1024)
	
	switch {
	case archiveCount < 100:
		return "sequential" // Simple for small archive counts
	case sizeGB > 100:
		return "hash" // Hash-based for large datasets (avoids S3 hotspots)
	default:
		return "date" // Date-based for organization and moderate performance
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}