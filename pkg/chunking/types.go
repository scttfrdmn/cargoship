package chunking

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// File represents a file to be chunked and archived
type File struct {
	Path      string            // Full path to the file
	Size      int64             // Size in bytes
	ModTime   time.Time         // Last modification time
	Directory string            // Parent directory
	Metadata  map[string]string // Additional metadata

	// File splitting support (Phase 5)
	Offset     int64 // Start offset for partial read (0 = full file)
	Length     int64 // Length to read (0 = read Size bytes from Offset)
	PartIndex  int   // Part index for split files (0 = not split or first part)
	TotalParts int   // Total parts if file is split (0 or 1 = not split)
}

// Chunk represents a group of files to be archived together
type Chunk struct {
	ID              int                  // Chunk identifier
	Files           []File               // Files in this chunk
	TotalSize       int64                // Total size of all files in bytes
	FileCount       int                  // Number of files
	EstimatedOps    int                  // Estimated S3 operations (multipart uploads)
	PreAssignedTier types.StorageClass   // Issue #164: Pre-assigned storage tier (empty = use default/youngest-file)
}

// ChunkStats provides statistics about chunking decisions
type ChunkStats struct {
	TotalFiles       int           // Total number of files
	TotalSize        int64         // Total size in bytes
	ChunkCount       int           // Number of chunks created
	AverageChunkSize int64         // Average chunk size in bytes
	MinChunkSize     int64         // Smallest chunk size in bytes
	MaxChunkSize     int64         // Largest chunk size in bytes
	EstimatedOps     int           // Total estimated S3 operations
	CostSavings      float64       // Estimated cost savings ratio (e.g., 1000 = 1000x savings)
	MemoryRequired   int64         // Peak memory required for processing
	EstimatedTime    time.Duration // Estimated processing time
}

// ChunkingConfig contains configuration for the chunking strategy
type ChunkingConfig struct {
	// Target chunk size in bytes (0 = auto-calculate)
	TargetChunkSize int64

	// Available memory for chunking operations (bytes)
	AvailableMemory int64

	// Number of parallel workers
	Workers int

	// Cost savings target (e.g., 1000 = aim for 1000x fewer operations)
	CostSavingsTarget float64

	// Minimum chunk size (bytes) - default 10MB
	MinChunkSize int64

	// Maximum chunk size (bytes) - default 5GB (S3 limit)
	MaxChunkSize int64

	// Grouping strategy: "size", "directory", "mixed"
	GroupingStrategy string

	// Bandwidth estimate for performance calculations (bytes/sec)
	Bandwidth int64

	// LargeFileThreshold defines the size threshold for "large" vs "small" files
	// in mixed grouping strategy (default: 100MB)
	LargeFileThreshold int64

	// MultipartPartSize defines the part size for S3 multipart upload operations
	// estimation (default: 100MB)
	MultipartPartSize int64

	// EnableFileSplitting enables splitting large files across multiple chunks (Phase 5)
	// When true, files larger than chunk size will be split into multiple parts
	EnableFileSplitting bool

	// MaxFileChunkSize defines the maximum size for a single file part when splitting
	// If 0, uses the calculated chunk size (default behavior)
	MaxFileChunkSize int64

	// Issue #164: Tier-aware chunking configuration
	// EnableTierAwareChunking groups files by storage tier before chunking
	// This results in homogeneous tier assignment per chunk, improving cost optimization
	EnableTierAwareChunking bool

	// TierGroupBufferSize limits the number of files buffered per tier group
	// This prevents excessive memory usage when tier-aware chunking is enabled
	// Default: 100000 files per tier
	TierGroupBufferSize int
}

// ChunkingStrategy defines the interface for chunking algorithms
type ChunkingStrategy interface {
	// CalculateOptimalChunkSize determines the optimal chunk size based on constraints
	CalculateOptimalChunkSize(
		totalSize int64,
		fileCount int,
		availableMemory int64,
		costSavingsTarget float64,
	) (chunkSize int64, stats ChunkStats)

	// GroupFilesIntoChunks groups files into chunks based on the strategy
	GroupFilesIntoChunks(
		files []File,
		chunkSize int64,
	) ([]Chunk, error)
}

// ConstraintType represents different optimization constraints
type ConstraintType int

const (
	ConstraintMemory      ConstraintType = iota // Memory-constrained optimization
	ConstraintCost                              // Cost-optimization (minimize S3 operations)
	ConstraintPerformance                       // Performance-optimization (maximize throughput)
	ConstraintAdaptive                          // Adaptive (balance all constraints)
)

// Constraint represents a single optimization constraint
type Constraint struct {
	Type   ConstraintType
	Value  int64   // Constraint value (e.g., memory limit in bytes)
	Weight float64 // Weight for multi-objective optimization (0.0-1.0)
}
