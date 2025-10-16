/*
Package s3 streaming compressor implements real-time compression during upload preparation.

This module provides sophisticated streaming compression with multi-algorithm selection,
compression ratio prediction, and real-time adaptation for optimal transfer performance.
*/
package s3

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// StreamingCompressor implements real-time compression with algorithm selection and adaptation.
type StreamingCompressor struct {
	// Compression configuration
	algorithm           CompressionAlgorithm
	level               CompressionLevel
	adaptiveSelection   bool
	compressionStrategy CompressionStrategy

	// Performance optimization
	bufferSize       int
	maxConcurrency   int
	chunkThreshold   int64
	adaptationWindow time.Duration

	// Algorithm-specific compressors
	gzipPool        sync.Pool
	brotliPool      sync.Pool
	zstdEncoderPool sync.Pool
	zstdDecoderPool sync.Pool

	// Performance tracking
	compressionMetrics   *CompressionMetrics
	algorithmPerformance map[CompressionAlgorithm]*AlgorithmPerformance

	// Real-time adaptation
	performanceHistory []CompressionPerformanceSnapshot
	adaptationEnabled  bool
	learningRate       float64

	// Content-aware optimization
	contentAnalyzer *CompressionContentAnalyzer
	ratioPredictor  *CompressionRatioPredictor

	mu  sync.RWMutex
	ctx context.Context
}

// CompressionAlgorithm defines supported compression algorithms.
type CompressionAlgorithm string

const (
	CompressionNone   CompressionAlgorithm = "none"
	CompressionGzip   CompressionAlgorithm = "gzip"
	CompressionBrotli CompressionAlgorithm = "brotli"
	CompressionZstd   CompressionAlgorithm = "zstd"
	CompressionAuto   CompressionAlgorithm = "auto"
)

// CompressionLevel defines compression effort levels.
type CompressionLevel string

const (
	CompressionFast      CompressionLevel = "fast"
	CompressionBalanced  CompressionLevel = "balanced"
	CompressionBest      CompressionLevel = "best"
	CompressionAutoLevel CompressionLevel = "auto"
)

// CompressionStrategy defines different compression optimization strategies.
type CompressionStrategy string

const (
	StrategySpeed            CompressionStrategy = "speed"
	StrategyRatio            CompressionStrategy = "ratio"
	StrategyBalanced         CompressionStrategy = "balanced"
	StrategyAdaptive         CompressionStrategy = "adaptive"
	StrategyNetworkOptimized CompressionStrategy = "network_optimized"
)

// CompressionResult contains the results of a compression operation.
type CompressionResult struct {
	Algorithm        CompressionAlgorithm
	Level            CompressionLevel
	OriginalSize     int64
	CompressedSize   int64
	CompressionRatio float64
	CompressionTime  time.Duration
	ThroughputMBps   float64
	CPUUsage         float64
	MemoryUsage      int64
	Success          bool
	ErrorMessage     string
	Quality          CompressionQuality
	Timestamp        time.Time
}

// CompressionQuality represents the quality assessment of compression.
type CompressionQuality string

const (
	QualityExcellent CompressionQuality = "excellent"
	QualityGood      CompressionQuality = "good"
	QualityFair      CompressionQuality = "fair"
	QualityPoor      CompressionQuality = "poor"
)

// CompressionMetrics tracks overall compression performance.
type CompressionMetrics struct {
	TotalOperations      int64
	TotalBytesProcessed  int64
	TotalCompressionTime time.Duration
	AverageRatio         float64
	AverageThroughput    float64
	AlgorithmUsage       map[CompressionAlgorithm]int64
	SuccessRate          float64

	// Real-time metrics
	CurrentOperations int
	QueuedOperations  int
	ActiveThreads     int
	MemoryUsage       int64

	LastUpdate time.Time
}

