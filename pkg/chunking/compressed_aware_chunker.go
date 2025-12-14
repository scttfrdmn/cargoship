package chunking

import (
	"fmt"
	"sync"
)

// CompressedAwareChunker creates chunks based on estimated compressed size
// rather than raw file size, providing uniform chunk sizes after compression.
type CompressedAwareChunker struct {
	estimator     *CompressionEstimator
	targetCalc    *AdaptiveTargetCalculator
	mu            sync.RWMutex
	lastChunkSize int // Last calculated optimal chunk size (MB)
}

// NewCompressedAwareChunker creates a new compressed-aware chunker
func NewCompressedAwareChunker() (*CompressedAwareChunker, error) {
	estimator, err := NewCompressionEstimator()
	if err != nil {
		return nil, fmt.Errorf("failed to create compression estimator: %w", err)
	}

	return &CompressedAwareChunker{
		estimator:  estimator,
		targetCalc: NewAdaptiveTargetCalculator(),
	}, nil
}

// NewCompressedAwareChunkerWithConfig creates a chunker with custom configuration
func NewCompressedAwareChunkerWithConfig(
	estimator *CompressionEstimator,
	targetCalc *AdaptiveTargetCalculator,
) *CompressedAwareChunker {
	return &CompressedAwareChunker{
		estimator:  estimator,
		targetCalc: targetCalc,
	}
}

