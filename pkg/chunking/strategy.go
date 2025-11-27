package chunking

import (
	"fmt"
	"sort"
)

// AdaptiveChunkingStrategy implements an adaptive chunking strategy
// that balances size, directory structure, and performance
type AdaptiveChunkingStrategy struct {
	calculator *ChunkSizeCalculator
	config     *ChunkingConfig
}

// NewAdaptiveChunkingStrategy creates a new adaptive chunking strategy
func NewAdaptiveChunkingStrategy(config *ChunkingConfig) *AdaptiveChunkingStrategy {
	return &AdaptiveChunkingStrategy{
		calculator: NewChunkSizeCalculator(config),
		config:     config,
	}
}

// CalculateOptimalChunkSize determines the optimal chunk size
func (s *AdaptiveChunkingStrategy) CalculateOptimalChunkSize(
	totalSize int64,
	fileCount int,
	availableMemory int64,
	costSavingsTarget float64,
) (int64, ChunkStats) {
	return s.calculator.CalculateOptimalChunkSize(
		totalSize,
		fileCount,
		availableMemory,
		costSavingsTarget,
	)
}

// GroupFilesIntoChunks groups files into chunks using an efficient greedy algorithm
// Target performance: >10,000 files/sec
func (s *AdaptiveChunkingStrategy) GroupFilesIntoChunks(
	files []File,
	chunkSize int64,
) ([]Chunk, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to chunk")
	}

	if chunkSize <= 0 {
		return nil, fmt.Errorf("invalid chunk size: %d", chunkSize)
	}

	// Select grouping strategy
	switch s.config.GroupingStrategy {
	case "size":
		return s.groupBySize(files, chunkSize)
	case "directory":
		return s.groupByDirectory(files, chunkSize)
	case "mixed", "":
		return s.groupMixed(files, chunkSize)
	default:
		return nil, fmt.Errorf("unknown grouping strategy: %s", s.config.GroupingStrategy)
	}
}

// groupBySize groups files purely by size using a greedy first-fit algorithm
// This is the fastest approach for maximum throughput
func (s *AdaptiveChunkingStrategy) groupBySize(files []File, chunkSize int64) ([]Chunk, error) {
	chunks := []Chunk{}
	currentChunk := Chunk{ID: 0}

	for i := range files {
		file := files[i]

		// If adding this file would exceed chunk size, start a new chunk
		if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = Chunk{
				ID: len(chunks),
			}
		}

		// Add file to current chunk
		currentChunk.Files = append(currentChunk.Files, file)
		currentChunk.TotalSize += file.Size
		currentChunk.FileCount++
	}

	// Add the last chunk if it has files
	if len(currentChunk.Files) > 0 {
		chunks = append(chunks, currentChunk)
	}

	// Calculate estimated operations for each chunk
	for i := range chunks {
		chunks[i].EstimatedOps = s.estimateS3Operations(chunks[i].TotalSize)
	}

	return chunks, nil
}

// groupByDirectory groups files by directory structure, then by size
// This preserves locality and can improve compression ratios
func (s *AdaptiveChunkingStrategy) groupByDirectory(files []File, chunkSize int64) ([]Chunk, error) {
	// Group files by directory
	dirMap := make(map[string][]File)
	for i := range files {
		dir := files[i].Directory
		if dir == "" {
			dir = "/"
		}
		dirMap[dir] = append(dirMap[dir], files[i])
	}

	// Create chunks directory by directory
	chunks := []Chunk{}
	currentChunk := Chunk{ID: 0}

	for _, dirFiles := range dirMap {
		for i := range dirFiles {
			file := dirFiles[i]

			// If adding this file would exceed chunk size, start a new chunk
			if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = Chunk{
					ID: len(chunks),
				}
			}

			// Add file to current chunk
			currentChunk.Files = append(currentChunk.Files, file)
			currentChunk.TotalSize += file.Size
			currentChunk.FileCount++
		}
	}

	// Add the last chunk if it has files
	if len(currentChunk.Files) > 0 {
		chunks = append(chunks, currentChunk)
	}

	// Calculate estimated operations
	for i := range chunks {
		chunks[i].EstimatedOps = s.estimateS3Operations(chunks[i].TotalSize)
	}

	return chunks, nil
}