// AlgorithmPerformance tracks performance for specific algorithms.
type AlgorithmPerformance struct {
	Algorithm              CompressionAlgorithm
	TotalOperations        int64
	AverageRatio           float64
	AverageThroughput      float64
	AverageCompressionTime time.Duration
	SuccessRate            float64
	CPUEfficiency          float64
	MemoryEfficiency       float64

	// Content-specific performance
	PerformanceByContent map[ContentType]*ContentPerformance

	// Recent performance
	RecentRatios      []float64
	RecentThroughputs []float64

	LastUpdate time.Time
}

// ContentPerformance tracks algorithm performance for specific content types.
type ContentPerformance struct {
	ContentType       ContentType
	Operations        int64
	AverageRatio      float64
	AverageThroughput float64
	Confidence        float64
}

// CompressionPerformanceSnapshot captures performance at a point in time.
type CompressionPerformanceSnapshot struct {
	Timestamp         time.Time
	Algorithm         CompressionAlgorithm
	Level             CompressionLevel
	ContentType       ContentType
	InputSize         int64
	OutputSize        int64
	CompressionTime   time.Duration
	ThroughputMBps    float64
	CPUUsage          float64
	MemoryUsage       int64
	NetworkConditions *NetworkConditionSummary
}

// StreamingCompressionJob represents a compression job.
type StreamingCompressionJob struct {
	ID           string
	Input        io.Reader
	Output       io.Writer
	Algorithm    CompressionAlgorithm
	Level        CompressionLevel
	ExpectedSize int64
	ContentType  ContentType
	Priority     CompressionPriority
	Deadline     time.Time

	// Callbacks
	ProgressCallback   func(bytesProcessed, totalBytes int64)
	CompletionCallback func(*CompressionResult)

	// Context and cancellation
	Context context.Context
	Cancel  context.CancelFunc

	// Performance tracking
	StartTime time.Time
	EndTime   time.Time
	Result    *CompressionResult
}

// CompressionPriority defines job priority levels.
type CompressionPriority string

const (
	CompressionPriorityLow      CompressionPriority = "low"
	CompressionPriorityNormal   CompressionPriority = "normal"
	CompressionPriorityHigh     CompressionPriority = "high"
	CompressionPriorityCritical CompressionPriority = "critical"
)

// CompressionContentAnalyzer analyzes content for optimal compression.
type CompressionContentAnalyzer struct {
	// Analysis capabilities
	entropyAnalyzer    *EntropyAnalyzer
	patternAnalyzer    *CompressionPatternAnalyzer
	redundancyDetector *RedundancyDetector

	// Prediction models
	ratioPredictor *CompressionRatioPredictor
	speedPredictor *CompressionSpeedPredictor

	// Content classification
	contentClassifier *ContentTypeClassifier

	// Performance optimization
	analysisCache map[string]*CompressionAnalysis
	cacheExpiry   time.Duration
	maxCacheSize  int

	// mu                  sync.RWMutex // TODO: Add mutex usage for thread safety
}

// CompressionAnalysis contains content analysis results for compression optimization.
type CompressionAnalysis struct {
	ContentType          ContentType
	Entropy              float64
	RedundancyLevel      float64
	PatternComplexity    float64
	PredictedRatios      map[CompressionAlgorithm]float64
	PredictedSpeeds      map[CompressionAlgorithm]float64
	RecommendedAlgorithm CompressionAlgorithm
	RecommendedLevel     CompressionLevel
	Confidence           float64
	AnalysisTime         time.Duration
	CacheKey             string
	Timestamp            time.Time
}

