// Package chunking provides intelligent file chunking and sharding for CargoShip
package chunking

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
)

// ShardStrategy defines how files are distributed across shards
type ShardStrategy int

const (
	// ShardByHash distributes files evenly using consistent hashing
	ShardByHash ShardStrategy = iota
	// ShardBySize groups files by size for optimal compression
	ShardBySize
	// ShardByType groups files by content type (images, docs, code)
	ShardByType
	// ShardByDirectory keeps directory structures together
	ShardByDirectory
	// ShardAdaptive uses ML-based optimization (future)
	ShardAdaptive
)

// String returns the string representation of a ShardStrategy
func (s ShardStrategy) String() string {
	switch s {
	case ShardByHash:
		return "hash"
	case ShardBySize:
		return "size"
	case ShardByType:
		return "type"
	case ShardByDirectory:
		return "directory"
	case ShardAdaptive:
		return "adaptive"
	default:
		return "unknown"
	}
}

// ParseShardStrategy parses a string into a ShardStrategy
func ParseShardStrategy(s string) (ShardStrategy, error) {
	switch strings.ToLower(s) {
	case "hash":
		return ShardByHash, nil
	case "size":
		return ShardBySize, nil
	case "type":
		return ShardByType, nil
	case "directory":
		return ShardByDirectory, nil
	case "adaptive":
		return ShardAdaptive, nil
	default:
		return ShardByHash, fmt.Errorf("unknown shard strategy: %s", s)
	}
}

// ShardRouterConfig configures the shard router
type ShardRouterConfig struct {
	// Strategy determines how files are distributed across shards
	Strategy ShardStrategy

	// ShardCount is the number of shards to create (default: 10)
	ShardCount int

	// SmallFileSizeThreshold defines "small" files for size-based routing (default: 1MB)
	SmallFileSizeThreshold int64

	// MediumFileSizeThreshold defines "medium" files for size-based routing (default: 100MB)
	MediumFileSizeThreshold int64
}

// DefaultShardRouterConfig returns a configuration with sensible defaults
func DefaultShardRouterConfig() *ShardRouterConfig {
	return &ShardRouterConfig{
		Strategy:                ShardByHash,
		ShardCount:              10,
		SmallFileSizeThreshold:  1 << 20,   // 1MB
		MediumFileSizeThreshold: 100 << 20, // 100MB
	}
}

// ShardRouter routes files to shards based on the configured strategy
type ShardRouter struct {
	config *ShardRouterConfig
}

// NewShardRouter creates a new shard router with the given configuration
func NewShardRouter(config *ShardRouterConfig) (*ShardRouter, error) {
	if config == nil {
		config = DefaultShardRouterConfig()
	}

	if config.ShardCount <= 0 {
		return nil, fmt.Errorf("shard count must be positive, got %d", config.ShardCount)
	}

	if config.ShardCount > 1000 {
		return nil, fmt.Errorf("shard count too large (max 1000), got %d", config.ShardCount)
	}

	return &ShardRouter{
		config: config,
	}, nil
}

// Route determines which shard a file should be assigned to (0-indexed)
func (r *ShardRouter) Route(file File) int {
	switch r.config.Strategy {
	case ShardByHash:
		return r.routeByHash(file)
	case ShardBySize:
		return r.routeBySize(file)
	case ShardByType:
		return r.routeByType(file)
	case ShardByDirectory:
		return r.routeByDirectory(file)
	case ShardAdaptive:
		// For now, fall back to hash-based
		// TODO: Implement ML-based adaptive routing
		return r.routeByHash(file)
	default:
		return r.routeByHash(file)
	}
}

// routeByHash uses consistent hashing for even distribution (FNV-1a)
func (r *ShardRouter) routeByHash(file File) int {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(file.Path))
	return int(hash.Sum64() % uint64(r.config.ShardCount))
}

// routeBySize groups files by size (small/medium/large buckets)
func (r *ShardRouter) routeBySize(file File) int {
	if file.Size < r.config.SmallFileSizeThreshold {
		// Small files go to shard 0
		return 0
	} else if file.Size < r.config.MediumFileSizeThreshold {
		// Medium files go to shard 1
		return 1
	} else {
		// Large files distributed across remaining shards
		if r.config.ShardCount <= 2 {
			return r.config.ShardCount - 1
		}
		// Distribute large files evenly across shards 2+
		remainingShards := r.config.ShardCount - 2
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(file.Path))
		return 2 + int(hash.Sum64()%uint64(remainingShards))
	}
}

