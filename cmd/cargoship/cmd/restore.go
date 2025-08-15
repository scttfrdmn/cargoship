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

// NewRestoreCmd creates a new 'restore' command for data restoration and preview
func NewRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore [LOCATION] [TARGET]",
		Short: "Restore archived data with preview and cost estimation",
		Long: `Restore files and directories from archived data with comprehensive preview capabilities.
This command provides detailed information about what will be restored before performing
actual restoration operations, including cost estimates and progress projections.

LOCATION can be:
- S3 archive: s3://bucket-name/archive.tar.gz
- S3 suitcase: s3://bucket-name/suitcase-01-of-05.tar.zst
- Local archive: /path/to/archive.tar.gz

TARGET is the destination directory for restored files (optional, defaults to current directory)`,
		Example: `# Preview what will be restored without downloading
cargoship restore --preview s3://bucket/dataset.tar.gz

# Estimate restoration costs and time
cargoship restore --estimate-cost s3://bucket/large-archive/

# List archive contents in JSON format
cargoship restore --list-contents s3://bucket/suitcase.tar.gz --format json

# Preview specific files only
cargoship restore --preview s3://bucket/archive.tar.gz --pattern "*.fastq"

# Estimate selective restoration
cargoship restore --estimate-cost s3://bucket/data/ --pattern "analysis*" --max-size 1GB`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runRestoreCommand,
	}

	// Preview and analysis options
	cmd.Flags().Bool("preview", false, "Preview what will be restored without downloading")
	cmd.Flags().Bool("estimate-cost", false, "Estimate restoration costs and time")
	cmd.Flags().Bool("list-contents", false, "List archive contents without restoration")
	cmd.Flags().Bool("dry-run", false, "Show what would be restored (same as --preview)")

	// Selection and filtering options
	cmd.Flags().String("pattern", "", "Glob pattern to match file names")
	cmd.Flags().StringSlice("extensions", []string{}, "File extensions to include (e.g. .txt,.json)")
	cmd.Flags().String("min-size", "", "Minimum file size (e.g. 1MB, 500KB)")
	cmd.Flags().String("max-size", "", "Maximum file size (e.g. 1GB, 100MB)")
	cmd.Flags().String("after", "", "Files modified after date (YYYY-MM-DD)")
	cmd.Flags().String("before", "", "Files modified before date (YYYY-MM-DD)")
	cmd.Flags().String("path-pattern", "", "Filter by file path pattern")
	cmd.Flags().Bool("preserve-structure", true, "Preserve original directory structure")
	cmd.Flags().Int("max-files", 0, "Maximum number of files to restore (0 = unlimited)")

	// Output and progress options
	cmd.Flags().String("format", "table", "Output format: table, json, yaml")
	cmd.Flags().Bool("show-metadata", false, "Display detailed file metadata")
	cmd.Flags().Bool("show-checksums", false, "Display file checksums")
	cmd.Flags().Bool("verbose-progress", false, "Show detailed progress information")

	// Cost estimation options
	cmd.Flags().String("storage-class", "", "Target storage class for cost estimation")
	cmd.Flags().String("region", "us-east-1", "AWS region for cost calculation")
	cmd.Flags().Bool("include-transfer-costs", true, "Include data transfer costs in estimates")
	cmd.Flags().Bool("show-cost-breakdown", false, "Show detailed cost breakdown")

	// Index and cache options
	cmd.Flags().StringArray("inventory-directory", []string{"."}, "Directories containing inventory files")
	cmd.Flags().String("index-cache-dir", "", "Directory for index cache (default: temp dir)")
	cmd.Flags().Bool("rebuild-index", false, "Force rebuild of archive indexes")
	cmd.Flags().Bool("no-cache", false, "Disable index caching")

	return cmd
}