// groupMixed uses a mixed strategy: small files by directory, large files individually
// Threshold: files > LargeFileThreshold are chunked individually, smaller files grouped by directory
func (s *AdaptiveChunkingStrategy) groupMixed(files []File, chunkSize int64) ([]Chunk, error) {
	// Use configurable threshold, default to 100MB if not set
	largeFileThreshold := s.config.LargeFileThreshold
	if largeFileThreshold == 0 {
		largeFileThreshold = 100 * 1024 * 1024 // 100 MB default
	}

	// Separate large and small files
	var largeFiles []File
	var smallFiles []File

	for i := range files {
		if files[i].Size > largeFileThreshold {
			largeFiles = append(largeFiles, files[i])
		} else {
			smallFiles = append(smallFiles, files[i])
		}
	}

	chunks := []Chunk{}
	chunkID := 0

	// Process large files first - pack multiple files into chunks until size limit
	if len(largeFiles) > 0 {
		currentChunk := Chunk{ID: chunkID}

		for i := range largeFiles {
			file := largeFiles[i]

			if file.Size > chunkSize {
				// File is larger than chunk size
				// Finish current chunk if it has files
				if len(currentChunk.Files) > 0 {
					currentChunk.EstimatedOps = s.estimateS3Operations(currentChunk.TotalSize)
					chunks = append(chunks, currentChunk)
					chunkID++
				}

				// Phase 5: Check if file splitting is enabled
				if s.config.EnableFileSplitting {
					// Split file into multiple chunks
					splitChunkSize := chunkSize
					if s.config.MaxFileChunkSize > 0 && s.config.MaxFileChunkSize < chunkSize {
						splitChunkSize = s.config.MaxFileChunkSize
					}

					numParts := (file.Size + splitChunkSize - 1) / splitChunkSize
					for partIdx := 0; partIdx < int(numParts); partIdx++ {
						offset := int64(partIdx) * splitChunkSize
						length := splitChunkSize
						if offset+length > file.Size {
							length = file.Size - offset
						}

						// Create partial file entry
						partialFile := File{
							Path:       file.Path,
							Size:       length, // Size represents the length to read
							ModTime:    file.ModTime,
							Directory:  file.Directory,
							Metadata:   file.Metadata,
							Offset:     offset,
							Length:     length,
							PartIndex:  partIdx,
							TotalParts: int(numParts),
						}

						// Create chunk for this part
						chunk := Chunk{
							ID:           chunkID,
							Files:        []File{partialFile},
							TotalSize:    length,
							FileCount:    1,
							EstimatedOps: s.estimateS3Operations(length),
						}
						chunks = append(chunks, chunk)
						chunkID++
					}
					currentChunk = Chunk{ID: chunkID}
				} else {
					// Original behavior: Create dedicated chunk for oversized file
					chunk := Chunk{
						ID:           chunkID,
						Files:        []File{file},
						TotalSize:    file.Size,
						FileCount:    1,
						EstimatedOps: s.estimateS3Operations(file.Size),
					}
					chunks = append(chunks, chunk)
					chunkID++
					currentChunk = Chunk{ID: chunkID}
				}
			} else {
				// File fits in chunk - try to pack it with others
				if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
					// Current chunk would overflow, finish it
					currentChunk.EstimatedOps = s.estimateS3Operations(currentChunk.TotalSize)
					chunks = append(chunks, currentChunk)
					chunkID++
					currentChunk = Chunk{ID: chunkID}
				}

				// Add file to current chunk
				currentChunk.Files = append(currentChunk.Files, file)
				currentChunk.TotalSize += file.Size
				currentChunk.FileCount++
			}
		}

		// Add final large file chunk if it has files
		if len(currentChunk.Files) > 0 {
			currentChunk.EstimatedOps = s.estimateS3Operations(currentChunk.TotalSize)
			chunks = append(chunks, currentChunk)
			chunkID++
		}
	}

	// Process small files grouped by directory
	if len(smallFiles) > 0 {
		smallChunks, err := s.groupByDirectory(smallFiles, chunkSize)
		if err != nil {
			return nil, err
		}

		// Renumber chunk IDs
		for i := range smallChunks {
			smallChunks[i].ID = chunkID
			chunkID++
		}

		chunks = append(chunks, smallChunks...)
	}

	return chunks, nil
}

