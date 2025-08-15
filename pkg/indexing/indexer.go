// Package indexing provides enhanced metadata and search capabilities for CargoShip archives
package indexing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

// Indexer manages archive indexes and provides enhanced search capabilities
type Indexer struct {
	logger   *slog.Logger
	indexes  map[string]*ArchiveIndex  // Cache of loaded indexes
	basePath string                    // Base path for storing indexes
}

// NewIndexer creates a new archive indexer
func NewIndexer(basePath string, logger *slog.Logger) *Indexer {
	if logger == nil {
		logger = slog.Default()
	}
	
	return &Indexer{
		logger:   logger.With("component", "indexer"),
		indexes:  make(map[string]*ArchiveIndex),
		basePath: basePath,
	}
}

// CreateIndex creates an enhanced index from an existing inventory
func (idx *Indexer) CreateIndex(ctx context.Context, inv *inventory.Inventory, location string) (*ArchiveIndex, error) {
	idx.logger.Info("creating archive index", "location", location, "file_count", len(inv.Files))
	
	startTime := time.Now()
	
	// Convert inventory files to enhanced files
	enhancedFiles := make([]*EnhancedFile, len(inv.Files))
	totalSize := int64(0)
	
	for i, file := range inv.Files {
		enhanced := ConvertFromInventoryFile(file)
		
		// Enrich with additional metadata
		if err := idx.enrichFileMetadata(ctx, enhanced, location); err != nil {
			idx.logger.Warn("failed to enrich file metadata", "file", file.Path, "error", err)
			// Continue with basic metadata
		}
		
		enhancedFiles[i] = enhanced
		totalSize += file.Size
	}
	
	// Create the archive index
	archiveIndex := &ArchiveIndex{
		Files:         enhancedFiles,
		CreatedAt:     time.Now(),
		Location:      location,
		TotalSize:     totalSize,
		FileCount:     len(enhancedFiles),
		IndexVersion:  "1.0",
		Checksums:     make(map[string]string),
		Statistics:    idx.calculateStatistics(enhancedFiles),
	}
	
	// Calculate overall compression statistics
	archiveIndex.Compression = idx.calculateOverallCompression(enhancedFiles)
	
	// Generate index checksums for integrity
	if err := idx.generateIndexChecksums(archiveIndex); err != nil {
		idx.logger.Error("failed to generate index checksums", "error", err)
	}
	
	// Store in cache
	idx.indexes[location] = archiveIndex
	
	duration := time.Since(startTime)
	idx.logger.Info("archive index created", 
		"location", location, 
		"files", len(enhancedFiles), 
		"total_size", totalSize,
		"duration", duration)
	
	return archiveIndex, nil
}

// LoadIndex loads an existing index from storage
func (idx *Indexer) LoadIndex(ctx context.Context, location string) (*ArchiveIndex, error) {
	// Check cache first
	if cached, exists := idx.indexes[location]; exists {
		idx.logger.Debug("returning cached index", "location", location)
		return cached, nil
	}
	
	// Load from storage
	indexPath := idx.getIndexPath(location)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("index not found for location: %s", location)
	}
	
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}
	
	var archiveIndex ArchiveIndex
	if err := json.Unmarshal(data, &archiveIndex); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}
	
	// Verify index integrity
	if err := idx.verifyIndexIntegrity(&archiveIndex); err != nil {
		idx.logger.Warn("index integrity check failed", "location", location, "error", err)
		// Continue with potentially corrupted index, but log the issue
	}
	
	// Store in cache
	idx.indexes[location] = &archiveIndex
	
	idx.logger.Info("index loaded", "location", location, "files", archiveIndex.FileCount)
	return &archiveIndex, nil
}

// SaveIndex persists an index to storage
func (idx *Indexer) SaveIndex(ctx context.Context, archiveIndex *ArchiveIndex) error {
	indexPath := idx.getIndexPath(archiveIndex.Location)
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}
	
	// Update checksums before saving
	if err := idx.generateIndexChecksums(archiveIndex); err != nil {
		idx.logger.Warn("failed to update index checksums", "error", err)
	}
	
	// Marshal to JSON
	data, err := json.MarshalIndent(archiveIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	
	// Write to file
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	
	// Update cache
	idx.indexes[archiveIndex.Location] = archiveIndex
	
	idx.logger.Info("index saved", "location", archiveIndex.Location, "path", indexPath)
	return nil
}

