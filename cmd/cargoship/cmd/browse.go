package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drewstinnett/gout/v2"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/indexing"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

// NewBrowseCmd creates a new 'browse' command for advanced archive exploration
func NewBrowseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browse [LOCATION] [PATH]",
		Short: "Browse archived data with advanced filtering and search",
		Long: `Browse and explore archived data using enhanced indexing capabilities.
This command provides directory-style navigation of archives with powerful
filtering, searching, and metadata display options.

LOCATION can be:
- S3 bucket: s3://bucket-name/prefix
- Local path: /path/to/archives
- Inventory files will be automatically detected and indexed

PATH is the directory path within the archive to browse (optional, defaults to root)`,
		Example: `# Browse S3 archived data
cargoship browse s3://my-archive-bucket/

# Browse with recursive listing and metadata
cargoship browse s3://bucket/project-2024/ --recursive --show-metadata

# Browse local archives 
cargoship browse ./archives/ --show-suitcase-contents

# Filter by file size and type
cargoship browse s3://bucket/ --min-size 1MB --extensions .fastq,.bam

# Search with advanced filters
cargoship browse s3://bucket/ --pattern "analysis*" --after 2024-01-01`,
		Args: cobra.RangeArgs(0, 2),
		RunE:  runBrowseCommand,
	}

	// Browse behavior options
	cmd.Flags().Bool("recursive", false, "Browse directories recursively")
	cmd.Flags().Bool("show-metadata", false, "Display detailed file metadata")
	cmd.Flags().Bool("show-hidden", false, "Include hidden files (starting with .)")
	cmd.Flags().Bool("show-suitcase-contents", false, "Show contents of suitcase archives")
	cmd.Flags().String("sort-by", "name", "Sort results by: name, size, date, type")
	cmd.Flags().String("sort-order", "asc", "Sort order: asc, desc")
	cmd.Flags().Int("max-depth", 0, "Maximum directory depth (0 = unlimited)")
	cmd.Flags().Int("page-size", 0, "Results per page (0 = no pagination)")
	cmd.Flags().Int("page", 1, "Page number for pagination")

	// Search and filtering options
	cmd.Flags().String("pattern", "", "Glob pattern to match file names")
	cmd.Flags().StringSlice("extensions", []string{}, "File extensions to include (e.g. .txt,.json)")
	cmd.Flags().String("min-size", "", "Minimum file size (e.g. 1MB, 500KB)")
	cmd.Flags().String("max-size", "", "Maximum file size (e.g. 1GB, 100MB)")
	cmd.Flags().String("after", "", "Files modified after date (YYYY-MM-DD)")
	cmd.Flags().String("before", "", "Files modified before date (YYYY-MM-DD)")
	cmd.Flags().String("content-type", "", "Filter by content type pattern")
	cmd.Flags().StringToString("tags", map[string]string{}, "Filter by tags (key=value pairs)")
	cmd.Flags().String("storage-class", "", "Filter by S3 storage class")
	cmd.Flags().String("suitcase-pattern", "", "Filter by suitcase name pattern")
	cmd.Flags().String("path-pattern", "", "Filter by file path pattern")
	cmd.Flags().Bool("has-archive-toc", false, "Only show files with archive table of contents")
	cmd.Flags().String("compression-type", "", "Filter by compression algorithm")
	cmd.Flags().Float64("min-compression-ratio", 0, "Minimum compression ratio")
	cmd.Flags().Int("max-results", 1000, "Maximum number of results to return")

	// Output options
	cmd.Flags().String("format", "table", "Output format: table, json, yaml")
	cmd.Flags().Bool("count-only", false, "Only show count of matching files")
	cmd.Flags().Bool("size-summary", false, "Show size summary by category")
	
	// Index management options
	cmd.Flags().StringArray("inventory-directory", []string{"."}, "Directories containing inventory files")
	cmd.Flags().String("index-cache-dir", "", "Directory for index cache (default: temp dir)")
	cmd.Flags().Bool("rebuild-index", false, "Force rebuild of archive indexes")
	cmd.Flags().Bool("no-cache", false, "Disable index caching")

	return cmd
}