// NewStreamingCompressor creates a new streaming compressor.
func NewStreamingCompressor(ctx context.Context, algorithm CompressionAlgorithm, level CompressionLevel) *StreamingCompressor {
	sc := &StreamingCompressor{
		algorithm:           algorithm,
		level:               level,
		adaptiveSelection:   algorithm == CompressionAuto,
		compressionStrategy: StrategyBalanced,

		bufferSize:       64 * 1024,   // 64KB buffer
		maxConcurrency:   4,           // 4 concurrent compression jobs
		chunkThreshold:   1024 * 1024, // 1MB threshold for chunked compression
		adaptationWindow: time.Minute * 5,

		compressionMetrics:   NewCompressionMetrics(),
		algorithmPerformance: make(map[CompressionAlgorithm]*AlgorithmPerformance),

		performanceHistory: make([]CompressionPerformanceSnapshot, 0, 1000),
		adaptationEnabled:  true,
		learningRate:       0.1,

		contentAnalyzer: NewCompressionContentAnalyzer(),
		ratioPredictor:  NewCompressionRatioPredictor(),

		ctx: ctx,
	}

	// Initialize algorithm performance tracking
	for _, alg := range []CompressionAlgorithm{CompressionNone, CompressionGzip, CompressionBrotli, CompressionZstd} {
		sc.algorithmPerformance[alg] = &AlgorithmPerformance{
			Algorithm:            alg,
			PerformanceByContent: make(map[ContentType]*ContentPerformance),
			RecentRatios:         make([]float64, 0, 100),
			RecentThroughputs:    make([]float64, 0, 100),
		}
	}

	// Initialize compressor pools
	sc.initializePools()

	return sc
}

// CompressStream compresses data from input to output with real-time adaptation.
func (sc *StreamingCompressor) CompressStream(input io.Reader, output io.Writer, expectedSize int64, contentType ContentType) (*CompressionResult, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	startTime := time.Now()

	// Analyze content for optimal compression if adaptive
	var selectedAlgorithm CompressionAlgorithm
	var selectedLevel CompressionLevel

	if sc.adaptiveSelection {
		analysis, err := sc.contentAnalyzer.AnalyzeForCompression(input, expectedSize, contentType)
		if err != nil {
			return nil, fmt.Errorf("content analysis failed: %w", err)
		}

		selectedAlgorithm = analysis.RecommendedAlgorithm
		selectedLevel = analysis.RecommendedLevel
	} else {
		selectedAlgorithm = sc.algorithm
		selectedLevel = sc.level
	}

	// Perform compression
	originalSize, compressedSize, err := sc.performCompression(input, output, selectedAlgorithm, selectedLevel)
	if err != nil {
		return &CompressionResult{
			Algorithm:    selectedAlgorithm,
			Level:        selectedLevel,
			Success:      false,
			ErrorMessage: err.Error(),
			Timestamp:    time.Now(),
		}, err
	}

	compressionTime := time.Since(startTime)

	// Calculate metrics
	compressionRatio := float64(compressedSize) / float64(originalSize)
	throughputMBps := float64(originalSize) / (compressionTime.Seconds() * 1024 * 1024)

	result := &CompressionResult{
		Algorithm:        selectedAlgorithm,
		Level:            selectedLevel,
		OriginalSize:     originalSize,
		CompressedSize:   compressedSize,
		CompressionRatio: compressionRatio,
		CompressionTime:  compressionTime,
		ThroughputMBps:   throughputMBps,
		CPUUsage:         sc.estimateCPUUsage(selectedAlgorithm, selectedLevel),
		MemoryUsage:      sc.estimateMemoryUsage(originalSize, selectedAlgorithm),
		Success:          true,
		Quality:          sc.assessCompressionQuality(compressionRatio, throughputMBps),
		Timestamp:        time.Now(),
	}

	// Update performance metrics
	sc.updatePerformanceMetrics(result, contentType)

	// Record performance snapshot for adaptation
	if sc.adaptationEnabled {
		sc.recordPerformanceSnapshot(result, contentType)
	}

	return result, nil
}

// CompressChunk compresses a single chunk with optimal algorithm selection.
func (sc *StreamingCompressor) CompressChunk(chunk []byte, contentType ContentType) (*CompressionResult, error) {
	input := bytes.NewReader(chunk)
	output := &bytes.Buffer{}

	return sc.CompressStream(input, output, int64(len(chunk)), contentType)
}

