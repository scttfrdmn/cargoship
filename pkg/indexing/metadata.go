// Package indexing provides enhanced metadata and search capabilities for CargoShip archives
package indexing

import (
	"fmt"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

// EnhancedFile extends the existing inventory.File with enhanced metadata for v0.4.2
type EnhancedFile struct {
	inventory.File                    // Embed existing File structure
	StorageClass    string            `yaml:"storage_class" json:"storage_class"`
	LastAccessed    *time.Time        `yaml:"last_accessed,omitempty" json:"last_accessed,omitempty"`
	ContentType     string            `yaml:"content_type,omitempty" json:"content_type,omitempty"`
	Tags            map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Checksum        string            `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	ChecksumType    string            `yaml:"checksum_type,omitempty" json:"checksum_type,omitempty"`
	CompressionInfo CompressionInfo   `yaml:"compression,omitempty" json:"compression,omitempty"`
	S3Metadata      *S3ObjectMetadata `yaml:"s3_metadata,omitempty" json:"s3_metadata,omitempty"`
	CreatedAt       time.Time         `yaml:"created_at" json:"created_at"`
	ModifiedAt      time.Time         `yaml:"modified_at" json:"modified_at"`
	ArchivedAt      *time.Time        `yaml:"archived_at,omitempty" json:"archived_at,omitempty"`
}

// CompressionInfo provides detailed compression statistics
type CompressionInfo struct {
	Algorithm        string  `yaml:"algorithm" json:"algorithm"`                 // e.g., "zstd", "gzip", "bzip2"
	OriginalSize     int64   `yaml:"original_size" json:"original_size"`         // Size before compression
	CompressedSize   int64   `yaml:"compressed_size" json:"compressed_size"`     // Size after compression
	CompressionRatio float64 `yaml:"compression_ratio" json:"compression_ratio"` // Ratio (compressed/original)
	Level            int     `yaml:"level,omitempty" json:"level,omitempty"`     // Compression level used
}

// S3ObjectMetadata provides S3-specific metadata
type S3ObjectMetadata struct {
	ETag                 string            `yaml:"etag" json:"etag"`
	Bucket               string            `yaml:"bucket" json:"bucket"`
	Key                  string            `yaml:"key" json:"key"`
	StorageClass         string            `yaml:"storage_class" json:"storage_class"`
	LastModified         time.Time         `yaml:"last_modified" json:"last_modified"`
	ServerSideEncryption string            `yaml:"server_side_encryption,omitempty" json:"server_side_encryption,omitempty"`
	Metadata             map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// ArchiveIndex represents a complete index of archived data
type ArchiveIndex struct {
	Files        []*EnhancedFile   `json:"files"`
	CreatedAt    time.Time         `json:"created_at"`
	Location     string            `json:"location"`      // S3 bucket/prefix or local path
	TotalSize    int64             `json:"total_size"`    // Total size in bytes
	FileCount    int               `json:"file_count"`    // Number of files
	IndexVersion string            `json:"index_version"` // Index format version for migrations
	Checksums    map[string]string `json:"checksums"`     // Index integrity checksums
	Compression  CompressionInfo   `json:"compression"`   // Overall compression stats
	Statistics   IndexStatistics   `json:"statistics"`    // Summary statistics
}

// IndexStatistics provides summary statistics for an archive index
type IndexStatistics struct {
	AverageFileSize      int64            `json:"average_file_size"`
	LargestFileSize      int64            `json:"largest_file_size"`
	SmallestFileSize     int64            `json:"smallest_file_size"`
	FileTypeDistribution map[string]int   `json:"file_type_distribution"` // Extension -> count
	SizeDistribution     map[string]int64 `json:"size_distribution"`      // Size ranges -> total bytes
	DirectoryCount       int              `json:"directory_count"`
	TotalDirectorySize   int64            `json:"total_directory_size"`
}

// SearchFilter represents advanced search criteria for archive browsing
type SearchFilter struct {
	// File name patterns
	NamePattern string   `json:"name_pattern,omitempty"` // Glob pattern for filename
	Extensions  []string `json:"extensions,omitempty"`   // File extensions to match

	// Size filters
	MinSize int64 `json:"min_size,omitempty"` // Minimum file size in bytes
	MaxSize int64 `json:"max_size,omitempty"` // Maximum file size in bytes

	// Date filters
	ModifiedAfter  *time.Time `json:"modified_after,omitempty"`  // Files modified after this date
	ModifiedBefore *time.Time `json:"modified_before,omitempty"` // Files modified before this date
	ArchivedAfter  *time.Time `json:"archived_after,omitempty"`  // Files archived after this date
	ArchivedBefore *time.Time `json:"archived_before,omitempty"` // Files archived before this date

	// Content filters
	ContentType string            `json:"content_type,omitempty"` // MIME type pattern
	Tags        map[string]string `json:"tags,omitempty"`         // Key-value tag filters

	// Location filters
	StorageClass    string `json:"storage_class,omitempty"`    // S3 storage class
	SuitcasePattern string `json:"suitcase_pattern,omitempty"` // Suitcase name pattern
	PathPattern     string `json:"path_pattern,omitempty"`     // Full path pattern

	// Advanced filters
	HasArchiveTOC       bool    `json:"has_archive_toc,omitempty"`       // Files with table of contents
	CompressionType     string  `json:"compression_type,omitempty"`      // Compression algorithm
	MinCompressionRatio float64 `json:"min_compression_ratio,omitempty"` // Minimum compression ratio

	// Search behavior
	CaseSensitive   bool `json:"case_sensitive,omitempty"`   // Case sensitive matching
	IncludeArchived bool `json:"include_archived,omitempty"` // Include archived files
	MaxResults      int  `json:"max_results,omitempty"`      // Limit result count
}

// SearchResult represents the result of a search operation
type SearchResult struct {
	Files        []*EnhancedFile `json:"files"`
	TotalMatches int             `json:"total_matches"` // Total matches (may exceed len(Files) if MaxResults set)
	SearchTime   time.Duration   `json:"search_time"`   // Time taken to perform search
	IndexUsed    string          `json:"index_used"`    // Which index was used for search
	Truncated    bool            `json:"truncated"`     // True if results were truncated due to MaxResults
}

// BrowseOptions configures how archive browsing is performed
type BrowseOptions struct {
	Recursive      bool          `json:"recursive"`                 // Browse recursively into directories
	ShowMetadata   bool          `json:"show_metadata"`             // Include detailed metadata in results
	ShowHidden     bool          `json:"show_hidden"`               // Include hidden files (starting with .)
	SortBy         string        `json:"sort_by"`                   // Sort field: name, size, date, type
	SortOrder      string        `json:"sort_order"`                // Sort order: asc, desc
	MaxDepth       int           `json:"max_depth,omitempty"`       // Maximum directory depth (0 = unlimited)
	Filter         *SearchFilter `json:"filter,omitempty"`          // Optional filter to apply
	PageSize       int           `json:"page_size,omitempty"`       // Results per page for pagination
	PageOffset     int           `json:"page_offset,omitempty"`     // Offset for pagination
	ContentPreview bool          `json:"content_preview,omitempty"` // Generate content previews for supported files
}

// BrowseResult represents the result of browsing an archive or directory
type BrowseResult struct {
	Path        string          `json:"path"`        // Path being browsed
	Files       []*EnhancedFile `json:"files"`       // Files in current directory
	Directories []DirectoryInfo `json:"directories"` // Subdirectories
	TotalFiles  int             `json:"total_files"` // Total files in result
	TotalSize   int64           `json:"total_size"`  // Total size of files
	BrowseTime  time.Duration   `json:"browse_time"` // Time taken to browse
	HasMore     bool            `json:"has_more"`    // True if more results available (pagination)
}

// DirectoryInfo provides information about a directory in browse results
type DirectoryInfo struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	FileCount    int       `json:"file_count"`
	TotalSize    int64     `json:"total_size"`
	LastModified time.Time `json:"last_modified"`
	IsArchive    bool      `json:"is_archive"` // True if this is an archive file being treated as directory
}

// ConvertFromInventoryFile converts an inventory.File to EnhancedFile with minimal metadata
func ConvertFromInventoryFile(file *inventory.File) *EnhancedFile {
	now := time.Now()
	return &EnhancedFile{
		File:         *file,
		CreatedAt:    now,
		ModifiedAt:   now,
		StorageClass: "STANDARD", // Default S3 storage class
		Tags:         make(map[string]string),
	}
}

// ToInventoryFile converts an EnhancedFile back to inventory.File for compatibility
func (ef *EnhancedFile) ToInventoryFile() *inventory.File {
	return &ef.File
}

// HasTag checks if the file has a specific tag with the given value
func (ef *EnhancedFile) HasTag(key, value string) bool {
	if ef.Tags == nil {
		return false
	}
	tagValue, exists := ef.Tags[key]
	return exists && tagValue == value
}

// AddTag adds or updates a tag on the file
func (ef *EnhancedFile) AddTag(key, value string) {
	if ef.Tags == nil {
		ef.Tags = make(map[string]string)
	}
	ef.Tags[key] = value
}

// GetHumanSize returns a human-readable file size
func (ef *EnhancedFile) GetHumanSize() string {
	return humanizeBytes(ef.Size)
}

// IsCompressed returns true if the file appears to be compressed
func (ef *EnhancedFile) IsCompressed() bool {
	return ef.CompressionInfo.CompressedSize > 0 && ef.CompressionInfo.OriginalSize > ef.CompressionInfo.CompressedSize
}

// GetCompressionRatio calculates and returns the compression ratio
func (ef *EnhancedFile) GetCompressionRatio() float64 {
	if ef.CompressionInfo.OriginalSize == 0 {
		return 0.0
	}
	return float64(ef.CompressionInfo.CompressedSize) / float64(ef.CompressionInfo.OriginalSize)
}

// humanizeBytes converts byte count to human readable format
func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