// runRestoreCommand executes the restore command with preview and cost estimation
func runRestoreCommand(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	
	// Parse arguments
	location := args[0]
	destination := "."
	if len(args) >= 2 {
		destination = args[1]
	}
	
	// Get command options
	options, err := parseRestoreOptions(cmd)
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
	
	// Initialize restoration engine
	restoreEngine := &RestoreEngine{
		indexer:      indexing.NewIndexer(indexCacheDir, logger),
		searchEngine: indexing.NewSearchEngine(indexing.NewIndexer(indexCacheDir, logger), logger),
		logger:       logger,
	}
	
	// Handle index management flags
	rebuildIndex, _ := cmd.Flags().GetBool("rebuild-index")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	
	if noCache {
		restoreEngine.indexer.ClearCache()
	}
	
	// Ensure index exists for the location
	err = ensureRestoreIndexExists(ctx, restoreEngine.indexer, location, cmd, rebuildIndex)
	if err != nil {
		return fmt.Errorf("failed to prepare restoration index: %w", err)
	}
	
	// Determine operation type
	preview, _ := cmd.Flags().GetBool("preview")
	estimateCost, _ := cmd.Flags().GetBool("estimate-cost")
	listContents, _ := cmd.Flags().GetBool("list-contents")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	
	// Set preview mode if dry-run is specified
	if dryRun {
		preview = true
	}
	
	switch {
	case preview || dryRun:
		return runRestorePreview(ctx, restoreEngine, location, destination, options, cmd)
	case estimateCost:
		return runRestoreCostEstimation(ctx, restoreEngine, location, options, cmd)
	case listContents:
		return runRestoreContentsListing(ctx, restoreEngine, location, options, cmd)
	default:
		// TODO: Implement actual restoration in future version
		return fmt.Errorf("actual restoration not yet implemented - use --preview or --estimate-cost flags")
	}
}

// parseRestoreOptions parses command flags into restore options
func parseRestoreOptions(cmd *cobra.Command) (*RestoreOptions, error) {
	options := &RestoreOptions{}
	
	// Basic restore options
	options.PreserveStructure, _ = cmd.Flags().GetBool("preserve-structure")
	options.ShowMetadata, _ = cmd.Flags().GetBool("show-metadata")
	options.ShowChecksums, _ = cmd.Flags().GetBool("show-checksums")
	options.VerboseProgress, _ = cmd.Flags().GetBool("verbose-progress")
	options.MaxFiles, _ = cmd.Flags().GetInt("max-files")
	
	// Cost estimation options
	options.StorageClass, _ = cmd.Flags().GetString("storage-class")
	options.Region, _ = cmd.Flags().GetString("region")
	options.IncludeTransferCosts, _ = cmd.Flags().GetBool("include-transfer-costs")
	options.ShowCostBreakdown, _ = cmd.Flags().GetBool("show-cost-breakdown")
	
	// Parse search filter for selective restoration
	filter, err := parseRestoreFilter(cmd)
	if err != nil {
		return nil, err
	}
	
	if filter != nil {
		options.Filter = filter
	}
	
	return options, nil
}

// parseRestoreFilter parses command flags into a restoration filter
func parseRestoreFilter(cmd *cobra.Command) (*indexing.SearchFilter, error) {
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
	
	// Result limits for restoration
	if maxFiles, _ := cmd.Flags().GetInt("max-files"); maxFiles > 0 {
		filter.MaxResults = maxFiles
		hasFilters = true
	}
	
	if !hasFilters {
		return nil, nil
	}
	
	return filter, nil
}

// ensureRestoreIndexExists makes sure an index exists for restoration operations
func ensureRestoreIndexExists(ctx context.Context, indexer *indexing.Indexer, location string, cmd *cobra.Command, forceRebuild bool) error {
	// Check if index already exists
	if !forceRebuild {
		if _, err := indexer.LoadIndex(ctx, location); err == nil {
			logger.Debug("using existing index for restoration", "location", location)
			return nil
		}
	}
	
	logger.Info("creating restoration index", "location", location, "force_rebuild", forceRebuild)
	
	// Get inventory directories
	inventoryDirs, err := cmd.Flags().GetStringArray("inventory-directory")
	if err != nil {
		return err
	}
	
	// For restore operations, we might need to handle different location types
	var inventoryFiles []string
	
	if strings.HasPrefix(location, "s3://") {
		// For S3 locations, look for inventory files that match the location
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
		return fmt.Errorf("no inventory files found for restoration indexing in directories: %v", inventoryDirs)
	}
	
	// Load the first inventory file (in a real implementation, we might merge multiple)
	inventoryFile := inventoryFiles[0]
	logger.Info("loading inventory for restoration", "file", inventoryFile)
	
	inv, err := inventory.NewInventoryWithFilename(inventoryFile)
	if err != nil {
		return fmt.Errorf("failed to load inventory from %s: %w", inventoryFile, err)
	}
	
	// Create index from inventory
	archiveIndex, err := indexer.CreateIndex(ctx, inv, location)
	if err != nil {
		return fmt.Errorf("failed to create restoration index: %w", err)
	}
	
	// Save index for future use
	err = indexer.SaveIndex(ctx, archiveIndex)
	if err != nil {
		logger.Warn("failed to save restoration index", "error", err)
		// Continue anyway, we can use the cached version
	}
	
	logger.Info("restoration index created successfully", "location", location, "files", archiveIndex.FileCount)
	return nil
}