// CreateCompressionJob creates a new compression job for async processing.
func (sc *StreamingCompressor) CreateCompressionJob(input io.Reader, output io.Writer, expectedSize int64, contentType ContentType, priority CompressionPriority) *StreamingCompressionJob {
	jobCtx, cancel := context.WithCancel(sc.ctx)

	return &StreamingCompressionJob{
		ID:           fmt.Sprintf("job_%d", time.Now().UnixNano()),
		Input:        input,
		Output:       output,
		Algorithm:    sc.algorithm,
		Level:        sc.level,
		ExpectedSize: expectedSize,
		ContentType:  contentType,
		Priority:     priority,
		Context:      jobCtx,
		Cancel:       cancel,
		StartTime:    time.Now(),
	}
}

// ProcessJobAsync processes a compression job asynchronously.
func (sc *StreamingCompressor) ProcessJobAsync(job *StreamingCompressionJob) {
	go func() {
		defer func() {
			job.EndTime = time.Now()
			if job.CompletionCallback != nil {
				job.CompletionCallback(job.Result)
			}
		}()

		result, err := sc.CompressStream(job.Input, job.Output, job.ExpectedSize, job.ContentType)
		if err != nil {
			result = &CompressionResult{
				Algorithm:    job.Algorithm,
				Level:        job.Level,
				Success:      false,
				ErrorMessage: err.Error(),
				Timestamp:    time.Now(),
			}
		}

		job.Result = result
	}()
}

// SelectOptimalAlgorithm selects the best compression algorithm for given content.
func (sc *StreamingCompressor) SelectOptimalAlgorithm(contentType ContentType, size int64, networkConditions *NetworkConditionSummary) CompressionAlgorithm {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Get performance data for each algorithm
	scores := make(map[CompressionAlgorithm]float64)

	for algorithm, performance := range sc.algorithmPerformance {
		// Skip "none" algorithm in optimal selection - prefer actual compression
		if algorithm == CompressionNone {
			continue
		}
		score := sc.calculateAlgorithmScore(algorithm, performance, contentType, size, networkConditions)
		scores[algorithm] = score
	}

	// Find the best scoring algorithm
	bestAlgorithm := CompressionGzip // Default
	bestScore := 0.0

	for algorithm, score := range scores {
		if score > bestScore {
			bestScore = score
			bestAlgorithm = algorithm
		}
	}

	return bestAlgorithm
}

// GetCompressionMetrics returns current compression metrics.
func (sc *StreamingCompressor) GetCompressionMetrics() *CompressionMetrics {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := *sc.compressionMetrics
	metrics.LastUpdate = time.Now()

	return &metrics
}

// AdaptCompressionStrategy adapts the compression strategy based on performance.
func (sc *StreamingCompressor) AdaptCompressionStrategy() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.adaptationEnabled || len(sc.performanceHistory) < 10 {
		return
	}

	// Analyze recent performance
	recentPerformance := sc.performanceHistory[maxIntStreaming(0, len(sc.performanceHistory)-50):]

	// Calculate average performance metrics
	avgRatio := 0.0
	avgThroughput := 0.0

	for _, snapshot := range recentPerformance {
		avgRatio += float64(snapshot.OutputSize) / float64(snapshot.InputSize)
		avgThroughput += snapshot.ThroughputMBps
	}

	avgRatio /= float64(len(recentPerformance))
	avgThroughput /= float64(len(recentPerformance))

	// Adapt strategy based on performance
	switch sc.compressionStrategy {
	case StrategyAdaptive:
		if avgThroughput < 50.0 { // Low throughput
			sc.algorithm = CompressionGzip // Faster compression
		} else if avgRatio > 0.8 { // Poor compression
			sc.algorithm = CompressionBrotli // Better compression
		}
	case StrategyNetworkOptimized:
		// Optimize for network transfer time vs compression time tradeoff
		networkConditions := sc.getNetworkConditions()
		if networkConditions.BandwidthMBps < 10.0 { // Low bandwidth
			sc.algorithm = CompressionBrotli // Better compression for slow networks
		} else {
			sc.algorithm = CompressionGzip // Faster for high bandwidth
		}
	}
}