// RebuildIndex recreates an index from the original inventory
func (idx *Indexer) RebuildIndex(ctx context.Context, inventoryPath string, location string) (*ArchiveIndex, error) {
	idx.logger.Info("rebuilding index", "inventory_path", inventoryPath, "location", location)
	
	// Load the original inventory
	inv, err := inventory.NewInventoryWithFilename(inventoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load inventory: %w", err)
	}
	
	// Clear cached index
	delete(idx.indexes, location)
	
	// Create new index
	return idx.CreateIndex(ctx, inv, location)
}

// ListIndexes returns a list of all available indexes
func (idx *Indexer) ListIndexes(ctx context.Context) ([]string, error) {
	indexDir := filepath.Join(idx.basePath, "indexes")
	
	if _, err := os.Stat(indexDir); os.IsNotExist(err) {
		return []string{}, nil
	}
	
	var locations []string
	err := filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			// Load the index to get the actual location
			data, err := os.ReadFile(path)
			if err != nil {
				idx.logger.Warn("failed to read index file for listing", "path", path, "error", err)
				return nil // Continue with other files
			}
			
			var archiveIndex ArchiveIndex
			if err := json.Unmarshal(data, &archiveIndex); err != nil {
				idx.logger.Warn("failed to parse index file for listing", "path", path, "error", err)
				return nil // Continue with other files
			}
			
			locations = append(locations, archiveIndex.Location)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to list indexes: %w", err)
	}
	
	return locations, nil
}

// GetCacheStats returns statistics about the indexer cache
func (idx *Indexer) GetCacheStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["cached_indexes"] = len(idx.indexes)
	
	totalFiles := 0
	totalSize := int64(0)
	
	for location, archiveIndex := range idx.indexes {
		totalFiles += archiveIndex.FileCount
		totalSize += archiveIndex.TotalSize
		
		stats[fmt.Sprintf("index_%s_files", sanitizeKey(location))] = archiveIndex.FileCount
		stats[fmt.Sprintf("index_%s_size", sanitizeKey(location))] = archiveIndex.TotalSize
	}
	
	stats["total_cached_files"] = totalFiles
	stats["total_cached_size"] = totalSize
	
	return stats
}

// ClearCache clears all cached indexes
func (idx *Indexer) ClearCache() {
	idx.indexes = make(map[string]*ArchiveIndex)
	idx.logger.Info("index cache cleared")
}

// enrichFileMetadata adds enhanced metadata to a file based on its properties and location
func (idx *Indexer) enrichFileMetadata(ctx context.Context, file *EnhancedFile, location string) error {
	// Determine content type from file extension
	file.ContentType = getContentType(file.Name)
	
	// Add location-based tags
	if strings.HasPrefix(location, "s3://") {
		file.AddTag("location_type", "s3")
		
		// Parse S3 location for metadata
		if bucket, key := parseS3Location(location); bucket != "" {
			file.AddTag("s3_bucket", bucket)
			if key != "" {
				file.AddTag("s3_key_prefix", key)
			}
		}
	} else {
		file.AddTag("location_type", "local")
		file.AddTag("local_path", location)
	}
	
	// Determine compression info from file extension and archive TOC
	if isCompressedFile(file.Name) {
		file.CompressionInfo = CompressionInfo{
			Algorithm:        getCompressionAlgorithm(file.Name),
			CompressedSize:   file.Size,
			OriginalSize:     file.Size, // Will be updated if we can determine actual original size
			CompressionRatio: 1.0,       // Default to no compression unless we can calculate better
		}
	}
	
	// Add file type tag
	file.AddTag("file_type", getFileType(file.Name))
	
	// Add size category tag
	file.AddTag("size_category", getSizeCategory(file.Size))
	
	return nil
}

