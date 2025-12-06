package chunking

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// CompressionEstimator estimates compression ratios for files
// by sampling and compressing the first few KB of each file.
type CompressionEstimator struct {
	encoder    *zstd.Encoder
	sampleSize int64
	cacheMu    sync.RWMutex
	cache      map[string]float64 // Cache by file extension
}

// NewCompressionEstimator creates a new compression estimator
func NewCompressionEstimator() (*CompressionEstimator, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedDefault),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	return &CompressionEstimator{
		encoder:    encoder,
		sampleSize: 4096, // 4KB sample
		cache:      make(map[string]float64),
	}, nil
}

// EstimateCompressionRatio estimates compression ratio for a file
// Returns ratio (0.0-1.0) where 0.3 means 70% compression (compressed = 30% of original)
func (ce *CompressionEstimator) EstimateCompressionRatio(filePath string) (float64, error) {
	// Check cache by extension
	ext := filepath.Ext(filePath)
	if ext != "" {
		ce.cacheMu.RLock()
		if ratio, exists := ce.cache[ext]; exists {
			ce.cacheMu.RUnlock()
			return ratio, nil
		}
		ce.cacheMu.RUnlock()
	}

	// Open file and read sample
	file, err := os.Open(filePath)
	if err != nil {
		return 1.0, fmt.Errorf("failed to open file: %w", err) // Assume no compression on error
	}
	defer func() {
		_ = file.Close()
	}()

	// Read first N KB
	sample := make([]byte, ce.sampleSize)
	n, err := io.ReadFull(file, sample)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 1.0, fmt.Errorf("failed to read file sample: %w", err)
	}
	if n == 0 {
		return 1.0, nil // Empty file, no compression
	}
	sample = sample[:n]

	// Compress sample
	var compressed bytes.Buffer
	ce.encoder.Reset(&compressed)
	_, err = ce.encoder.Write(sample)
	if err != nil {
		return 1.0, fmt.Errorf("failed to compress sample: %w", err)
	}
	if err := ce.encoder.Close(); err != nil {
		return 1.0, fmt.Errorf("failed to close encoder: %w", err)
	}

	// Calculate ratio
	ratio := float64(compressed.Len()) / float64(len(sample))

	// Cache result if extension is known
	if ext != "" {
		ce.cacheMu.Lock()
		ce.cache[ext] = ratio
		ce.cacheMu.Unlock()
	}

	return ratio, nil
}

// EstimateCompressedSize estimates compressed size for a file
func (ce *CompressionEstimator) EstimateCompressedSize(filePath string, fileSize int64) (int64, error) {
	if fileSize == 0 {
		return 0, nil
	}

	ratio, err := ce.EstimateCompressionRatio(filePath)
	if err != nil {
		// On error, assume no compression
		return fileSize, err
	}

	return int64(float64(fileSize) * ratio), nil
}

// GetCacheStats returns statistics about the compression ratio cache
func (ce *CompressionEstimator) GetCacheStats() map[string]interface{} {
	ce.cacheMu.RLock()
	defer ce.cacheMu.RUnlock()

	stats := make(map[string]interface{})
	stats["cached_extensions"] = len(ce.cache)

	// Copy cache for inspection
	cacheCopy := make(map[string]float64)
	for k, v := range ce.cache {
		cacheCopy[k] = v
	}
	stats["cache"] = cacheCopy

	return stats
}

// ClearCache clears the compression ratio cache
func (ce *CompressionEstimator) ClearCache() {
	ce.cacheMu.Lock()
	defer ce.cacheMu.Unlock()
	ce.cache = make(map[string]float64)
}