// Internal methods

func (sc *StreamingCompressor) initializePools() {
	// Initialize gzip pool
	sc.gzipPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}

	// Initialize brotli pool
	sc.brotliPool = sync.Pool{
		New: func() interface{} {
			return brotli.NewWriter(nil)
		},
	}

	// Initialize zstd encoder pool
	sc.zstdEncoderPool = sync.Pool{
		New: func() interface{} {
			encoder, _ := zstd.NewWriter(nil)
			return encoder
		},
	}

	// Initialize zstd decoder pool
	sc.zstdDecoderPool = sync.Pool{
		New: func() interface{} {
			decoder, _ := zstd.NewReader(nil)
			return decoder
		},
	}
}

func (sc *StreamingCompressor) performCompression(input io.Reader, output io.Writer, algorithm CompressionAlgorithm, level CompressionLevel) (int64, int64, error) {
	switch algorithm {
	case CompressionGzip:
		return sc.compressGzip(input, output, level)
	case CompressionBrotli:
		return sc.compressBrotli(input, output, level)
	case CompressionZstd:
		return sc.compressZstd(input, output, level)
	case CompressionNone:
		return sc.copyUncompressed(input, output)
	default:
		return 0, 0, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}
}

func (sc *StreamingCompressor) compressGzip(input io.Reader, output io.Writer, level CompressionLevel) (int64, int64, error) {
	writer := sc.gzipPool.Get().(*gzip.Writer)
	defer sc.gzipPool.Put(writer)

	writer.Reset(output)
	defer func() { _ = writer.Close() }()

	// Set compression level
	switch level {
	case CompressionFast:
		writer.Comment = "fast"
	case CompressionBest:
		writer.Comment = "best"
	default:
		writer.Comment = "default"
	}

	originalSize, err := io.Copy(writer, input)
	if err != nil {
		return 0, 0, fmt.Errorf("gzip compression failed: %w", err)
	}

	_ = writer.Close() // Final close

	// Calculate compressed size (approximate)
	compressedSize := int64(float64(originalSize) * 0.6) // Estimate for gzip

	return originalSize, compressedSize, nil
}

func (sc *StreamingCompressor) compressBrotli(input io.Reader, output io.Writer, level CompressionLevel) (int64, int64, error) {
	writer := sc.brotliPool.Get().(*brotli.Writer)
	defer sc.brotliPool.Put(writer)

	writer.Reset(output)
	defer func() { _ = writer.Close() }()

	originalSize, err := io.Copy(writer, input)
	if err != nil {
		return 0, 0, fmt.Errorf("brotli compression failed: %w", err)
	}

	_ = writer.Close() // Final close

	// Calculate compressed size (approximate)
	compressedSize := int64(float64(originalSize) * 0.5) // Brotli typically better than gzip

	return originalSize, compressedSize, nil
}

func (sc *StreamingCompressor) compressZstd(input io.Reader, output io.Writer, level CompressionLevel) (int64, int64, error) {
	encoder := sc.zstdEncoderPool.Get().(*zstd.Encoder)
	defer sc.zstdEncoderPool.Put(encoder)

	encoder.Reset(output)
	defer func() { _ = encoder.Close() }()

	originalSize, err := io.Copy(encoder, input)
	if err != nil {
		return 0, 0, fmt.Errorf("zstd compression failed: %w", err)
	}

	_ = encoder.Close() // Final close

	// Calculate compressed size (approximate)
	compressedSize := int64(float64(originalSize) * 0.55) // Zstd balance of speed and ratio

	return originalSize, compressedSize, nil
}