// runBrowseCommand executes the browse command with enhanced indexing capabilities
func runBrowseCommand(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	
	// Parse arguments
	location := "."
	browsePath := ""
	
	if len(args) >= 1 {
		location = args[0]
	}
	if len(args) >= 2 {
		browsePath = args[1]
	}
	
	// Get command options
	options, err := parseBrowseOptions(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse options: %w", err)
	}
	
	// Set up index cache directory
	indexCacheDir, err := cmd.Flags().GetString("index-cache-dir")
	if err != nil {
		return err
	}
	if indexCacheDir == "" {
		indexCacheDir = filepath.Join(os.TempDir(), "cargoship-index-cache")
	}
	
	// Initialize indexer and search engine  
	indexer := indexing.NewIndexer(indexCacheDir, slog.Default())
	searchEngine := indexing.NewSearchEngine(indexer, slog.Default())
	
	// Handle index management flags
	rebuildIndex, _ := cmd.Flags().GetBool("rebuild-index")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	
	if noCache {
		indexer.ClearCache()
	}
	
	// Load or create index for the location
	err = ensureIndexExists(ctx, indexer, location, cmd, rebuildIndex)
	if err != nil {
		return fmt.Errorf("failed to prepare index: %w", err)
	}
	
	// Check if this is a search operation or browse operation
	if hasSearchFilters(cmd) {
		return runSearchOperation(ctx, searchEngine, location, options, cmd)
	}
	
	// Run browse operation
	return runBrowseOperation(ctx, searchEngine, location, browsePath, options, cmd)
}

// parseBrowseOptions parses command flags into browse options
func parseBrowseOptions(cmd *cobra.Command) (*indexing.BrowseOptions, error) {
	options := &indexing.BrowseOptions{}
	
	// Basic browse options
	options.Recursive, _ = cmd.Flags().GetBool("recursive")
	options.ShowMetadata, _ = cmd.Flags().GetBool("show-metadata")
	options.ShowHidden, _ = cmd.Flags().GetBool("show-hidden")
	options.SortBy, _ = cmd.Flags().GetString("sort-by")
	options.SortOrder, _ = cmd.Flags().GetString("sort-order")
	options.MaxDepth, _ = cmd.Flags().GetInt("max-depth")
	options.ContentPreview, _ = cmd.Flags().GetBool("show-suitcase-contents")
	
	// Pagination options
	pageSize, _ := cmd.Flags().GetInt("page-size")
	page, _ := cmd.Flags().GetInt("page")
	
	if pageSize > 0 {
		options.PageSize = pageSize
		options.PageOffset = (page - 1) * pageSize
	}
	
	// Parse search filter if any filters are specified
	filter, err := parseSearchFilter(cmd)
	if err != nil {
		return nil, err
	}
	
	if filter != nil {
		options.Filter = filter
	}
	
	return options, nil
}

