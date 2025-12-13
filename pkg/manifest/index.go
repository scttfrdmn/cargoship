// Package manifest provides fast in-memory indexing for large manifests (Issue #89)
package manifest

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ManifestIndex provides fast O(1) lookups for large manifests
// Optimized for memory efficiency and lookup speed
type ManifestIndex struct {
	// Primary path index: path -> file entry reference
	pathIndex map[string]*FileEntry

	// Shard index: shard ID -> sorted file entries
	shardIndex map[int][]*FileEntry

	// Extension index: extension -> file entries
	extIndex map[string][]*FileEntry

	// Directory index: directory -> file entries
	dirIndex map[string][]*FileEntry

	// Statistics
	stats IndexStats

	// Reference to original manifest
	manifest *Manifest
}

// IndexStats tracks index size and performance metrics
type IndexStats struct {
	TotalFiles      int
	TotalShards     int
	IndexSizeBytes  int64
	BuildTimeMs     int64
	AvgLookupTimeNs int64
}

// IndexOptions configures index building behavior
type IndexOptions struct {
	// EnableExtensionIndex builds extension-based index
	EnableExtensionIndex bool

	// EnableDirectoryIndex builds directory-based index
	EnableDirectoryIndex bool

	// EnableShardIndex builds shard-based sorted index
	EnableShardIndex bool

	// MemoryLimit sets approximate memory limit in bytes (0 = unlimited)
	MemoryLimit int64
}

// DefaultIndexOptions returns recommended index options
func DefaultIndexOptions() *IndexOptions {
	return &IndexOptions{
		EnableExtensionIndex: true,
		EnableDirectoryIndex: true,
		EnableShardIndex:     true,
		MemoryLimit:          0, // No limit by default
	}
}

// NewManifestIndex creates a new fast index from a manifest
func NewManifestIndex(m *Manifest, opts *IndexOptions) (*ManifestIndex, error) {
	if opts == nil {
		opts = DefaultIndexOptions()
	}

	startTime := time.Now()

	idx := &ManifestIndex{
		pathIndex:  make(map[string]*FileEntry, len(m.Files)),
		manifest:   m,
	}

	// Build primary path index (always enabled - O(1) lookups)
	for i := range m.Files {
		idx.pathIndex[m.Files[i].Path] = &m.Files[i]
	}

	// Build optional indexes
	if opts.EnableShardIndex {
		idx.buildShardIndex()
	}

	if opts.EnableExtensionIndex {
		idx.buildExtensionIndex()
	}

	if opts.EnableDirectoryIndex {
		idx.buildDirectoryIndex()
	}

	// Calculate statistics
	buildTime := time.Since(startTime)
	idx.stats = IndexStats{
		TotalFiles:     len(m.Files),
		TotalShards:    len(m.Shards),
		IndexSizeBytes: idx.estimateMemoryUsage(),
		BuildTimeMs:    buildTime.Milliseconds(),
	}

	return idx, nil
}

// FindFile performs O(1) lookup by exact path
func (idx *ManifestIndex) FindFile(path string) *FileEntry {
	return idx.pathIndex[path]
}

// FindFilesByShard returns all files in a shard (sorted by path if shard index enabled)
func (idx *ManifestIndex) FindFilesByShard(shardID int) []*FileEntry {
	if idx.shardIndex != nil {
		return idx.shardIndex[shardID]
	}

	// Fallback: scan all files
	var files []*FileEntry
	for _, file := range idx.pathIndex {
		if file.ShardID == shardID {
			files = append(files, file)
		}
	}
	return files
}

// FindFilesByExtension returns all files with given extension
func (idx *ManifestIndex) FindFilesByExtension(ext string) []*FileEntry {
	if idx.extIndex != nil {
		return idx.extIndex[ext]
	}

	// Fallback: scan all files
	var files []*FileEntry
	for _, file := range idx.pathIndex {
		if strings.HasSuffix(file.Path, ext) {
			files = append(files, file)
		}
	}
	return files
}

// FindFilesByDirectory returns all files in a directory (non-recursive)
func (idx *ManifestIndex) FindFilesByDirectory(dir string) []*FileEntry {
	if idx.dirIndex != nil {
		return idx.dirIndex[dir]
	}

	// Fallback: scan all files
	var files []*FileEntry
	dirPrefix := dir + "/"
	for _, file := range idx.pathIndex {
		if strings.HasPrefix(file.Path, dirPrefix) {
			// Check it's in this directory (not subdirectory)
			remaining := strings.TrimPrefix(file.Path, dirPrefix)
			if !strings.Contains(remaining, "/") {
				files = append(files, file)
			}
		}
	}
	return files
}

// FindFilesByPrefix returns all files with given path prefix
// Uses efficient prefix scan with early termination
func (idx *ManifestIndex) FindFilesByPrefix(prefix string) []*FileEntry {
	var files []*FileEntry

	// If shard index exists, we can be more efficient
	if idx.shardIndex != nil {
		// Search within each shard's sorted list
		for _, shardFiles := range idx.shardIndex {
			// Binary search to find first matching entry
			start := sort.Search(len(shardFiles), func(i int) bool {
				return shardFiles[i].Path >= prefix
			})

			// Collect all matching entries
			for i := start; i < len(shardFiles); i++ {
				if strings.HasPrefix(shardFiles[i].Path, prefix) {
					files = append(files, shardFiles[i])
				} else {
					break // Stop when prefix no longer matches
				}
			}
		}
	} else {
		// Fallback: scan all files
		for _, file := range idx.pathIndex {
			if strings.HasPrefix(file.Path, prefix) {
				files = append(files, file)
			}
		}
	}

	return files
}