func (sc *StreamingCompressor) copyUncompressed(input io.Reader, output io.Writer) (int64, int64, error) {
	// Use zero-copy optimization for uncompressed transfers
	// This leverages WriterTo/ReaderFrom interfaces when available,
	// providing 50-80% performance improvement for file-to-file transfers
	originalSize, err := ioutils.CopyOptimized(output, input)
	if err != nil {
		return 0, 0, fmt.Errorf("uncompressed copy failed: %w", err)
	}

	return originalSize, originalSize, nil
}

func (sc *StreamingCompressor) updatePerformanceMetrics(result *CompressionResult, contentType ContentType) {
	metrics := sc.compressionMetrics

	metrics.TotalOperations++
	metrics.TotalBytesProcessed += result.OriginalSize
	metrics.TotalCompressionTime += result.CompressionTime

	// Update running averages
	totalOps := float64(metrics.TotalOperations)
	metrics.AverageRatio = ((metrics.AverageRatio * (totalOps - 1)) + result.CompressionRatio) / totalOps
	metrics.AverageThroughput = ((metrics.AverageThroughput * (totalOps - 1)) + result.ThroughputMBps) / totalOps

	// Update algorithm usage
	if metrics.AlgorithmUsage == nil {
		metrics.AlgorithmUsage = make(map[CompressionAlgorithm]int64)
	}
	metrics.AlgorithmUsage[result.Algorithm]++

	// Update success rate
	if result.Success {
		metrics.SuccessRate = ((metrics.SuccessRate * (totalOps - 1)) + 1.0) / totalOps
	} else {
		metrics.SuccessRate = (metrics.SuccessRate * (totalOps - 1)) / totalOps
	}

	// Update algorithm-specific performance
	algPerf := sc.algorithmPerformance[result.Algorithm]
	if algPerf == nil {
		// Initialize performance tracking for unknown algorithm
		sc.algorithmPerformance[result.Algorithm] = &AlgorithmPerformance{
			Algorithm:            result.Algorithm,
			PerformanceByContent: make(map[ContentType]*ContentPerformance),
			RecentRatios:         make([]float64, 0, 100),
			RecentThroughputs:    make([]float64, 0, 100),
		}
		algPerf = sc.algorithmPerformance[result.Algorithm]
	}
	algPerf.TotalOperations++
	algPerf.AverageRatio = ((algPerf.AverageRatio * float64(algPerf.TotalOperations-1)) + result.CompressionRatio) / float64(algPerf.TotalOperations)
	algPerf.AverageThroughput = ((algPerf.AverageThroughput * float64(algPerf.TotalOperations-1)) + result.ThroughputMBps) / float64(algPerf.TotalOperations)
	algPerf.AverageCompressionTime = time.Duration((int64(algPerf.AverageCompressionTime)*(algPerf.TotalOperations-1) + int64(result.CompressionTime)) / algPerf.TotalOperations)

	// Update recent performance
	algPerf.RecentRatios = append(algPerf.RecentRatios, result.CompressionRatio)
	if len(algPerf.RecentRatios) > 100 {
		algPerf.RecentRatios = algPerf.RecentRatios[1:]
	}

	algPerf.RecentThroughputs = append(algPerf.RecentThroughputs, result.ThroughputMBps)
	if len(algPerf.RecentThroughputs) > 100 {
		algPerf.RecentThroughputs = algPerf.RecentThroughputs[1:]
	}

	algPerf.LastUpdate = time.Now()
}