// parseSearchFilter parses command flags into a search filter
func parseSearchFilter(cmd *cobra.Command) (*indexing.SearchFilter, error) {
	filter := &indexing.SearchFilter{}
	hasFilters := false
	
	// Name and path patterns
	if pattern, _ := cmd.Flags().GetString("pattern"); pattern != "" {
		filter.NamePattern = pattern
		hasFilters = true
	}
	
	if pathPattern, _ := cmd.Flags().GetString("path-pattern"); pathPattern != "" {
		filter.PathPattern = pathPattern
		hasFilters = true
	}
	
	// Extensions
	if extensions, _ := cmd.Flags().GetStringSlice("extensions"); len(extensions) > 0 {
		filter.Extensions = extensions
		hasFilters = true
	}
	
	// Size filters
	if minSizeStr, _ := cmd.Flags().GetString("min-size"); minSizeStr != "" {
		minSize, err := parseSize(minSizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid min-size: %w", err)
		}
		filter.MinSize = minSize
		hasFilters = true
	}
	
	if maxSizeStr, _ := cmd.Flags().GetString("max-size"); maxSizeStr != "" {
		maxSize, err := parseSize(maxSizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid max-size: %w", err)
		}
		filter.MaxSize = maxSize
		hasFilters = true
	}
	
	// Date filters
	if afterStr, _ := cmd.Flags().GetString("after"); afterStr != "" {
		after, err := time.Parse("2006-01-02", afterStr)
		if err != nil {
			return nil, fmt.Errorf("invalid after date format, use YYYY-MM-DD: %w", err)
		}
		filter.ModifiedAfter = &after
		hasFilters = true
	}
	
	if beforeStr, _ := cmd.Flags().GetString("before"); beforeStr != "" {
		before, err := time.Parse("2006-01-02", beforeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid before date format, use YYYY-MM-DD: %w", err)
		}
		filter.ModifiedBefore = &before
		hasFilters = true
	}
	
	// Content and metadata filters
	if contentType, _ := cmd.Flags().GetString("content-type"); contentType != "" {
		filter.ContentType = contentType
		hasFilters = true
	}
	
	if tags, _ := cmd.Flags().GetStringToString("tags"); len(tags) > 0 {
		filter.Tags = tags
		hasFilters = true
	}
	
	if storageClass, _ := cmd.Flags().GetString("storage-class"); storageClass != "" {
		filter.StorageClass = storageClass
		hasFilters = true
	}
	
	if suitcasePattern, _ := cmd.Flags().GetString("suitcase-pattern"); suitcasePattern != "" {
		filter.SuitcasePattern = suitcasePattern
		hasFilters = true
	}
	
	// Archive and compression filters
	if hasArchiveTOC, _ := cmd.Flags().GetBool("has-archive-toc"); hasArchiveTOC {
		filter.HasArchiveTOC = true
		hasFilters = true
	}
	
	if compressionType, _ := cmd.Flags().GetString("compression-type"); compressionType != "" {
		filter.CompressionType = compressionType
		hasFilters = true
	}
	
	if minCompressionRatio, _ := cmd.Flags().GetFloat64("min-compression-ratio"); minCompressionRatio > 0 {
		filter.MinCompressionRatio = minCompressionRatio
		hasFilters = true
	}
	
	// Result limits
	if maxResults, _ := cmd.Flags().GetInt("max-results"); maxResults > 0 {
		filter.MaxResults = maxResults
		hasFilters = true
	}
	
	if !hasFilters {
		return nil, nil
	}
	
	return filter, nil
}

// ensureIndexExists makes sure an index exists for the given location
func ensureIndexExists(ctx context.Context, indexer *indexing.Indexer, location string, cmd *cobra.Command, forceRebuild bool) error {
	// Check if index already exists
	if !forceRebuild {
		if _, err := indexer.LoadIndex(ctx, location); err == nil {
			slog.Debug("using existing index", "location", location)
			return nil
		}
	}
	
	slog.Info("creating index for location", "location", location, "force_rebuild", forceRebuild)
	
	// Get inventory directories
	inventoryDirs, err := cmd.Flags().GetStringArray("inventory-directory")
	if err != nil {
		return err
	}
	
	// For S3 locations, we might need different logic, but for now use inventory files
	var inventoryFiles []string
	
	if strings.HasPrefix(location, "s3://") {
		// For S3, look for inventory files that match the location
		for _, dir := range inventoryDirs {
			matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
			if err != nil {
				continue
			}
			inventoryFiles = append(inventoryFiles, matches...)
		}
	} else {
		// For local paths, find inventory files in the specified directories
		for _, dir := range inventoryDirs {
			matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
			if err != nil {
				continue
			}
			inventoryFiles = append(inventoryFiles, matches...)
		}
	}
	
	if len(inventoryFiles) == 0 {
		return fmt.Errorf("no inventory files found in directories: %v", inventoryDirs)
	}
	
	// Load the first inventory file (in a real implementation, we might merge multiple)
	inventoryFile := inventoryFiles[0]
	slog.Info("loading inventory from file", "file", inventoryFile)
	
	inv, err := inventory.NewInventoryWithFilename(inventoryFile)
	if err != nil {
		return fmt.Errorf("failed to load inventory from %s: %w", inventoryFile, err)
	}
	
	// Create index from inventory
	archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	
	// Save index for future use
	err = indexer.SaveIndex(ctx, archiveIndex)
	if err != nil {
		slog.Warn("failed to save index", "error", err)
		// Continue anyway, we can use the cached version
	}
	
	slog.Info("index created successfully", "location", location, "files", archiveIndex.FileCount)
	return nil
}

// hasSearchFilters checks if any search filters are specified
func hasSearchFilters(cmd *cobra.Command) bool {
	searchFlags := []string{
		"pattern", "extensions", "min-size", "max-size", "after", "before",
		"content-type", "tags", "storage-class", "suitcase-pattern", "path-pattern",
		"compression-type", "min-compression-ratio",
	}
	
	for _, flag := range searchFlags {
		if cmd.Flags().Changed(flag) {
			return true
		}
	}
	
	hasArchiveTOC, _ := cmd.Flags().GetBool("has-archive-toc")
	return hasArchiveTOC
}