// routeByType routes files by content type (images, docs, code, binary)
func (r *ShardRouter) routeByType(file File) int {
	ext := strings.ToLower(filepath.Ext(file.Path))
	contentType := classifyContentType(ext)

	// Map content types to shard ranges
	switch contentType {
	case contentTypeImage:
		// Images: shard 0 (already compressed, use low compression)
		return 0
	case contentTypeDocument:
		// Documents: shard 1 (good compression potential)
		return 1
	case contentTypeCode:
		// Code: shard 2 (excellent compression potential)
		return 2
	case contentTypeBinary:
		// Binary: shard 3 (fast compression)
		return 3
	case contentTypeArchive:
		// Archives: shard 4 (already compressed, no recompression)
		return 4
	default:
		// Unknown types: distribute across remaining shards
		if r.config.ShardCount <= 5 {
			return r.config.ShardCount - 1
		}
		remainingShards := r.config.ShardCount - 5
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(file.Path))
		return 5 + int(hash.Sum64()%uint64(remainingShards))
	}
}

// routeByDirectory keeps directory structures together
func (r *ShardRouter) routeByDirectory(file File) int {
	dirPath := filepath.Dir(file.Path)
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(dirPath))
	return int(hash.Sum64() % uint64(r.config.ShardCount))
}

// contentType represents different file content categories
type contentType int

const (
	contentTypeUnknown contentType = iota
	contentTypeImage
	contentTypeDocument
	contentTypeCode
	contentTypeBinary
	contentTypeArchive
)

// classifyContentType determines the content type from file extension
func classifyContentType(ext string) contentType {
	switch ext {
	// Images (already compressed)
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".webp", ".heic", ".svg":
		return contentTypeImage

	// Documents (good compression)
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp",
		".txt", ".md", ".rtf", ".csv", ".tsv", ".log":
		return contentTypeDocument

	// Code (excellent compression)
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".hpp", ".rs", ".rb",
		".php", ".swift", ".kt", ".scala", ".sh", ".bash", ".zsh", ".pl", ".r", ".m",
		".html", ".css", ".scss", ".sass", ".less", ".xml", ".json", ".yaml", ".yml",
		".toml", ".ini", ".conf", ".cfg":
		return contentTypeCode

	// Binary executables and libraries (mixed compression)
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".obj", ".class", ".pyc",
		".jar", ".war", ".ear":
		return contentTypeBinary

	// Already compressed archives (no recompression)
	case ".zip", ".gz", ".bz2", ".xz", ".7z", ".rar", ".tar", ".tgz", ".tbz2",
		".zst", ".lz4", ".lzma":
		return contentTypeArchive

	default:
		return contentTypeUnknown
	}
}

// GetShardCount returns the configured number of shards
func (r *ShardRouter) GetShardCount() int {
	return r.config.ShardCount
}

// GetStrategy returns the configured routing strategy
func (r *ShardRouter) GetStrategy() ShardStrategy {
	return r.config.Strategy
}

// ShardDistribution represents statistics about file distribution across shards
type ShardDistribution struct {
	TotalFiles     int     // Total number of files
	TotalSize      int64   // Total size in bytes
	ShardCount     int     // Number of shards
	FileCounts     []int   // Files per shard
	Sizes          []int64 // Size per shard (bytes)
	MinFiles       int     // Minimum files in any shard
	MaxFiles       int     // Maximum files in any shard
	AvgFiles       float64 // Average files per shard
	Variance       float64 // Variance in file count distribution
	BalanceQuality float64 // Balance quality score (0-1, 1 = perfect balance)
}

// AnalyzeDistribution analyzes the distribution of files across shards
func (r *ShardRouter) AnalyzeDistribution(files []File) *ShardDistribution {
	dist := &ShardDistribution{
		TotalFiles: len(files),
		ShardCount: r.config.ShardCount,
		FileCounts: make([]int, r.config.ShardCount),
		Sizes:      make([]int64, r.config.ShardCount),
	}

	// Count files and sizes per shard
	for _, file := range files {
		shardID := r.Route(file)
		dist.FileCounts[shardID]++
		dist.Sizes[shardID] += file.Size
		dist.TotalSize += file.Size
	}

	// Calculate statistics
	if len(files) == 0 {
		return dist
	}

	dist.MinFiles = dist.FileCounts[0]
	dist.MaxFiles = dist.FileCounts[0]
	sum := 0
	for _, count := range dist.FileCounts {
		if count < dist.MinFiles {
			dist.MinFiles = count
		}
		if count > dist.MaxFiles {
			dist.MaxFiles = count
		}
		sum += count
	}
	dist.AvgFiles = float64(sum) / float64(r.config.ShardCount)

	// Calculate variance
	var sumSquaredDiff float64
	for _, count := range dist.FileCounts {
		diff := float64(count) - dist.AvgFiles
		sumSquaredDiff += diff * diff
	}
	dist.Variance = sumSquaredDiff / float64(r.config.ShardCount)

	// Calculate balance quality (inverse of coefficient of variation)
	// Perfect balance = 1.0, poor balance = closer to 0
	if dist.AvgFiles > 0 {
		cv := (dist.Variance / (dist.AvgFiles * dist.AvgFiles)) // Coefficient of variation squared
		dist.BalanceQuality = 1.0 / (1.0 + cv)
	}

	return dist
}
