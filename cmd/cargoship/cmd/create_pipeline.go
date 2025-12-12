package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
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
		Args:          cobra.MinimumNArgs(1),
		RunE:          createPipelineRunE,
		PreRunE:       createPipelinePreRunE,
		SilenceUsage:  true,
		SilenceErrors: false,
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

	// Network tuning flags
	cmd.Flags().String("network-profile", "default", "Network tuning profile: default, aggressive, conservative")
	cmd.Flags().Bool("http2", true, "Enable HTTP/2")
	cmd.Flags().Int("http2-max-streams", 250, "Max concurrent HTTP/2 streams per connection")
	cmd.Flags().Int("max-idle-conns", 100, "Max idle connections per host")
	cmd.Flags().Duration("idle-conn-timeout", 300*time.Second, "Idle connection timeout")

	// Resume capability flags (Issue #157)
	cmd.Flags().Bool("resume", false, "Resume a previous incomplete upload")
	cmd.Flags().String("upload-id", "", "Upload ID to resume (auto-detect if not specified)")
	cmd.Flags().Bool("skip-existing", false, "Skip chunks that already exist in S3 (HeadObject check)")

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
	networkProfile, _ := cmd.Flags().GetString("network-profile")
	enableHTTP2, _ := cmd.Flags().GetBool("http2")
	maxStreams, _ := cmd.Flags().GetInt("http2-max-streams")
	maxIdleConns, _ := cmd.Flags().GetInt("max-idle-conns")
	idleConnTimeout, _ := cmd.Flags().GetDuration("idle-conn-timeout")
	resumeMode, _ := cmd.Flags().GetBool("resume")
	uploadID, _ := cmd.Flags().GetString("upload-id")
	skipExisting, _ := cmd.Flags().GetBool("skip-existing")

	// Build HTTP transport configuration based on network profile and flags
	var httpConfig *cargoconfig.HTTPTransportConfig
	switch networkProfile {
	case "aggressive":
		httpConfig = cargoconfig.AggressiveHTTPTransportConfig()
	case "conservative":
		httpConfig = cargoconfig.ConservativeHTTPTransportConfig()
	default:
		httpConfig = cargoconfig.DefaultHTTPTransportConfig()
	}

	// Apply CLI flag overrides
	if cmd.Flags().Changed("http2") {
		httpConfig.EnableHTTP2 = enableHTTP2
	}
	if cmd.Flags().Changed("http2-max-streams") {
		httpConfig.MaxConcurrentStreams = maxStreams
	}
	if cmd.Flags().Changed("max-idle-conns") {
		httpConfig.MaxIdleConnsPerHost = maxIdleConns
		httpConfig.MaxConnsPerHost = maxIdleConns
	}
	if cmd.Flags().Changed("idle-conn-timeout") {
		httpConfig.IdleConnTimeout = idleConnTimeout
	}

	// Create S3 client with optimized HTTP transport
	s3Client, err := cargoconfig.GetOrCreateS3Client(ctx, bucket, region, "", httpConfig)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

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

		// Resume capability (Issue #157)
		ResumeMode:                  resumeMode,
		ResumeUploadID:              uploadID,
		SkipExisting:                skipExisting,
		EnablePartialManifest:       true,
		PartialManifestSaveInterval: 30 * time.Second,
		EnableManifest:              true,
		SourcePath:                  sourceDirs[0], // Use first source dir for manifest
	}

	// Override chunk size if specified
	if chunkSizeMB > 0 {
		pipelineConfig.ForceChunkSizeMB = int(chunkSizeMB)
	}

	// Handle resume mode with auto-detect upload ID
	if resumeMode && uploadID == "" {
		// Auto-detect most recent incomplete upload
		fmt.Println("🔍 Auto-detecting most recent incomplete upload...")
		detectedID, err := autoDetectResumeUploadID(ctx, s3Client, bucket, prefix)
		if err != nil {
			return fmt.Errorf("failed to auto-detect upload ID: %w", err)
		}
		if detectedID == "" {
			return fmt.Errorf("no incomplete uploads found - cannot resume")
		}
		pipelineConfig.ResumeUploadID = detectedID
		fmt.Printf("✅ Detected upload ID: %s\n", detectedID)
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
			// Issue #103 Phase 2: Display detailed per-shard error breakdown with classification
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "\n❌ Upload failed for %s\n\n", sourceDir)

			// Create error classifier
			classifier := pipeline.NewErrorClassifier()

			// Group failed jobs by shard and collect error types
			shardErrors := make(map[int][]*pipeline.Job)
			errorTypeCount := make(map[pipeline.ErrorType]int)
			var firstClassifiedError *pipeline.ClassifiedError

			for _, job := range result.FailedJobs {
				shardErrors[job.ShardID] = append(shardErrors[job.ShardID], job)

				// Classify the first error we encounter for summary
				if firstClassifiedError == nil && job.Error != nil {
					firstClassifiedError = classifier.Classify(job.Error)
				}

				// Count error types
				if job.Error != nil {
					classified := classifier.Classify(job.Error)
					errorTypeCount[classified.Type]++
				}
			}

			// Display per-shard status
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Shard Status:\n")
			for shardID := 0; shardID < 8; shardID++ { // Default 8 shards
				jobs := shardErrors[shardID]
				if len(jobs) == 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "  Shard %d (shard-%d): ✅ Success\n", shardID, shardID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "  Shard %d (shard-%d): ❌ Failed (%d jobs)\n", shardID, shardID, len(jobs))
					for _, job := range jobs {
						if job.AttemptNumber > 1 {
							_, _ = fmt.Fprintf(cmd.OutOrStderr(), "    • Job %d: Failed after %d attempts\n", job.ID, job.AttemptNumber)
						} else {
							_, _ = fmt.Fprintf(cmd.OutOrStderr(), "    • Job %d: Failed on first attempt\n", job.ID)
						}

						// Classify and display error with type
						if job.Error != nil {
							classified := classifier.Classify(job.Error)
							_, _ = fmt.Fprintf(cmd.OutOrStderr(), "      Type: %s\n", classified.Type)
							_, _ = fmt.Fprintf(cmd.OutOrStderr(), "      %s\n", classified.UserMessage)
						}
					}
				}
			}

			// Display error summary with most common error type
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "\n📊 Error Summary:\n")
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   Total errors: %d\n", len(result.Errors))

			// Show error type breakdown
			if len(errorTypeCount) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   Error types:\n")
				for errType, count := range errorTypeCount {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "     • %s: %d\n", errType, count)
				}
			}

			// Display troubleshooting tips for the most common error type
			if firstClassifiedError != nil && len(firstClassifiedError.TroubleshootingTips) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "\n💡 Troubleshooting Tips:\n")
				for _, tip := range firstClassifiedError.TroubleshootingTips {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   • %s\n", tip)
				}
			}

			return fmt.Errorf("pipeline completed with %d errors", len(result.Errors))
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