// runSearchOperation performs a search across the archive index
func runSearchOperation(ctx context.Context, searchEngine *indexing.SearchEngine, location string, options *indexing.BrowseOptions, cmd *cobra.Command) error {
	slog.Info("performing search operation", "location", location)
	
	// Get search filter from options
	filter := indexing.SearchFilter{}
	if options.Filter != nil {
		filter = *options.Filter
	}
	
	// Perform search
	result, err := searchEngine.Search(ctx, filter, location)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	
	// Handle count-only flag
	if countOnly, _ := cmd.Flags().GetBool("count-only"); countOnly {
		fmt.Printf("Found %d matching files\n", result.TotalMatches)
		return nil
	}
	
	// Handle size-summary flag
	if sizeSummary, _ := cmd.Flags().GetBool("size-summary"); sizeSummary {
		return displaySizeSummary(result.Files)
	}
	
	// Display results
	return displaySearchResults(result, cmd)
}

// runBrowseOperation performs directory-style browsing
func runBrowseOperation(ctx context.Context, searchEngine *indexing.SearchEngine, location string, browsePath string, options *indexing.BrowseOptions, cmd *cobra.Command) error {
	slog.Info("performing browse operation", "location", location, "path", browsePath)
	
	// Perform browse
	result, err := searchEngine.Browse(ctx, location, browsePath, *options)
	if err != nil {
		return fmt.Errorf("browse failed: %w", err)
	}
	
	// Handle count-only flag
	if countOnly, _ := cmd.Flags().GetBool("count-only"); countOnly {
		fmt.Printf("Found %d files and %d directories\n", result.TotalFiles, len(result.Directories))
		return nil
	}
	
	// Display results
	return displayBrowseResults(result, cmd)
}

// displaySearchResults displays search results in the requested format
func displaySearchResults(result *indexing.SearchResult, cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "json":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(result)
	case "yaml":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(result)
	case "table":
		fallthrough
	default:
		return displaySearchResultsTable(result, cmd)
	}
}

// displayBrowseResults displays browse results in the requested format
func displayBrowseResults(result *indexing.BrowseResult, cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "json":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(result)
	case "yaml":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(result)
	case "table":
		fallthrough
	default:
		return displayBrowseResultsTable(result, cmd)
	}
}

// displaySearchResultsTable displays search results in a formatted table
func displaySearchResultsTable(result *indexing.SearchResult, cmd *cobra.Command) error {
	showMetadata, _ := cmd.Flags().GetBool("show-metadata")
	
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n🔍 Search Results\n")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "═══════════════════════════════════════════════════════════════\n")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d matches in %v (using %s)\n", 
		result.TotalMatches, result.SearchTime, result.IndexUsed)
	
	if result.Truncated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Results truncated - showing first %d matches\n", len(result.Files))
	}
	
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n📁 Files:\n")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
	
	for _, file := range result.Files {
		// Display basic file info
		fmt.Fprintf(cmd.OutOrStdout(), "📄 %s\n", file.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "   Path: %s\n", file.Destination)
		fmt.Fprintf(cmd.OutOrStdout(), "   Size: %s\n", file.GetHumanSize())
		fmt.Fprintf(cmd.OutOrStdout(), "   Suitcase: %s\n", file.SuitcaseName)
		
		if showMetadata {
			fmt.Fprintf(cmd.OutOrStdout(), "   Type: %s\n", file.ContentType)
			fmt.Fprintf(cmd.OutOrStdout(), "   Storage: %s\n", file.StorageClass)
			fmt.Fprintf(cmd.OutOrStdout(), "   Modified: %s\n", file.ModifiedAt.Format("2006-01-02 15:04:05"))
			
			if len(file.Tags) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "   Tags: ")
				first := true
				for k, v := range file.Tags {
					if !first {
						fmt.Fprintf(cmd.OutOrStdout(), ", ")
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s", k, v)
					first = false
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n")
			}
			
			if file.IsCompressed() {
				fmt.Fprintf(cmd.OutOrStdout(), "   Compression: %s (ratio: %.2f)\n", 
					file.CompressionInfo.Algorithm, file.GetCompressionRatio())
			}
			
			if len(file.ArchiveTOC) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "   Archive contents: %d files\n", len(file.ArchiveTOC))
			}
		}
		
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
	}
	
	return nil
}