// runRestorePreview shows what would be restored without actually doing it
func runRestorePreview(ctx context.Context, engine *RestoreEngine, location string, destination string, options *RestoreOptions, cmd *cobra.Command) error {
	logger.Info("generating restoration preview", "location", location, "destination", destination)
	
	// Get files to restore using search engine
	var files []*indexing.EnhancedFile
	
	if options.Filter != nil {
		result, err := engine.searchEngine.Search(ctx, *options.Filter, location)
		if err != nil {
			return fmt.Errorf("failed to search for files to restore: %w", err)
		}
		files = result.Files
	} else {
		// Get all files from the index
		index, err := engine.indexer.LoadIndex(ctx, location)
		if err != nil {
			return fmt.Errorf("failed to load archive index: %w", err)
		}
		files = index.Files
	}
	
	// Apply max-files limit
	if options.MaxFiles > 0 && len(files) > options.MaxFiles {
		files = files[:options.MaxFiles]
		logger.Info("limiting preview to max files", "max_files", options.MaxFiles, "total_available", len(files))
	}
	
	// Create preview result
	preview := &RestorePreview{
		Location:       location,
		Destination:    destination,
		Files:          files,
		TotalFiles:     len(files),
		TotalSize:      calculateTotalSize(files),
		PreviewTime:    time.Now(),
		EstimatedTime:  estimateRestoreTime(files),
		RequiredSpace:  calculateRequiredSpace(files, options.PreserveStructure),
	}
	
	// Display preview
	return displayRestorePreview(preview, cmd)
}

// runRestoreCostEstimation estimates the costs for restoration
func runRestoreCostEstimation(ctx context.Context, engine *RestoreEngine, location string, options *RestoreOptions, cmd *cobra.Command) error {
	logger.Info("estimating restoration costs", "location", location)
	
	// Get files for cost estimation
	var files []*indexing.EnhancedFile
	
	if options.Filter != nil {
		result, err := engine.searchEngine.Search(ctx, *options.Filter, location)
		if err != nil {
			return fmt.Errorf("failed to search for cost estimation: %w", err)
		}
		files = result.Files
	} else {
		// Get all files from the index
		index, err := engine.indexer.LoadIndex(ctx, location)
		if err != nil {
			return fmt.Errorf("failed to load archive index: %w", err)
		}
		files = index.Files
	}
	
	// Apply max-files limit
	if options.MaxFiles > 0 && len(files) > options.MaxFiles {
		files = files[:options.MaxFiles]
	}
	
	// Calculate cost estimates
	costEstimate := &RestoreCostEstimate{
		Location:           location,
		Files:              files,
		TotalFiles:         len(files),
		TotalSize:          calculateTotalSize(files),
		Region:             options.Region,
		IncludeTransfer:    options.IncludeTransferCosts,
		EstimationTime:     time.Now(),
	}
	
	// Calculate costs based on AWS pricing
	costEstimate.StorageCost = calculateStorageCosts(files, options.StorageClass, options.Region)
	costEstimate.TransferCost = 0.0  // Will be calculated when transfer costs are included
	if options.IncludeTransferCosts {
		costEstimate.TransferCost = calculateTransferCosts(files, options.Region)
	}
	costEstimate.TotalCost = costEstimate.StorageCost + costEstimate.TransferCost
	
	// Display cost estimation
	return displayRestoreCostEstimate(costEstimate, cmd)
}