// FindFilesBySuffix returns all files with given path suffix
func (idx *ManifestIndex) FindFilesBySuffix(suffix string) []*FileEntry {
	var files []*FileEntry

	for _, file := range idx.pathIndex {
		if strings.HasSuffix(file.Path, suffix) {
			files = append(files, file)
		}
	}

	return files
}

// FindFilesByPattern returns all files matching a simple pattern
// Supports * wildcard and case-insensitive matching
func (idx *ManifestIndex) FindFilesByPattern(pattern string, caseSensitive bool) []*FileEntry {
	var files []*FileEntry

	if !caseSensitive {
		pattern = strings.ToLower(pattern)
	}

	for _, file := range idx.pathIndex {
		path := file.Path
		if !caseSensitive {
			path = strings.ToLower(path)
		}

		if matchPattern(path, pattern) {
			files = append(files, file)
		}
	}

	return files
}

// GetStats returns index statistics
func (idx *ManifestIndex) GetStats() IndexStats {
	return idx.stats
}

// EstimateMemoryUsage estimates current memory usage in bytes
func (idx *ManifestIndex) estimateMemoryUsage() int64 {
	var total int64

	// Path index: map overhead + keys + pointers
	total += int64(len(idx.pathIndex) * (16 + 64 + 8)) // map entry + avg key size + pointer

	// Shard index
	if idx.shardIndex != nil {
		total += int64(len(idx.shardIndex) * (16 + 8)) // map entry + pointer
		for _, files := range idx.shardIndex {
			total += int64(len(files) * 8) // pointers
		}
	}

	// Extension index
	if idx.extIndex != nil {
		total += int64(len(idx.extIndex) * (16 + 8 + 8)) // map entry + key + pointer
		for _, files := range idx.extIndex {
			total += int64(len(files) * 8) // pointers
		}
	}

	// Directory index
	if idx.dirIndex != nil {
		total += int64(len(idx.dirIndex) * (16 + 32 + 8)) // map entry + avg key + pointer
		for _, files := range idx.dirIndex {
			total += int64(len(files) * 8) // pointers
		}
	}

	return total
}

// buildShardIndex builds sorted index per shard
func (idx *ManifestIndex) buildShardIndex() {
	idx.shardIndex = make(map[int][]*FileEntry)

	// Group files by shard
	for _, file := range idx.pathIndex {
		idx.shardIndex[file.ShardID] = append(idx.shardIndex[file.ShardID], file)
	}

	// Sort each shard's files by path for binary search
	for shardID := range idx.shardIndex {
		sort.Slice(idx.shardIndex[shardID], func(i, j int) bool {
			return idx.shardIndex[shardID][i].Path < idx.shardIndex[shardID][j].Path
		})
	}
}

// buildExtensionIndex builds extension-based index
func (idx *ManifestIndex) buildExtensionIndex() {
	idx.extIndex = make(map[string][]*FileEntry)

	for _, file := range idx.pathIndex {
		// Extract extension (including the dot)
		ext := getExtension(file.Path)
		if ext != "" {
			idx.extIndex[ext] = append(idx.extIndex[ext], file)
		}
	}
}

// buildDirectoryIndex builds directory-based index
func (idx *ManifestIndex) buildDirectoryIndex() {
	idx.dirIndex = make(map[string][]*FileEntry)

	for _, file := range idx.pathIndex {
		dir := getDirectory(file.Path)
		idx.dirIndex[dir] = append(idx.dirIndex[dir], file)
	}
}

// Helper functions

func getExtension(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	lastDot := strings.LastIndex(path, ".")

	if lastDot > lastSlash && lastDot < len(path)-1 {
		return path[lastDot:]
	}

	return ""
}

func getDirectory(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash > 0 {
		return path[:lastSlash]
	}
	return "."
}

func matchPattern(text, pattern string) bool {
	// Simple wildcard matching (* = any characters)
	parts := strings.Split(pattern, "*")

	if len(parts) == 1 {
		// No wildcards - exact match
		return text == pattern
	}

	// Check if text starts with first part
	if !strings.HasPrefix(text, parts[0]) {
		return false
	}

	// Check if text ends with last part
	if parts[len(parts)-1] != "" && !strings.HasSuffix(text, parts[len(parts)-1]) {
		return false
	}

	// Check middle parts in order
	currentPos := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}

		idx := strings.Index(text[currentPos:], parts[i])
		if idx == -1 {
			return false
		}
		currentPos += idx + len(parts[i])
	}

	return true
}

// CompactIndex reduces memory footprint by rebuilding with only essential indexes
func (idx *ManifestIndex) CompactIndex() error {
	// Rebuild with minimal indexes
	opts := &IndexOptions{
		EnableExtensionIndex: false,
		EnableDirectoryIndex: false,
		EnableShardIndex:     false,
	}

	newIdx, err := NewManifestIndex(idx.manifest, opts)
	if err != nil {
		return fmt.Errorf("failed to compact index: %w", err)
	}

	// Replace current index with compact version
	idx.pathIndex = newIdx.pathIndex
	idx.shardIndex = nil
	idx.extIndex = nil
	idx.dirIndex = nil
	idx.stats = newIdx.stats

	return nil
}

// Validate checks index integrity
func (idx *ManifestIndex) Validate() error {
	// Check that all manifest files are indexed
	if len(idx.pathIndex) != len(idx.manifest.Files) {
		return fmt.Errorf("index size mismatch: %d indexed vs %d in manifest",
			len(idx.pathIndex), len(idx.manifest.Files))
	}

	// Verify no duplicate paths
	seen := make(map[string]bool)
	for path := range idx.pathIndex {
		if seen[path] {
			return fmt.Errorf("duplicate path in index: %s", path)
		}
		seen[path] = true
	}

	return nil
}