// estimateS3Operations estimates the number of S3 operations for a given size
func (s *AdaptiveChunkingStrategy) estimateS3Operations(size int64) int {
	// For chunks < 5GB, assume single put operation
	if size < DefaultMaxChunkSize {
		return 1
	}

	// For larger chunks, estimate multipart upload operations
	// Use configurable part size, default to 100MB if not set
	partSize := s.config.MultipartPartSize
	if partSize == 0 {
		partSize = 100 * 1024 * 1024 // 100MB default
	}
	parts := (size + partSize - 1) / partSize // Ceiling division
	return int(parts)
}

// SizeBasedChunkingStrategy implements a simple size-based chunking strategy
// This is the fastest strategy for pure performance
type SizeBasedChunkingStrategy struct {
	calculator *ChunkSizeCalculator
	config     *ChunkingConfig
}

// NewSizeBasedChunkingStrategy creates a new size-based strategy
func NewSizeBasedChunkingStrategy(config *ChunkingConfig) *SizeBasedChunkingStrategy {
	return &SizeBasedChunkingStrategy{
		calculator: NewChunkSizeCalculator(config),
		config:     config,
	}
}

// CalculateOptimalChunkSize determines the optimal chunk size
func (s *SizeBasedChunkingStrategy) CalculateOptimalChunkSize(
	totalSize int64,
	fileCount int,
	availableMemory int64,
	costSavingsTarget float64,
) (int64, ChunkStats) {
	return s.calculator.CalculateOptimalChunkSize(
		totalSize,
		fileCount,
		availableMemory,
		costSavingsTarget,
	)
}

// GroupFilesIntoChunks groups files purely by size for maximum performance
func (s *SizeBasedChunkingStrategy) GroupFilesIntoChunks(
	files []File,
	chunkSize int64,
) ([]Chunk, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to chunk")
	}

	if chunkSize <= 0 {
		return nil, fmt.Errorf("invalid chunk size: %d", chunkSize)
	}

	// Optional: Sort files by size (largest first) for better packing
	// This is disabled by default for maximum speed
	if s.config.GroupingStrategy == "size-sorted" {
		sort.Slice(files, func(i, j int) bool {
			return files[i].Size > files[j].Size
		})
	}

	chunks := []Chunk{}
	currentChunk := Chunk{ID: 0}

	for i := range files {
		file := files[i]

		// Start new chunk if adding this file would exceed size
		if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
			currentChunk.EstimatedOps = s.estimateS3Operations(currentChunk.TotalSize)
			chunks = append(chunks, currentChunk)
			currentChunk = Chunk{ID: len(chunks)}
		}

		// Add file to current chunk
		currentChunk.Files = append(currentChunk.Files, file)
		currentChunk.TotalSize += file.Size
		currentChunk.FileCount++
	}

	// Add final chunk
	if len(currentChunk.Files) > 0 {
		currentChunk.EstimatedOps = s.estimateS3Operations(currentChunk.TotalSize)
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// estimateS3Operations estimates the number of S3 operations for a given size
func (s *SizeBasedChunkingStrategy) estimateS3Operations(size int64) int {
	// For chunks < 5GB, assume single put operation
	if size < DefaultMaxChunkSize {
		return 1
	}

	// For larger chunks, estimate multipart upload operations
	// Use configurable part size, default to 100MB if not set
	partSize := s.config.MultipartPartSize
	if partSize == 0 {
		partSize = 100 * 1024 * 1024 // 100MB default
	}
	parts := (size + partSize - 1) / partSize // Ceiling division
	return int(parts)
}