// runRestoreContentsListing lists the contents of an archive
func runRestoreContentsListing(ctx context.Context, engine *RestoreEngine, location string, options *RestoreOptions, cmd *cobra.Command) error {
	logger.Info("listing archive contents", "location", location)
	
	// Load archive index
	index, err := engine.indexer.LoadIndex(ctx, location)
	if err != nil {
		return fmt.Errorf("failed to load archive index: %w", err)
	}
	
	// Filter files if needed
	var files []*indexing.EnhancedFile
	if options.Filter != nil {
		result, err := engine.searchEngine.Search(ctx, *options.Filter, location)
		if err != nil {
			return fmt.Errorf("failed to filter archive contents: %w", err)
		}
		files = result.Files
	} else {
		files = index.Files
	}
	
	// Create contents listing
	contents := &ArchiveContents{
		Location:      location,
		Files:         files,
		TotalFiles:    len(files),
		TotalSize:     calculateTotalSize(files),
		IndexVersion:  index.IndexVersion,
		CreatedAt:     index.CreatedAt,
	}
	
	// Display contents
	return displayArchiveContents(contents, cmd)
}

// RestoreEngine handles restoration operations
type RestoreEngine struct {
	indexer      *indexing.Indexer
	searchEngine *indexing.SearchEngine
	logger       *slog.Logger
}

// RestoreOptions contains options for restoration operations
type RestoreOptions struct {
	PreserveStructure    bool
	ShowMetadata         bool
	ShowChecksums        bool
	VerboseProgress      bool
	MaxFiles            int
	StorageClass        string
	Region              string
	IncludeTransferCosts bool
	ShowCostBreakdown   bool
	Filter              *indexing.SearchFilter
}

// RestorePreview contains information about what will be restored
type RestorePreview struct {
	Location       string                    `json:"location"`
	Destination    string                    `json:"destination"`
	Files          []*indexing.EnhancedFile `json:"files"`
	TotalFiles     int                      `json:"total_files"`
	TotalSize      int64                    `json:"total_size"`
	PreviewTime    time.Time                `json:"preview_time"`
	EstimatedTime  time.Duration            `json:"estimated_time"`
	RequiredSpace  int64                    `json:"required_space"`
}

// RestoreCostEstimate contains cost estimation for restoration
type RestoreCostEstimate struct {
	Location        string                    `json:"location"`
	Files           []*indexing.EnhancedFile `json:"files"`
	TotalFiles      int                      `json:"total_files"`
	TotalSize       int64                    `json:"total_size"`
	Region          string                   `json:"region"`
	StorageCost     float64                  `json:"storage_cost"`
	TransferCost    float64                  `json:"transfer_cost"`
	TotalCost       float64                  `json:"total_cost"`
	IncludeTransfer bool                     `json:"include_transfer"`
	EstimationTime  time.Time                `json:"estimation_time"`
}

// ArchiveContents contains information about archive contents
type ArchiveContents struct {
	Location     string                    `json:"location"`
	Files        []*indexing.EnhancedFile `json:"files"`
	TotalFiles   int                      `json:"total_files"`
	TotalSize    int64                    `json:"total_size"`
	IndexVersion string                   `json:"index_version"`
	CreatedAt    time.Time                `json:"created_at"`
}

// Helper functions

