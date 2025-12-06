package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// NewCreatePipelineCmd creates a new command for uploading with the streaming pipeline
func NewCreatePipelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload SOURCE_DIR...",
		Short: "Upload directories to S3 using streaming pipeline",
		Long: `Upload directories to S3 using the high-performance streaming pipeline.

This command replaces the legacy suitcase/rclone system with a modern streaming
architecture that provides:
- Real-time progress tracking with beautiful TUI
- Multi-prefix S3 parallel uploads (8x throughput improvement)
- Zero local disk usage (streaming directly to S3)
- Automatic compression (zstd)
- Intelligent chunking and sharding`,
		Example: `  # Upload a directory with progress tracking
  cargoship create upload /path/to/data --bucket my-bucket

  # Upload with custom prefix
  cargoship create upload /path/to/data --bucket my-bucket --prefix backups/2025-12-05

  # Quiet mode (no progress display)
  cargoship create upload /path/to/data --bucket my-bucket --quiet

  # JSON progress output (for scripts)
  cargoship create upload /path/to/data --bucket my-bucket --progress-format json`,
		Args:             cobra.MinimumNArgs(1),
		RunE:             createPipelineRunE,
		PreRunE:          createPipelinePreRunE,
		SilenceUsage:     true,
		SilenceErrors:    false,
	}

	// S3 configuration flags
	cmd.Flags().String("bucket", "", "S3 bucket name (required)")
	cmd.Flags().String("prefix", "", "S3 key prefix (optional)")
	cmd.Flags().String("region", "us-west-2", "AWS region")
	cmd.Flags().String("storage-class", "STANDARD", "S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.)")

	// Progress and output flags
	cmd.Flags().Bool("quiet", false, "Disable progress display")
	cmd.Flags().String("progress-format", "tui", "Progress output format: tui, json, text")

	// Performance tuning flags
	cmd.Flags().Int("shards", 8, "Number of S3 prefix shards for parallel uploads")
	cmd.Flags().Int("workers", 4, "Workers per stage (scanner, archiver, uploader)")
	cmd.Flags().Int64("chunk-size-mb", 200, "Target chunk size in MB (0 = adaptive)")

	// Mark required flags
	_ = cmd.MarkFlagRequired("bucket")

	return cmd
}

func createPipelinePreRunE(cmd *cobra.Command, args []string) error {
	// Validate source directories exist
	for _, dir := range args {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("source directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("source path %s is not a directory", dir)
		}
	}

	// Validate bucket name
	bucket, _ := cmd.Flags().GetString("bucket")
	if bucket == "" {
		return fmt.Errorf("--bucket is required")
	}

	// Validate progress format
	progressFormat, _ := cmd.Flags().GetString("progress-format")
	validFormats := map[string]bool{"tui": true, "json": true, "text": true}
	if !validFormats[progressFormat] {
		return fmt.Errorf("invalid progress-format: %s (must be tui, json, or text)", progressFormat)
	}

	return nil
}

func createPipelineRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get flags
	bucket, _ := cmd.Flags().GetString("bucket")
	prefix, _ := cmd.Flags().GetString("prefix")
	region, _ := cmd.Flags().GetString("region")
	storageClass, _ := cmd.Flags().GetString("storage-class")
	quiet, _ := cmd.Flags().GetBool("quiet")
	progressFormat, _ := cmd.Flags().GetString("progress-format")
	shardCount, _ := cmd.Flags().GetInt("shards")
	workers, _ := cmd.Flags().GetInt("workers")
	chunkSizeMB, _ := cmd.Flags().GetInt64("chunk-size-mb")

	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	// Verify bucket exists and is accessible
	_, err = s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		return fmt.Errorf("bucket %s is not accessible: %w", bucket, err)
	}

	// Get absolute paths for all source directories
	var sourceDirs []string
	for _, dir := range args {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("failed to resolve path %s: %w", dir, err)
		}
		sourceDirs = append(sourceDirs, absPath)
	}

	// Create pipeline config
	pipelineConfig := &pipeline.PipelineConfig{
		ScannerWorkers:  workers,
		ArchiverWorkers: workers,
		UploaderWorkers: workers,
		S3Bucket:        bucket,
		S3Prefix:        prefix,
		S3Region:        region,
		UseRealS3:       true,
		S3Client:        s3Client,
		S3StorageClass:  storageClass,
		S3PartSize:      64 * 1024 * 1024, // 64MB parts

		// Multi-prefix optimization (Phase 3)
		EnableMultiPrefix: true,
		ShardCount:        shardCount,
		WorkersPerPrefix:  2,

		// Progress tracking
		EnableProgress:   !quiet && progressFormat == "tui",
		ProgressInterval: 100 * 1000000, // 100ms in nanoseconds

		// Chunking
		ChunkBufferSize:   100,
		ArchiveBufferSize: 100,
		ResultBufferSize:  200,
	}

	// Override chunk size if specified
	if chunkSizeMB > 0 {
		pipelineConfig.ForceChunkSizeMB = int(chunkSizeMB)
	}

	// Create pipeline
	pipe, err := pipeline.NewPipeline(pipelineConfig)
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Setup progress tracking based on format and terminal detection
	if !quiet && progressFormat == "tui" {
		// Check if stdout is a TTY (terminal detection)
		if term.IsTerminal(int(os.Stdout.Fd())) {
			// Register TUI progress callback
			pipe.SetProgressCallback(func(p pipeline.Progress) {
				// Clear line and move cursor to start
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\r\033[K")

				// Calculate throughput
				mbps := float64(p.BytesPerSecond) / (1024 * 1024)

				// Render progress line
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"🚢 Uploading: %d files | %.2f GB | %d chunks | %.1f MB/s | %s elapsed",
					p.FilesProcessed,
					float64(p.BytesProcessed)/(1024*1024*1024),
					p.ChunksCompleted,
					mbps,
					p.ElapsedTime.Round(1000000000), // Round to 1s
				)
			})
		}
		// Note: If not a TTY, progress display is automatically disabled (graceful fallback)
	}

	// Run pipeline for each source directory
	for _, sourceDir := range sourceDirs {

		result, err := pipe.Run(ctx, sourceDir)
		if err != nil {
			return fmt.Errorf("pipeline failed for %s: %w", sourceDir, err)
		}

		if !result.Success {
			return fmt.Errorf("pipeline completed with errors for %s: %v", sourceDir, result.Errors)
		}

		// Print newline after progress display (if TUI mode was active)
		if !quiet && progressFormat == "tui" && term.IsTerminal(int(os.Stdout.Fd())) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}

		// Print summary
		if !quiet {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n✅ Upload complete: %s\n", sourceDir)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Files:      %d\n", result.TotalFiles)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Size:       %.2f GB\n", float64(result.TotalBytes)/(1024*1024*1024))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Duration:   %v\n", result.TotalTime)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Throughput: %.2f MB/s\n", float64(result.TotalBytes)/(1024*1024)/result.TotalTime.Seconds())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   S3 Location: s3://%s/%s\n", bucket, prefix)
		}

		// JSON output mode
		if progressFormat == "json" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "{\"status\":\"success\",\"files\":%d,\"bytes\":%d,\"duration_seconds\":%.2f}\n",
				result.TotalFiles, result.TotalBytes, result.TotalTime.Seconds())
		}
	}

	return nil
}