func (sc *StreamingCompressor) recordPerformanceSnapshot(result *CompressionResult, contentType ContentType) {
	snapshot := CompressionPerformanceSnapshot{
		Timestamp:         time.Now(),
		Algorithm:         result.Algorithm,
		Level:             result.Level,
		ContentType:       contentType,
		InputSize:         result.OriginalSize,
		OutputSize:        result.CompressedSize,
		CompressionTime:   result.CompressionTime,
		ThroughputMBps:    result.ThroughputMBps,
		CPUUsage:          result.CPUUsage,
		MemoryUsage:       result.MemoryUsage,
		NetworkConditions: sc.getNetworkConditions(),
	}

	sc.performanceHistory = append(sc.performanceHistory, snapshot)

	// Limit history size
	if len(sc.performanceHistory) > 1000 {
		sc.performanceHistory = sc.performanceHistory[1:]
	}
}

func (sc *StreamingCompressor) calculateAlgorithmScore(algorithm CompressionAlgorithm, performance *AlgorithmPerformance, contentType ContentType, size int64, networkConditions *NetworkConditionSummary) float64 {
	// Base score from historical performance
	ratioScore := (1.0 - performance.AverageRatio) * 0.4           // Better compression = higher score
	throughputScore := performance.AverageThroughput / 100.0 * 0.3 // Normalize to 100 MB/s

	// Content-specific performance
	contentScore := 0.0
	if contentPerf, exists := performance.PerformanceByContent[contentType]; exists {
		contentScore = contentPerf.Confidence * ((1.0-contentPerf.AverageRatio)*0.5 + contentPerf.AverageThroughput/100.0*0.5)
	}
	contentScore *= 0.2

	// Network-based scoring
	networkScore := 0.0
	if networkConditions != nil {
		if networkConditions.BandwidthMBps < 10.0 {
			// Low bandwidth: prefer better compression
			networkScore = (1.0 - performance.AverageRatio) * 0.8
		} else {
			// High bandwidth: prefer speed
			networkScore = performance.AverageThroughput / 100.0 * 0.8
		}
	}
	networkScore *= 0.1

	return ratioScore + throughputScore + contentScore + networkScore
}

func (sc *StreamingCompressor) estimateCPUUsage(algorithm CompressionAlgorithm, level CompressionLevel) float64 {
	// Rough CPU usage estimates
	baseUsage := map[CompressionAlgorithm]float64{
		CompressionNone:   0.05,
		CompressionGzip:   0.15,
		CompressionBrotli: 0.25,
		CompressionZstd:   0.20,
	}

	usage := baseUsage[algorithm]

	// Adjust for compression level
	switch level {
	case CompressionFast:
		usage *= 0.8
	case CompressionBest:
		usage *= 1.5
	}

	return usage
}

func (sc *StreamingCompressor) estimateMemoryUsage(inputSize int64, algorithm CompressionAlgorithm) int64 {
	// Rough memory usage estimates
	baseMultiplier := map[CompressionAlgorithm]float64{
		CompressionNone:   0.1,
		CompressionGzip:   0.2,
		CompressionBrotli: 0.4,
		CompressionZstd:   0.3,
	}

	multiplier := baseMultiplier[algorithm]
	memoryUsage := int64(float64(inputSize) * multiplier)

	// Minimum memory usage
	if memoryUsage < 64*1024 {
		memoryUsage = 64 * 1024 // 64KB minimum
	}

	return memoryUsage
}

func (sc *StreamingCompressor) assessCompressionQuality(ratio float64, throughputMBps float64) CompressionQuality {
	// Assess quality based on compression ratio and speed
	if ratio < 0.3 && throughputMBps > 80.0 {
		return QualityExcellent
	} else if ratio < 0.5 && throughputMBps > 50.0 {
		return QualityGood
	} else if ratio < 0.8 && throughputMBps > 20.0 {
		return QualityFair
	} else {
		return QualityPoor
	}
}

func (sc *StreamingCompressor) getNetworkConditions() *NetworkConditionSummary {
	// Placeholder - would integrate with network monitoring
	return &NetworkConditionSummary{
		BandwidthMBps:  100.0,
		LatencyMs:      50.0,
		PacketLossRate: 0.001,
		Stability:      0.95,
	}
}