func calculateTotalSize(files []*indexing.EnhancedFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func estimateRestoreTime(files []*indexing.EnhancedFile) time.Duration {
	// Rough estimation: 10MB/second average restoration speed
	totalSize := calculateTotalSize(files)
	const avgSpeed = 10 * 1024 * 1024 // 10 MB/s
	
	seconds := float64(totalSize) / float64(avgSpeed)
	return time.Duration(seconds) * time.Second
}

func calculateRequiredSpace(files []*indexing.EnhancedFile, preserveStructure bool) int64 {
	// For now, assume 1:1 space requirement
	// In a real implementation, we'd account for compression, deduplication, etc.
	return calculateTotalSize(files)
}

func calculateStorageCosts(files []*indexing.EnhancedFile, storageClass, region string) float64 {
	// Simplified AWS pricing calculation
	// In a real implementation, this would use actual AWS pricing APIs
	totalSizeGB := float64(calculateTotalSize(files)) / (1024 * 1024 * 1024)
	
	var costPerGBMonth float64
	switch storageClass {
	case "STANDARD":
		costPerGBMonth = 0.023
	case "STANDARD_IA":
		costPerGBMonth = 0.0125
	case "GLACIER":
		costPerGBMonth = 0.004
	case "DEEP_ARCHIVE":
		costPerGBMonth = 0.00099
	default:
		costPerGBMonth = 0.023 // Default to STANDARD
	}
	
	return totalSizeGB * costPerGBMonth
}

func calculateTransferCosts(files []*indexing.EnhancedFile, region string) float64 {
	// Simplified transfer cost calculation
	// First 1 GB/month free, then $0.09/GB
	totalSizeGB := float64(calculateTotalSize(files)) / (1024 * 1024 * 1024)
	
	if totalSizeGB <= 1.0 {
		return 0.0
	}
	
	return (totalSizeGB - 1.0) * 0.09
}

// Display functions

func displayRestorePreview(preview *RestorePreview, cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "json":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(preview)
	case "yaml":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(preview)
	case "table":
		fallthrough
	default:
		return displayRestorePreviewTable(preview, cmd)
	}
}

func displayRestoreCostEstimate(estimate *RestoreCostEstimate, cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "json":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(estimate)
	case "yaml":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(estimate)
	case "table":
		fallthrough
	default:
		return displayRestoreCostEstimateTable(estimate, cmd)
	}
}

func displayArchiveContents(contents *ArchiveContents, cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "json":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(contents)
	case "yaml":
		gout.SetWriter(cmd.OutOrStdout())
		return gout.Print(contents)
	case "table":
		fallthrough
	default:
		return displayArchiveContentsTable(contents, cmd)
	}
}

func displayRestorePreviewTable(preview *RestorePreview, cmd *cobra.Command) error {
	showMetadata, _ := cmd.Flags().GetBool("show-metadata")
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n🔍 Restoration Preview\n")
	fmt.Fprintf(cmd.OutOrStdout(), "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", preview.Location)
	fmt.Fprintf(cmd.OutOrStdout(), "Destination: %s\n", preview.Destination)
	fmt.Fprintf(cmd.OutOrStdout(), "Files to restore: %d\n", preview.TotalFiles)
	fmt.Fprintf(cmd.OutOrStdout(), "Total size: %s\n", humanizeBytes(preview.TotalSize))
	fmt.Fprintf(cmd.OutOrStdout(), "Required space: %s\n", humanizeBytes(preview.RequiredSpace))
	fmt.Fprintf(cmd.OutOrStdout(), "Estimated time: %v\n", preview.EstimatedTime.Round(time.Second))
	fmt.Fprintf(cmd.OutOrStdout(), "Preview generated: %s\n", preview.PreviewTime.Format("2006-01-02 15:04:05"))
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n📄 Files to restore:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
	
	// Show first 20 files, then summarize the rest
	filesToShow := preview.Files
	showSummary := false
	if len(filesToShow) > 20 {
		filesToShow = preview.Files[:20]
		showSummary = true
	}
	
	for _, file := range filesToShow {
		fmt.Fprintf(cmd.OutOrStdout(), "📄 %s\n", file.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "   Path: %s\n", file.Destination)
		fmt.Fprintf(cmd.OutOrStdout(), "   Size: %s\n", file.GetHumanSize())
		
		if showMetadata {
			fmt.Fprintf(cmd.OutOrStdout(), "   Type: %s\n", file.ContentType)
			fmt.Fprintf(cmd.OutOrStdout(), "   Storage: %s\n", file.StorageClass)
			fmt.Fprintf(cmd.OutOrStdout(), "   Suitcase: %s\n", file.SuitcaseName)
		}
		
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
	}
	
	if showSummary {
		remaining := len(preview.Files) - len(filesToShow)
		fmt.Fprintf(cmd.OutOrStdout(), "... and %d more files\n", remaining)
		fmt.Fprintf(cmd.OutOrStdout(), "(use --format json to see all files)\n")
	}
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n💡 Use 'cargoship restore %s %s' to perform actual restoration\n", 
		preview.Location, preview.Destination)
	
	return nil
}