// autoDetectResumeUploadID finds the most recent incomplete upload (Issue #157)
func autoDetectResumeUploadID(ctx context.Context, s3Client *s3.Client, bucket, prefix string) (string, error) {
	// List all partial manifests in the uploads/ directory
	listPrefix := prefix
	if listPrefix != "" {
		listPrefix += "/"
	}
	listPrefix += "uploads/"

	input := &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &listPrefix,
	}

	result, err := s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to list S3 objects: %w", err)
	}

	// Find all partial manifest files and extract upload IDs with timestamps
	type partialManifest struct {
		uploadID     string
		lastModified time.Time
	}
	var manifests []partialManifest

	for _, obj := range result.Contents {
		key := *obj.Key
		// Check if it's a partial manifest: uploads/{uploadID}/manifest.partial.json.gz
		if len(key) > len("manifest.partial.json.gz") {
			if key[len(key)-len("manifest.partial.json.gz"):] == "manifest.partial.json.gz" {
				// Extract upload ID from key path
				parts := make([]string, 0)
				start := 0
				for i := 0; i < len(key); i++ {
					if key[i] == '/' {
						if i > start {
							parts = append(parts, key[start:i])
						}
						start = i + 1
					}
				}
				if start < len(key) {
					parts = append(parts, key[start:])
				}

				// Parts should be: [prefix?, "uploads", uploadID, "manifest.partial.json.gz"]
				if len(parts) >= 3 {
					uploadID := parts[len(parts)-2]
					manifests = append(manifests, partialManifest{
						uploadID:     uploadID,
						lastModified: *obj.LastModified,
					})
				}
			}
		}
	}

	if len(manifests) == 0 {
		return "", nil
	}

	// Find the most recent partial manifest
	mostRecent := manifests[0]
	for i := 1; i < len(manifests); i++ {
		if manifests[i].lastModified.After(mostRecent.lastModified) {
			mostRecent = manifests[i]
		}
	}

	return mostRecent.uploadID, nil
}