// CreateChunks creates optimally-sized chunks based on estimated compressed size
func (cac *CompressedAwareChunker) CreateChunks(files []File) ([]Chunk, error) {
	if len(files) == 0 {
		return []Chunk{}, nil
	}

	// Step 1: Estimate compressed size for all files
	fileEstimates := make([]FileEstimate, 0, len(files))
	totalEstimatedCompressedSize := int64(0)

	for _, file := range files {
		compressedSize, err := cac.estimator.EstimateCompressedSize(file.Path, file.Size)
		if err != nil {
			// On error, assume no compression (use raw size)
			compressedSize = file.Size
		}

		fileEstimates = append(fileEstimates, FileEstimate{
			File:                file,
			EstimatedCompressed: compressedSize,
			CompressionRatio:    float64(compressedSize) / float64(file.Size),
		})

		totalEstimatedCompressedSize += compressedSize
	}

	// Step 2: Calculate optimal chunk size based on total estimated compressed size
	optimalChunkSizeMB := cac.targetCalc.CalculateOptimalChunkSize(totalEstimatedCompressedSize)
	cac.mu.Lock()
	cac.lastChunkSize = optimalChunkSizeMB
	cac.mu.Unlock()

	targetChunkSizeBytes := int64(optimalChunkSizeMB) * 1024 * 1024

	// Step 3: Create chunks targeting optimal compressed size
	chunks := make([]Chunk, 0)
	currentChunk := Chunk{
		ID:        0,
		Files:     make([]File, 0),
		TotalSize: 0,
		FileCount: 0,
	}
	currentEstimatedCompressed := int64(0)

	for _, estimate := range fileEstimates {
		// Check if adding this file would exceed target
		if currentEstimatedCompressed > 0 &&
			(currentEstimatedCompressed+estimate.EstimatedCompressed) > targetChunkSizeBytes {
			// Finalize current chunk
			chunks = append(chunks, currentChunk)

			// Start new chunk
			currentChunk = Chunk{
				ID:        len(chunks),
				Files:     make([]File, 0),
				TotalSize: 0,
				FileCount: 0,
			}
			currentEstimatedCompressed = 0
		}

		// Add file to current chunk
		currentChunk.Files = append(currentChunk.Files, estimate.File)
		currentChunk.TotalSize += estimate.File.Size
		currentChunk.FileCount++
		currentEstimatedCompressed += estimate.EstimatedCompressed
	}

	// Add final chunk if not empty
	if currentChunk.FileCount > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// CreateChunksWithMetadata creates chunks and returns additional metadata
func (cac *CompressedAwareChunker) CreateChunksWithMetadata(files []File) (ChunkingResult, error) {
	if len(files) == 0 {
		return ChunkingResult{
			Chunks:                       []Chunk{},
			TotalFiles:                   0,
			TotalRawSize:                 0,
			TotalEstimatedCompressedSize: 0,
			OptimalChunkSizeMB:           0,
			Rationale:                    "No files to chunk",
		}, nil
	}

	// Step 1: Estimate compressed size for all files
	fileEstimates := make([]FileEstimate, 0, len(files))
	totalRawSize := int64(0)
	totalEstimatedCompressedSize := int64(0)

	for _, file := range files {
		compressedSize, err := cac.estimator.EstimateCompressedSize(file.Path, file.Size)
		if err != nil {
			// On error, assume no compression (use raw size)
			compressedSize = file.Size
		}

		fileEstimates = append(fileEstimates, FileEstimate{
			File:                file,
			EstimatedCompressed: compressedSize,
			CompressionRatio:    float64(compressedSize) / float64(file.Size),
		})

		totalRawSize += file.Size
		totalEstimatedCompressedSize += compressedSize
	}

	// Step 2: Calculate optimal chunk size with rationale
	optimalChunkSizeMB, rationale := cac.targetCalc.CalculateOptimalChunkSizeWithRationale(totalEstimatedCompressedSize)
	cac.mu.Lock()
	cac.lastChunkSize = optimalChunkSizeMB
	cac.mu.Unlock()

	targetChunkSizeBytes := int64(optimalChunkSizeMB) * 1024 * 1024

	// Step 3: Create chunks targeting optimal compressed size
	chunks := make([]Chunk, 0)
	chunkMetadata := make([]ChunkMetadata, 0)

	currentChunk := Chunk{
		ID:        0,
		Files:     make([]File, 0),
		TotalSize: 0,
		FileCount: 0,
	}
	currentEstimatedCompressed := int64(0)

	for _, estimate := range fileEstimates {
		// Check if adding this file would exceed target
		if currentEstimatedCompressed > 0 &&
			(currentEstimatedCompressed+estimate.EstimatedCompressed) > targetChunkSizeBytes {
			// Finalize current chunk
			chunks = append(chunks, currentChunk)
			chunkMetadata = append(chunkMetadata, ChunkMetadata{
				ChunkID:                 currentChunk.ID,
				FileCount:               currentChunk.FileCount,
				TotalRawSize:            currentChunk.TotalSize,
				EstimatedCompressedSize: currentEstimatedCompressed,
				CompressionRatio:        float64(currentEstimatedCompressed) / float64(currentChunk.TotalSize),
			})

			// Start new chunk
			currentChunk = Chunk{
				ID:        len(chunks),
				Files:     make([]File, 0),
				TotalSize: 0,
				FileCount: 0,
			}
			currentEstimatedCompressed = 0
		}

		// Add file to current chunk
		currentChunk.Files = append(currentChunk.Files, estimate.File)
		currentChunk.TotalSize += estimate.File.Size
		currentChunk.FileCount++
		currentEstimatedCompressed += estimate.EstimatedCompressed
	}

	// Add final chunk if not empty
	if currentChunk.FileCount > 0 {
		chunks = append(chunks, currentChunk)
		chunkMetadata = append(chunkMetadata, ChunkMetadata{
			ChunkID:                 currentChunk.ID,
			FileCount:               currentChunk.FileCount,
			TotalRawSize:            currentChunk.TotalSize,
			EstimatedCompressedSize: currentEstimatedCompressed,
			CompressionRatio:        float64(currentEstimatedCompressed) / float64(currentChunk.TotalSize),
		})
	}

	// Validate load balancing
	isBalanced, balanceMessage := cac.targetCalc.ValidateLoadBalancing(
		totalEstimatedCompressedSize,
		optimalChunkSizeMB,
	)

	return ChunkingResult{
		Chunks:                       chunks,
		ChunkMetadata:                chunkMetadata,
		TotalFiles:                   len(files),
		TotalRawSize:                 totalRawSize,
		TotalEstimatedCompressedSize: totalEstimatedCompressedSize,
		AverageCompressionRatio:      float64(totalEstimatedCompressedSize) / float64(totalRawSize),
		OptimalChunkSizeMB:           optimalChunkSizeMB,
		Rationale:                    rationale,
		LoadBalanced:                 isBalanced,
		LoadBalanceMessage:           balanceMessage,
	}, nil
}

// GetLastOptimalChunkSize returns the last calculated optimal chunk size (thread-safe)
func (cac *CompressedAwareChunker) GetLastOptimalChunkSize() int {
	cac.mu.RLock()
	defer cac.mu.RUnlock()
	return cac.lastChunkSize
}

// GetCompressionStats returns statistics from the compression estimator
func (cac *CompressedAwareChunker) GetCompressionStats() map[string]interface{} {
	return cac.estimator.GetCacheStats()
}

// ClearCompressionCache clears the compression ratio cache
func (cac *CompressedAwareChunker) ClearCompressionCache() {
	cac.estimator.ClearCache()
}

// FileEstimate represents estimated compression information for a file
type FileEstimate struct {
	File                File
	EstimatedCompressed int64
	CompressionRatio    float64
}

// ChunkMetadata contains detailed metadata about a chunk
type ChunkMetadata struct {
	ChunkID                 int
	FileCount               int
	TotalRawSize            int64
	EstimatedCompressedSize int64
	CompressionRatio        float64
}

// ChunkingResult contains the result of chunking with detailed metadata
type ChunkingResult struct {
	Chunks                       []Chunk
	ChunkMetadata                []ChunkMetadata
	TotalFiles                   int
	TotalRawSize                 int64
	TotalEstimatedCompressedSize int64
	AverageCompressionRatio      float64
	OptimalChunkSizeMB           int
	Rationale                    string
	LoadBalanced                 bool
	LoadBalanceMessage           string
}