func displayRestoreCostEstimateTable(estimate *RestoreCostEstimate, cmd *cobra.Command) error {
	showBreakdown, _ := cmd.Flags().GetBool("show-cost-breakdown")
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n💰 Restoration Cost Estimate\n")
	fmt.Fprintf(cmd.OutOrStdout(), "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", estimate.Location)
	fmt.Fprintf(cmd.OutOrStdout(), "Region: %s\n", estimate.Region)
	fmt.Fprintf(cmd.OutOrStdout(), "Files: %d\n", estimate.TotalFiles)
	fmt.Fprintf(cmd.OutOrStdout(), "Total size: %s\n", humanizeBytes(estimate.TotalSize))
	fmt.Fprintf(cmd.OutOrStdout(), "Estimation time: %s\n", estimate.EstimationTime.Format("2006-01-02 15:04:05"))
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n💵 Cost Breakdown:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Storage costs: $%.4f\n", estimate.StorageCost)
	
	if estimate.IncludeTransfer {
		fmt.Fprintf(cmd.OutOrStdout(), "Transfer costs: $%.4f\n", estimate.TransferCost)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Transfer costs: Not included\n")
	}
	
	fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Total estimated cost: $%.4f\n", estimate.TotalCost)
	
	if showBreakdown {
		fmt.Fprintf(cmd.OutOrStdout(), "\n📊 Detailed Cost Analysis:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
		
		// Group files by storage class for detailed breakdown
		storageClasses := make(map[string][]*indexing.EnhancedFile)
		for _, file := range estimate.Files {
			class := file.StorageClass
			if class == "" {
				class = "STANDARD"
			}
			storageClasses[class] = append(storageClasses[class], file)
		}
		
		for class, files := range storageClasses {
			totalSize := calculateTotalSize(files)
			classCost := calculateStorageCosts(files, class, estimate.Region)
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d files, %s, $%.4f\n", 
				class, len(files), humanizeBytes(totalSize), classCost)
		}
	}
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n⚠️  Estimates are approximate and based on current AWS pricing\n")
	fmt.Fprintf(cmd.OutOrStdout(), "💡 Use 'cargoship restore --preview' to see what will be restored\n")
	
	return nil
}

func displayArchiveContentsTable(contents *ArchiveContents, cmd *cobra.Command) error {
	showMetadata, _ := cmd.Flags().GetBool("show-metadata")
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n📋 Archive Contents\n")
	fmt.Fprintf(cmd.OutOrStdout(), "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Location: %s\n", contents.Location)
	fmt.Fprintf(cmd.OutOrStdout(), "Index version: %s\n", contents.IndexVersion)
	fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", contents.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(cmd.OutOrStdout(), "Total files: %d\n", contents.TotalFiles)
	fmt.Fprintf(cmd.OutOrStdout(), "Total size: %s\n", humanizeBytes(contents.TotalSize))
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n📄 Files:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "───────────────────────────────────────────────────────────────\n")
	
	// Show first 50 files for contents listing
	filesToShow := contents.Files
	showSummary := false
	if len(filesToShow) > 50 {
		filesToShow = contents.Files[:50]
		showSummary = true
	}
	
	for _, file := range filesToShow {
		fmt.Fprintf(cmd.OutOrStdout(), "📄 %s", file.Name)
		
		if showMetadata {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s, %s)", file.GetHumanSize(), file.ContentType)
		}
		
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
	}
	
	if showSummary {
		remaining := len(contents.Files) - len(filesToShow)
		fmt.Fprintf(cmd.OutOrStdout(), "\n... and %d more files\n", remaining)
		fmt.Fprintf(cmd.OutOrStdout(), "(use --format json or yaml to see all files)\n")
	}
	
	fmt.Fprintf(cmd.OutOrStdout(), "\n💡 Use 'cargoship restore --preview' to preview restoration\n")
	
	return nil
}