// calculateStatistics generates summary statistics for a set of files
func (idx *Indexer) calculateStatistics(files []*EnhancedFile) IndexStatistics {
	stats := IndexStatistics{
		FileTypeDistribution: make(map[string]int),
		SizeDistribution:     make(map[string]int64),
	}
	
	if len(files) == 0 {
		return stats
	}
	
	totalSize := int64(0)
	maxSize := int64(0)
	minSize := files[0].Size
	directories := make(map[string]bool)
	
	for _, file := range files {
		totalSize += file.Size
		
		if file.Size > maxSize {
			maxSize = file.Size
		}
		if file.Size < minSize {
			minSize = file.Size
		}
		
		// Track file type distribution
		ext := strings.ToLower(filepath.Ext(file.Name))
		if ext == "" {
			ext = "no_extension"
		}
		stats.FileTypeDistribution[ext]++
		
		// Track size distribution
		sizeCategory := getSizeCategory(file.Size)
		stats.SizeDistribution[sizeCategory] += file.Size
		
		// Track unique directories
		dir := filepath.Dir(file.Destination)
		if dir != "." {
			directories[dir] = true
		}
	}
	
	stats.AverageFileSize = totalSize / int64(len(files))
	stats.LargestFileSize = maxSize
	stats.SmallestFileSize = minSize
	stats.DirectoryCount = len(directories)
	stats.TotalDirectorySize = totalSize
	
	return stats
}

// calculateOverallCompression calculates overall compression statistics
func (idx *Indexer) calculateOverallCompression(files []*EnhancedFile) CompressionInfo {
	totalOriginal := int64(0)
	totalCompressed := int64(0)
	compressedFiles := 0
	algorithmCounts := make(map[string]int)
	
	for _, file := range files {
		if file.IsCompressed() {
			totalOriginal += file.CompressionInfo.OriginalSize
			totalCompressed += file.CompressionInfo.CompressedSize
			compressedFiles++
			algorithmCounts[file.CompressionInfo.Algorithm]++
		}
	}
	
	info := CompressionInfo{
		Algorithm:       "mixed",
		OriginalSize:    totalOriginal,
		CompressedSize:  totalCompressed,
		CompressionRatio: 1.0,
	}
	
	if totalOriginal > 0 {
		info.CompressionRatio = float64(totalCompressed) / float64(totalOriginal)
	}
	
	// Determine dominant algorithm
	maxCount := 0
	for algorithm, count := range algorithmCounts {
		if count > maxCount {
			maxCount = count
			info.Algorithm = algorithm
		}
	}
	
	if compressedFiles <= 1 {
		info.Algorithm = "none"
	}
	
	return info
}

// generateIndexChecksums generates integrity checksums for the index
func (idx *Indexer) generateIndexChecksums(archiveIndex *ArchiveIndex) error {
	// Generate a simple checksum of the file list for integrity checking
	// This is a simplified implementation - in production you might want more sophisticated checksums
	
	fileHashes := make([]string, len(archiveIndex.Files))
	for i, file := range archiveIndex.Files {
		fileHashes[i] = fmt.Sprintf("%s:%d:%s", file.Path, file.Size, file.ModifiedAt.Format(time.RFC3339))
	}
	
	// Simple hash of concatenated file information
	combined := strings.Join(fileHashes, "|")
	archiveIndex.Checksums["file_list"] = fmt.Sprintf("%x", len(combined)) // Simplified checksum
	archiveIndex.Checksums["created_at"] = archiveIndex.CreatedAt.Format(time.RFC3339)
	
	return nil
}

// verifyIndexIntegrity verifies the integrity of a loaded index
func (idx *Indexer) verifyIndexIntegrity(archiveIndex *ArchiveIndex) error {
	// Basic integrity checks
	if archiveIndex.FileCount != len(archiveIndex.Files) {
		return fmt.Errorf("file count mismatch: expected %d, got %d", archiveIndex.FileCount, len(archiveIndex.Files))
	}
	
	calculatedSize := int64(0)
	for _, file := range archiveIndex.Files {
		calculatedSize += file.Size
	}
	
	if archiveIndex.TotalSize != calculatedSize {
		return fmt.Errorf("total size mismatch: expected %d, got %d", archiveIndex.TotalSize, calculatedSize)
	}
	
	return nil
}

// getIndexPath returns the file system path for storing an index
func (idx *Indexer) getIndexPath(location string) string {
	// Convert location to safe file path
	safePath := sanitizePathForFilename(location)
	return filepath.Join(idx.basePath, "indexes", safePath+".json")
}

// Helper functions

// sanitizeKey sanitizes a string for use as a map key
func sanitizeKey(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", "_"), ":", "_")
}