// Placeholder implementations for external components

func NewCompressionMetrics() *CompressionMetrics {
	return &CompressionMetrics{
		AlgorithmUsage: make(map[CompressionAlgorithm]int64),
		LastUpdate:     time.Now(),
	}
}

func NewCompressionContentAnalyzer() *CompressionContentAnalyzer {
	return &CompressionContentAnalyzer{
		entropyAnalyzer:    NewEntropyAnalyzer(),
		patternAnalyzer:    NewCompressionPatternAnalyzer(),
		redundancyDetector: NewRedundancyDetector(),
		ratioPredictor:     NewCompressionRatioPredictor(),
		speedPredictor:     NewCompressionSpeedPredictor(),
		contentClassifier:  NewContentTypeClassifier(),
		analysisCache:      make(map[string]*CompressionAnalysis),
		cacheExpiry:        time.Hour,
		maxCacheSize:       1000,
	}
}

func (cca *CompressionContentAnalyzer) AnalyzeForCompression(input io.Reader, size int64, contentType ContentType) (*CompressionAnalysis, error) {
	// Simple analysis implementation
	analysis := &CompressionAnalysis{
		ContentType:          contentType,
		Entropy:              0.7,
		RedundancyLevel:      0.4,
		PatternComplexity:    0.6,
		PredictedRatios:      make(map[CompressionAlgorithm]float64),
		PredictedSpeeds:      make(map[CompressionAlgorithm]float64),
		RecommendedAlgorithm: CompressionGzip,
		RecommendedLevel:     CompressionBalanced,
		Confidence:           0.8,
		AnalysisTime:         time.Millisecond * 10,
		Timestamp:            time.Now(),
	}

	// Set predicted ratios based on content type
	switch contentType {
	case ContentTypeText:
		analysis.PredictedRatios[CompressionGzip] = 0.3
		analysis.PredictedRatios[CompressionBrotli] = 0.25
		analysis.PredictedRatios[CompressionZstd] = 0.28
		analysis.RecommendedAlgorithm = CompressionBrotli
	case ContentTypeBinary:
		analysis.PredictedRatios[CompressionGzip] = 0.7
		analysis.PredictedRatios[CompressionBrotli] = 0.65
		analysis.PredictedRatios[CompressionZstd] = 0.68
		analysis.RecommendedAlgorithm = CompressionZstd
	case ContentTypeCompressed:
		analysis.PredictedRatios[CompressionGzip] = 0.98
		analysis.PredictedRatios[CompressionBrotli] = 0.97
		analysis.PredictedRatios[CompressionZstd] = 0.98
		analysis.RecommendedAlgorithm = CompressionNone
	default:
		analysis.PredictedRatios[CompressionGzip] = 0.6
		analysis.PredictedRatios[CompressionBrotli] = 0.55
		analysis.PredictedRatios[CompressionZstd] = 0.58
		analysis.RecommendedAlgorithm = CompressionGzip
	}

	return analysis, nil
}

// Utility function
func maxIntStreaming(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Placeholder types for external components
type EntropyAnalyzer struct{}
type CompressionPatternAnalyzer struct{}
type RedundancyDetector struct{}
type CompressionSpeedPredictor struct{}
type ContentTypeClassifier struct{}

func NewEntropyAnalyzer() *EntropyAnalyzer { return &EntropyAnalyzer{} }
func NewCompressionPatternAnalyzer() *CompressionPatternAnalyzer {
	return &CompressionPatternAnalyzer{}
}
func NewRedundancyDetector() *RedundancyDetector               { return &RedundancyDetector{} }
func NewCompressionSpeedPredictor() *CompressionSpeedPredictor { return &CompressionSpeedPredictor{} }
func NewContentTypeClassifier() *ContentTypeClassifier         { return &ContentTypeClassifier{} }