// displayBrowseResultsTable displays browse results in a formatted table
func displayBrowseResultsTable(result *indexing.BrowseResult, cmd *cobra.Command) error {
	showMetadata, _ := cmd.Flags().GetBool("show-metadata")
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n📂 Browse: %s\n", result.Path)
	fmt.Fprintf(cmd.OutOrStdout(), "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Found %d files in %v\n", result.TotalFiles, result.BrowseTime)
	
	if result.HasMore {
		fmt.Fprintf(cmd.OutOrStdout(), "⚠️  More results available - use pagination flags to see all\n")
	}
	
	// Display directories first
	if len(result.Directories) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n📁 Directories:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
		
		for _, dir := range result.Directories {
			fmt.Fprintf(cmd.OutOrStdout(), "📁 %s/\n", dir.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "   Files: %d (%s)\n", dir.FileCount, humanizeBytes(dir.TotalSize))
			if dir.IsArchive {
				fmt.Fprintf(cmd.OutOrStdout(), "   Type: Archive\n")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n")
		}
	}
	
	// Display files
	if len(result.Files) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n📄 Files:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
		
		for _, file := range result.Files {
			// Display basic file info
			fmt.Fprintf(cmd.OutOrStdout(), "📄 %s\n", file.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "   Size: %s\n", file.GetHumanSize())
			
			if showMetadata {
				fmt.Fprintf(cmd.OutOrStdout(), "   Type: %s\n", file.ContentType)
				fmt.Fprintf(cmd.OutOrStdout(), "   Storage: %s\n", file.StorageClass)
				fmt.Fprintf(cmd.OutOrStdout(), "   Suitcase: %s\n", file.SuitcaseName)
				fmt.Fprintf(cmd.OutOrStdout(), "   Modified: %s\n", file.ModifiedAt.Format("2006-01-02 15:04:05"))
				
				if file.IsCompressed() {
					fmt.Fprintf(cmd.OutOrStdout(), "   Compression: %s (%.1f%%)\n", 
						file.CompressionInfo.Algorithm, (1.0-file.GetCompressionRatio())*100)
				}
			}
			
			fmt.Fprintf(cmd.OutOrStdout(), "\n")
		}
	}
	
	return nil
}

// displaySizeSummary shows a summary of files by size category
func displaySizeSummary(files []*indexing.EnhancedFile) error {
	sizeCounts := make(map[string]int)
	sizeTotals := make(map[string]int64)
	
	for _, file := range files {
		category := getSizeCategory(file.Size)
		sizeCounts[category]++
		sizeTotals[category] += file.Size
	}
	
	fmt.Printf("\n📊 Size Summary\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	
	categories := []string{"tiny", "small", "medium", "large", "xlarge", "xxlarge", "huge", "massive"}
	for _, category := range categories {
		if count := sizeCounts[category]; count > 0 {
			fmt.Printf("%-10s: %6d files (%s)\n", 
				Title(category), count, humanizeBytes(sizeTotals[category]))
		}
	}
	
	return nil
}

// parseSize parses a human-readable size string to bytes
func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	
	// Extract number and unit
	var number float64
	var unit string
	
	if _, err := fmt.Sscanf(sizeStr, "%f%s", &number, &unit); err != nil {
		// Try parsing as just a number (bytes)
		if _, err := fmt.Sscanf(sizeStr, "%f", &number); err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
		unit = "B"
	}
	
	// Convert to bytes
	var multiplier int64
	switch unit {
	case "B", "BYTES":
		multiplier = 1
	case "KB", "K":
		multiplier = 1024
	case "MB", "M":
		multiplier = 1024 * 1024
	case "GB", "G":
		multiplier = 1024 * 1024 * 1024
	case "TB", "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}
	
	return int64(number * float64(multiplier)), nil
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
		return "tiny"
	case size < MB:
		return "small"
	case size < 10*MB:
		return "medium"
	case size < 100*MB:
		return "large"
	case size < GB:
		return "xlarge"
	case size < 10*GB:
		return "xxlarge"
	case size < TB:
		return "huge"
	default:
		return "massive"
	}
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

// Title capitalizes the first letter of a string
func Title(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}