// sanitizePathForFilename converts a location string to a safe filename
func sanitizePathForFilename(location string) string {
	// Replace problematic characters with safe alternatives
	safe := strings.ReplaceAll(location, "://", "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	safe = strings.ReplaceAll(safe, "?", "_")
	safe = strings.ReplaceAll(safe, "*", "_")
	safe = strings.ReplaceAll(safe, "|", "_")
	safe = strings.ReplaceAll(safe, "<", "_")
	safe = strings.ReplaceAll(safe, ">", "_")
	safe = strings.ReplaceAll(safe, "\"", "_")
	return safe
}

// getContentType determines MIME type from file extension
func getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".log":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".gz":
		return "application/gzip"
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".fastq", ".fq":
		return "application/x-fastq"
	case ".bam":
		return "application/x-bam"
	case ".vcf":
		return "application/x-vcf"
	default:
		return "application/octet-stream"
	}
}

// parseS3Location parses an S3 URL into bucket and key
func parseS3Location(location string) (bucket, key string) {
	if !strings.HasPrefix(location, "s3://") {
		return "", ""
	}
	
	path := strings.TrimPrefix(location, "s3://")
	parts := strings.SplitN(path, "/", 2)
	
	bucket = parts[0]
	if len(parts) > 1 {
		key = parts[1]
	}
	
	return bucket, key
}

// isCompressedFile checks if a file appears to be compressed based on extension
func isCompressedFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	compressedExtensions := []string{".gz", ".bz2", ".xz", ".zst", ".zip", ".tar", ".7z", ".rar"}
	
	for _, compressedExt := range compressedExtensions {
		if ext == compressedExt {
			return true
		}
	}
	
	// Check for compound extensions like .tar.gz
	if strings.Contains(filename, ".tar.") {
		return true
	}
	
	return false
}

// getCompressionAlgorithm determines compression algorithm from filename
func getCompressionAlgorithm(filename string) string {
	lower := strings.ToLower(filename)
	
	if strings.HasSuffix(lower, ".zst") || strings.HasSuffix(lower, ".zstd") {
		return "zstd"
	}
	if strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".gzip") {
		return "gzip"
	}
	if strings.HasSuffix(lower, ".bz2") || strings.HasSuffix(lower, ".bzip2") {
		return "bzip2"
	}
	if strings.HasSuffix(lower, ".xz") {
		return "xz"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	if strings.HasSuffix(lower, ".7z") {
		return "7zip"
	}
	if strings.HasSuffix(lower, ".rar") {
		return "rar"
	}
	if strings.HasSuffix(lower, ".tar") {
		return "tar"
	}
	
	return "unknown"
}

// getFileType determines file type category from filename
func getFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	
	// Bioinformatics formats
	bioFormats := []string{".fastq", ".fq", ".fasta", ".fa", ".bam", ".sam", ".vcf", ".bed", ".gff", ".gtf"}
	for _, bioExt := range bioFormats {
		if ext == bioExt {
			return "bioinformatics"
		}
	}
	
	// Data formats
	dataFormats := []string{".csv", ".tsv", ".json", ".xml", ".yaml", ".yml"}
	for _, dataExt := range dataFormats {
		if ext == dataExt {
			return "data"
		}
	}
	
	// Text formats
	textFormats := []string{".txt", ".log", ".md", ".rst"}
	for _, textExt := range textFormats {
		if ext == textExt {
			return "text"
		}
	}
	
	// Image formats
	imageFormats := []string{".jpg", ".jpeg", ".png", ".gif", ".tiff", ".bmp"}
	for _, imgExt := range imageFormats {
		if ext == imgExt {
			return "image"
		}
	}
	
	// Archive formats
	archiveFormats := []string{".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar"}
	for _, archExt := range archiveFormats {
		if ext == archExt {
			return "archive"
		}
	}
	
	// Document formats
	docFormats := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx"}
	for _, docExt := range docFormats {
		if ext == docExt {
			return "document"
		}
	}
	
	return "other"
}

// getSizeCategory categorizes file size into human-readable categories
func getSizeCategory(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	
	switch {
	case size < KB:
		return "tiny"      // < 1KB
	case size < MB:
		return "small"     // 1KB - 1MB
	case size < 10*MB:
		return "medium"    // 1MB - 10MB
	case size < 100*MB:
		return "large"     // 10MB - 100MB
	case size < GB:
		return "xlarge"    // 100MB - 1GB
	case size < 10*GB:
		return "xxlarge"   // 1GB - 10GB
	case size < TB:
		return "huge"      // 10GB - 1TB
	default:
		return "massive"   // > 1TB
	}
}