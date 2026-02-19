package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/observability/metrics"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// NewUploadCmd creates the 'upload' command for CargoHold uploads with sharding (Issue #95)
func NewUploadCmd() *cobra.Command {
	var (
		bucket           string
		region           string
		storageClass     string
		shardCount       int
		shardStrategy    string
		compressionLevel int
		quiet            bool
		interactive      bool // Issue #112: Interactive TUI mode

		// v0.6.2: Advanced transporter configuration
		transporterType    string
		enableOptimization bool
		congestionControl  string
		disableStaging     bool

		// Issue #155: Observability configuration
		enableTracing     bool
		tracingExporter   string
		tracingEndpoint   string
		tracingSampleRate float64
		prometheusAddr    string

		// Issue #163: Encryption configuration
		kmsKeyID        string
		encryptManifest bool

		// Issue #119: Resume configuration
		forceRestart bool

		// Issue #168: Skip confirmation prompts
		skipConfirmation bool

		// Issue #179: Incremental sync
		incrementalMode bool
		prevManifest    string

		// Issue #180: DVC .dvc file generation
		generateDVCFiles bool
		dvcCacheDir      string
		dvcOutputDir     string

		// Issue #183: DVC budget integration
		dvcProject  string
		uploadTags  []string // "key=value" pairs
	)

	cmd := &cobra.Command{
		Use:   "upload SOURCE_DIR DESTINATION",
		Short: "Upload directory to S3 with CargoHold sharding",
		Long: `Upload a directory to S3 using CargoHold's intelligent sharding system.

CargoHold divides large datasets into multiple shards for parallel uploads,
providing:
- Intelligent shard distribution (hash, size, type, or directory-based)
- Per-shard compression with configurable levels (zstd)
- Parallel uploads for maximum throughput
- Automatic manifest generation for easy restore
- Progress tracking with per-shard visibility

Shard Strategies:
  hash      - Hash-based distribution (balanced, default)
  size      - Size-based distribution (large files in separate shards)
  type      - File type distribution (group by extension)
  directory - Directory-based distribution (keep directories together)

Examples:
  # Upload with default settings (10 shards, hash strategy, compression level 3)
  cargoship upload /data s3://my-bucket/dataset

  # Upload with custom shard count and strategy
  cargoship upload /data s3://my-bucket/dataset --shard-count 20 --shard-strategy size

  # Upload with maximum compression
  cargoship upload /data s3://my-bucket/dataset --compression-level 19

  # Quiet mode (no progress display)
  cargoship upload /data s3://my-bucket/dataset --quiet
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Parse arguments
			sourceDir := args[0]
			destination := args[1]

			// Validate source directory exists
			info, err := os.Stat(sourceDir)
			if err != nil {
				return fmt.Errorf("source directory %s: %w", sourceDir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source path %s is not a directory", sourceDir)
			}

			// Parse destination (s3://bucket/prefix format)
			bucket, prefix, err := parseS3Destination(destination)
			if err != nil {
				return fmt.Errorf("invalid destination: %w", err)
			}

			// Validate shard strategy
			validStrategies := map[string]bool{
				"hash":      true,
				"size":      true,
				"type":      true,
				"directory": true,
			}
			if !validStrategies[shardStrategy] {
				return fmt.Errorf("invalid shard-strategy: %s (must be hash, size, type, or directory)", shardStrategy)
			}

			// Validate compression level (zstd range: 1-22, recommended: 1-19)
			if compressionLevel < 1 || compressionLevel > 22 {
				return fmt.Errorf("compression-level must be between 1 and 22 (zstd range)")
			}

			// Validate shard count (0 = auto, 4-32 = manual)
			if shardCount < 0 || shardCount > 32 {
				return fmt.Errorf("shard-count must be between 0 (auto) and 32")
			}

			// Validate encryption flags (Issue #163)
			if encryptManifest && kmsKeyID == "" {
				return fmt.Errorf("--encrypt-manifest requires --kms-key-id to be set")
			}

			// Validate auto-tier flag (Issue #32)
			autoTier, _ := cmd.Flags().GetBool("auto-tier")
			if autoTier && cmd.Flags().Changed("storage-class") {
				return fmt.Errorf("cannot use both --auto-tier and --storage-class (they are mutually exclusive)")
			}

			// Create S3 client with optimized HTTP transport
			httpConfig := cargoconfig.DefaultHTTPTransportConfig()
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

			// Get absolute path for source directory
			absPath, err := filepath.Abs(sourceDir)
			if err != nil {
				return fmt.Errorf("failed to resolve path %s: %w", sourceDir, err)
			}

			// Issue #106: Auto-detect optimal shard count if not specified
			if shardCount == 0 {
				if !quiet {
					fmt.Println("🔍 Analyzing workload for optimal shard count...")
				}

				calculator := pipeline.NewAdaptiveShardCalculator()
				result, err := calculator.CalculateOptimalShardCount(ctx, absPath)
				if err != nil {
					// Non-fatal: Fall back to default
					if !quiet {
						fmt.Printf("⚠️  Auto-detection failed (%v), using default: 8 shards\n\n", err)
					}
					shardCount = 8
				} else {
					shardCount = result.ShardCount

					if !quiet {
						fmt.Printf("\n📊 Auto-selected shard count: %d\n\n", shardCount)
						fmt.Println(result.Rationale)

						// Print warnings if any
						for _, warning := range result.Warnings {
							fmt.Printf("⚠️  %s\n", warning)
						}
						fmt.Println()
					}
				}
			}

			// Issue #119: Auto-detect interrupted uploads
			if !forceRestart {
				detectedState, err := resume.DetectInterruptedUpload(absPath, bucket, prefix)
				if err != nil {
					fmt.Printf("⚠️  Warning: Failed to check for interrupted uploads: %v\n", err)
				} else if detectedState != nil && resume.ShouldPromptForResume(detectedState) {
					// Found an interrupted upload - prompt user
					if promptForResume(detectedState) {
						fmt.Println("\n⚠️  Direct resume via upload command not yet fully implemented")
						fmt.Printf("💡 Use: cargoship resume %s\n", detectedState.UploadID)
						fmt.Println("    Or use --force-restart to ignore saved state and start fresh")
						return fmt.Errorf("resume via upload command pending implementation")
					}
					// User chose not to resume - continue with fresh upload
					fmt.Println("Starting fresh upload...")
				}
			}

			// v0.6.2: Create advanced transporter if requested
			var transporter interface{} // s3transport.BasicTransporter
			if transporterType != "" && transporterType != "none" {
				// Create S3 config for transporter
				s3Config := cargoconfig.S3Config{
					Bucket:             bucket,
					MultipartChunkSize: 64 * 1024 * 1024, // 64MB
					Concurrency:        4,
					KMSKeyID:           kmsKeyID, // Issue #163: KMS encryption
				}

				// Create transporter config
				transporterConfig := pipeline.TransporterConfig{
					Type:               pipeline.TransporterType(transporterType),
					S3Client:           s3Client,
					S3Config:           s3Config,
					EnableOptimization: enableOptimization,
					CongestionControl:  congestionControl,
					DisableStaging:     disableStaging,
					Logger:             nil, // Use default logger
				}

				// Create transporter
				transporter, err = pipeline.NewPipelineTransporter(transporterConfig)
				if err != nil {
					return fmt.Errorf("failed to create transporter: %w", err)
				}

				if !quiet {
					fmt.Printf("🚀 Advanced Transporter: %s\n", transporterType)
					if enableOptimization {
						fmt.Printf("   Optimization:      enabled (%s)\n", congestionControl)
					}
					if !disableStaging && (transporterType == "staging" || transporterType == "adaptive") {
						fmt.Printf("   Adaptive Staging:  enabled\n")
					}
					fmt.Printf("\n")
				}
			}

			// Issue #155: Initialize observability (tracing and metrics)
			var metricsCollector *metrics.PrometheusCollector

			// Initialize distributed tracing if enabled
			if enableTracing {
				tracingConfig := tracing.Config{
					Enabled:        true,
					ExporterType:   tracingExporter,
					Endpoint:       tracingEndpoint,
					SampleRate:     tracingSampleRate,
					ServiceName:    "cargoship",
					ServiceVersion: "v0.6.2",
				}

				tracerProvider, err := tracing.NewTracerProvider(ctx, tracingConfig)
				if err != nil {
					return fmt.Errorf("failed to initialize tracing: %w", err)
				}
				if tracerProvider != nil {
					defer func() {
						shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
							_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to shutdown tracer: %v\n", err)
						}
					}()
				}

				if !quiet {
					fmt.Printf("🔍 Distributed Tracing: enabled\n")
					fmt.Printf("   Exporter:          %s\n", tracingExporter)
					if tracingEndpoint != "" {
						fmt.Printf("   Endpoint:          %s\n", tracingEndpoint)
					}
					fmt.Printf("   Sample Rate:       %.0f%%\n\n", tracingSampleRate*100)
				}
			}

			// Initialize Prometheus metrics if enabled
			if prometheusAddr != "" {
				metricsCollector = metrics.NewPrometheusCollector()

				// Start metrics HTTP server in background
				go func() {
					if err := metricsCollector.ServeMetrics(prometheusAddr); err != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: metrics server failed: %v\n", err)
					}
				}()

				defer func() {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := metricsCollector.Shutdown(shutdownCtx); err != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to shutdown metrics server: %v\n", err)
					}
				}()

				if !quiet {
					fmt.Printf("📊 Prometheus Metrics: enabled\n")
					fmt.Printf("   Endpoint:          %s\n\n", metricsCollector.GetMetricsURL())
				}

				// Record upload start
				metricsCollector.RecordUploadStart()
			}

			// Create KMS client if encryption is enabled (Issue #163)
			var kmsClient *kms.Client
			if encryptManifest && kmsKeyID != "" {
				// Use same config as S3 client for consistency
				cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
				if err != nil {
					return fmt.Errorf("failed to load AWS config for KMS: %w", err)
				}
				kmsClient = kms.NewFromConfig(cfg)
			}

			// Issue #32: Build TierSelector if --auto-tier is enabled
			var tierSelector *pipeline.StorageTierSelector
			if autoTier {
				hotDays, _ := cmd.Flags().GetInt("tier-hot-days")
				coldDays, _ := cmd.Flags().GetInt("tier-cold-days")
				archiveDays, _ := cmd.Flags().GetInt("tier-archive-days")

				// Issue #168: Get and validate tier-max if specified
				tierMaxStr, _ := cmd.Flags().GetString("tier-max")
				var tierMax types.StorageClass
				if tierMaxStr != "" {
					// Validate tier-max value
					validTiers := map[string]types.StorageClass{
						"STANDARD":     types.StorageClassStandard,
						"STANDARD_IA":  types.StorageClassStandardIa,
						"GLACIER":      types.StorageClassGlacier,
						"DEEP_ARCHIVE": types.StorageClassDeepArchive,
					}
					var ok bool
					tierMax, ok = validTiers[strings.ToUpper(tierMaxStr)]
					if !ok {
						return fmt.Errorf("invalid --tier-max: %q (must be STANDARD, STANDARD_IA, GLACIER, or DEEP_ARCHIVE)", tierMaxStr)
					}
				}

				tierSelector = &pipeline.StorageTierSelector{
					Enabled:         true,
					DefaultClass:    types.StorageClassStandard,
					HotDays:         hotDays,
					ColdDays:        coldDays,
					ArchiveDays:     archiveDays,
					MaxTier:         tierMax, // Issue #168: Apply tier cap
					FallbackToMtime: true,
				}

				if !quiet {
					fmt.Println("📊 Automatic storage tier selection enabled")
					fmt.Printf("   Hot threshold:     %d days (STANDARD)\n", hotDays)
					fmt.Printf("   Cold threshold:    %d days (GLACIER)\n", coldDays)
					fmt.Printf("   Archive threshold: %d days (DEEP_ARCHIVE)\n", archiveDays)
					if tierMax != "" {
						fmt.Printf("   Maximum tier:      %s (capped)\n", tierMax)
					}
					fmt.Println()
				}
			}

			// Issue #183: Parse --tag key=value flags into a map.
			uploadTagMap := make(map[string]string)
			for _, t := range uploadTags {
				parts := strings.SplitN(t, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid --tag value %q: expected key=value", t)
				}
				uploadTagMap[parts[0]] = parts[1]
			}

			// Issue #179: Incremental sync — determine which files need uploading.
			var includeOnlyFiles []string
			var prevUploadID string
			if incrementalMode && prevManifest != "" {
				prev, err := manifest.LoadManifestFromFile(prevManifest)
				if err != nil {
					return fmt.Errorf("load previous manifest: %w", err)
				}
				prevUploadID = prev.UploadID

				incScanner, err := pipeline.NewIncrementalScanner(prev, "")
				if err != nil {
					return fmt.Errorf("create incremental scanner: %w", err)
				}

				toUpload, scanErr := incScanner.FilterFiles(absPath)
				if scanErr != nil && !quiet {
					fmt.Printf("⚠️  Incremental scan errors (proceeding): %v\n", scanErr)
				}
				includeOnlyFiles = toUpload

				if !quiet {
					stats := incScanner.Stats()
					fmt.Printf("📊 Incremental Scan:\n")
					fmt.Printf("   Files scanned:     %d\n", stats.FilesScanned)
					fmt.Printf("   Unchanged (skip):  %d (%.2f GB saved)\n",
						stats.FilesSkipped, float64(stats.BytesSaved)/(1024*1024*1024))
					fmt.Printf("   Changed (upload):  %d\n\n", stats.FilesUploaded)
				}
			}

			// Issue #164: Get tier chunking strategy
			tierStrategy, _ := cmd.Flags().GetString("tier-strategy")
			if tierStrategy != "youngest-file" && tierStrategy != "tier-aware" {
				return fmt.Errorf("invalid tier-strategy: %q (must be 'youngest-file' or 'tier-aware')", tierStrategy)
			}
			if tierStrategy == "tier-aware" && !autoTier {
				return fmt.Errorf("--tier-strategy=tier-aware requires --auto-tier to be enabled")
			}

			// Issue #168: Show cost warning and prompt for confirmation when using tier-aware
			if tierStrategy == "tier-aware" && !skipConfirmation && !quiet {
				fmt.Println("\n⚠️  TIER-AWARE CHUNKING: COST IMPLICATIONS WARNING")
				fmt.Println("══════════════════════════════════════════════════")
				fmt.Println("\nTier-aware chunking will assign files to cost-optimized storage tiers:")
				fmt.Println("\n📦 STANDARD:")
				fmt.Println("   • No minimum duration")
				fmt.Println("   • Free retrieval")
				fmt.Println("   • Immediate access")
				fmt.Println("\n📦 STANDARD_IA (Infrequent Access):")
				fmt.Println("   • 30-day minimum storage duration")
				fmt.Println("   • $0.01/GB retrieval fee")
				fmt.Println("   • Immediate access")
				fmt.Println("\n🧊 GLACIER:")
				fmt.Println("   • 90-day minimum storage duration (early deletion penalty applies)")
				fmt.Println("   • $0.01/GB retrieval fee")
				fmt.Println("   • 3-5 hour retrieval time (standard)")
				fmt.Println("   • Best for: Data accessed <1x per year")
				fmt.Println("\n❄️  DEEP_ARCHIVE:")
				fmt.Println("   • 180-day minimum storage duration (early deletion penalty applies)")
				fmt.Println("   • $0.02/GB retrieval fee")
				fmt.Println("   • 12 hour retrieval time (standard)")
				fmt.Println("   • Best for: Compliance/long-term archives accessed <1x per 5 years")
				fmt.Println("\n💡 Important:")
				fmt.Println("   • Frequent access can negate storage savings due to retrieval fees")
				fmt.Println("   • Ensure 6+ month retention to avoid early deletion penalties")
				fmt.Println("   • Total cost = storage + retrieval + data transfer")
				fmt.Println("\n📚 More info: https://github.com/scttfrdmn/cargoship/issues/168")
				fmt.Println("\nDo you understand these cost implications and wish to proceed? [y/N]: ")

				var response string
				_, _ = fmt.Scanln(&response) // Ignore error - empty input treated as "no"
				response = strings.ToLower(strings.TrimSpace(response))

				if response != "y" && response != "yes" {
					return fmt.Errorf("tier-aware chunking cancelled by user (use --yes to skip this prompt)")
				}
				fmt.Println("✓ Proceeding with tier-aware chunking")
			}

			// Create pipeline config with CargoHold settings
			pipelineConfig := &pipeline.PipelineConfig{
				ScannerWorkers:       4,
				ArchiverWorkers:      4,
				UploaderWorkers:      4,
				S3Bucket:             bucket,
				S3Prefix:             prefix,
				S3Region:             region,
				UseRealS3:            true,
				S3Client:             s3Client,
				S3StorageClass:       storageClass,
				TierSelector:         tierSelector,     // Issue #32: Automatic tier selection
				TierChunkingStrategy: tierStrategy,     // Issue #164: Tier chunking strategy
				S3PartSize:           64 * 1024 * 1024, // 64MB parts

				// Issue #163: KMS encryption configuration
				KMSKeyID:        kmsKeyID,
				EncryptManifest: encryptManifest,
				KMSClient:       kmsClient,

				// Issue #108: Deduplication configuration
				EnableDeduplication: func() bool { v, _ := cmd.Flags().GetBool("enable-dedup"); return v }(),

				// Issue #179: Incremental sync configuration
				IncludeOnlyFiles: includeOnlyFiles,
				SyncType: func() string {
					if incrementalMode {
						return "incremental"
					}
					return "full"
				}(),
				PreviousUploadID: prevUploadID,

				// v0.6.2: Advanced transporter
				Transporter: transporter,

				// CargoHold sharding configuration (Issue #95)
				EnableMultiPrefix: true,
				ShardCount:        shardCount,
				WorkersPerPrefix:  2,

				// Progress tracking
				EnableProgress:   !quiet,
				ProgressInterval: 100 * 1000000, // 100ms in nanoseconds

				// Chunking and buffering
				ChunkBufferSize:   100,
				ArchiveBufferSize: 100,
				ResultBufferSize:  200,

				// Manifest generation
				EnableManifest:              true,
				EnablePartialManifest:       true,
				PartialManifestSaveInterval: 30 * time.Second,
				SourcePath:                  absPath,

				// Cleanup on failure
				CleanupOnFailure: true,

				// Issue #166: Direct upload optimization
				EnableDirectUpload:     cmd.Flags().Changed("direct-upload"),
				ForceDirectUpload:      cmd.Flags().Changed("force-direct-upload"),
				EnableAutoDirectUpload: true, // Auto-enable when thresholds met

				// Issue #183: DVC budget integration
				ProjectID: dvcProject,
				Tags:      uploadTagMap,
			}

			// Note: Compression level and shard strategy are not yet implemented in the pipeline
			// These will be added in future enhancements
			// For now, we log them for visibility
			if !quiet {
				fmt.Printf("🚢 CargoHold Upload Configuration:\n")
				fmt.Printf("   Source:            %s\n", absPath)
				fmt.Printf("   Destination:       s3://%s/%s\n", bucket, prefix)
				// Show whether shard count was auto-selected or manual
				if cmd.Flags().Changed("shard-count") {
					fmt.Printf("   Shard Count:       %d (manual)\n", shardCount)
				} else {
					fmt.Printf("   Shard Count:       %d (auto-selected)\n", shardCount)
				}
				fmt.Printf("   Shard Strategy:    %s\n", shardStrategy)
				fmt.Printf("   Compression Level: %d (zstd)\n", compressionLevel)
				fmt.Printf("   Storage Class:     %s\n\n", storageClass)
			}

			// Create pipeline
			pipe, err := pipeline.NewPipeline(pipelineConfig)
			if err != nil {
				return fmt.Errorf("failed to create pipeline: %w", err)
			}

			// Setup progress tracking with per-shard visibility
			if !quiet {
				// Check if stdout is a TTY (terminal detection)
				if term.IsTerminal(int(os.Stdout.Fd())) {
					// Register TUI progress callback with shard info
					pipe.SetProgressCallback(func(p pipeline.Progress) {
						// Clear line and move cursor to start
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\r\033[K")

						// Calculate throughput
						mbps := float64(p.BytesPerSecond) / (1024 * 1024)

						// Render progress line
						_, _ = fmt.Fprintf(cmd.OutOrStdout(),
							"🚢 Uploading: %d files | %.2f GB | %d chunks | %.1f MB/s | %s",
							p.FilesProcessed,
							float64(p.BytesProcessed)/(1024*1024*1024),
							p.ChunksCompleted,
							mbps,
							p.ElapsedTime.Round(time.Second),
						)
					})
				}
			}

			// Run pipeline
			result, err := pipe.Run(ctx, absPath)
			if err != nil {
				// Record error metrics if enabled
				if metricsCollector != nil {
					metricsCollector.RecordUploadError(ctx, result.UploadID, "pipeline_error", "pipeline")
				}
				return fmt.Errorf("pipeline failed: %w", err)
			}

			if !result.Success {
				// Record error metrics if enabled
				if metricsCollector != nil {
					metricsCollector.RecordUploadError(ctx, result.UploadID, "upload_failed", "pipeline")
				}

				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "\n❌ Upload failed\n")
				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   Errors: %d\n", len(result.Errors))
				for i, err := range result.Errors {
					if i < 5 { // Show first 5 errors
						_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   • %v\n", err)
					}
				}
				if len(result.Errors) > 5 {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "   ... and %d more errors\n", len(result.Errors)-5)
				}
				return fmt.Errorf("pipeline completed with %d errors", len(result.Errors))
			}

			// Issue #180: Generate DVC .dvc sidecar files after a successful upload.
			if generateDVCFiles && result.Success {
				outDir := dvcOutputDir
				if outDir == "" {
					outDir = absPath // default: write .dvc files alongside source data
				}
				dvcOpts := &manifest.DVCGenerateOptions{CacheDir: dvcCacheDir}

				// Retrieve the completed manifest to generate .dvc files from it.
				// The pipeline exposes the finalized manifest via GetManifest().
				if m := pipe.GetManifest(); m != nil {
					n, dvcErr := m.GenerateDVCFiles(outDir, dvcOpts)
					if dvcErr != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStderr(),
							"⚠️  DVC file generation failed: %v\n", dvcErr)
					} else if !quiet {
						fmt.Printf("📄 DVC files generated: %d .dvc files → %s\n\n", n, outDir)
					}
				} else if !quiet {
					fmt.Println("⚠️  DVC file generation skipped: manifest not available")
				}
			}

			// Record success metrics if enabled
			if metricsCollector != nil {
				transporterName := "basic"
				if transporterType != "" && transporterType != "none" {
					transporterName = transporterType
				}
				metricsCollector.RecordUploadComplete(
					ctx,
					result.UploadID,
					result.TotalBytes,
					result.TotalTime,
					storageClass,
					transporterName,
				)
			}

			// Print newline after progress display (if TUI mode was active)
			if !quiet && term.IsTerminal(int(os.Stdout.Fd())) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}

			// Print success summary
			if !quiet {
				fmt.Printf("\n✅ Upload complete!\n\n")
				fmt.Printf("📊 Summary:\n")
				fmt.Printf("   Files Uploaded:    %d\n", result.TotalFiles)
				fmt.Printf("   Total Size:        %.2f GB\n", float64(result.TotalBytes)/(1024*1024*1024))
				fmt.Printf("   Shards Created:    %d\n", shardCount)
				fmt.Printf("   Duration:          %v\n", result.TotalTime)
				fmt.Printf("   Average Throughput: %.2f MB/s\n\n", float64(result.TotalBytes)/(1024*1024)/result.TotalTime.Seconds())
				fmt.Printf("📍 Location:\n")
				fmt.Printf("   S3 URL:            s3://%s/%s\n", bucket, prefix)
				fmt.Printf("   Manifest:          s3://%s/%s/uploads/%s/manifest.json.gz\n\n", bucket, prefix, result.UploadID)
				fmt.Printf("💡 Next steps:\n")
				fmt.Printf("   • Verify upload:   cargoship verify s3://%s/%s/uploads/%s\n", bucket, prefix, result.UploadID)
				fmt.Printf("   • View info:       cargoship info s3://%s/%s/uploads/%s\n", bucket, prefix, result.UploadID)
				fmt.Printf("   • List files:      cargoship list s3://%s/%s/uploads/%s\n", bucket, prefix, result.UploadID)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (or use s3:// URL in DESTINATION)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.)")

	// Issue #32: Automatic storage tier selection based on file access time
	cmd.Flags().Bool("auto-tier", false, "Enable automatic storage tier selection based on file access time")
	cmd.Flags().Int("tier-hot-days", 30, "Days since access to consider 'hot' (STANDARD)")
	cmd.Flags().Int("tier-cold-days", 90, "Days since access to consider 'cold' (GLACIER)")
	cmd.Flags().Int("tier-archive-days", 180, "Days since access to consider 'archive' (DEEP_ARCHIVE)")

	// Issue #168: Limit maximum tier selection
	cmd.Flags().String("tier-max", "", "Maximum storage tier (STANDARD, STANDARD_IA, GLACIER, DEEP_ARCHIVE) - prevents automatic selection of more restrictive tiers")

	// Issue #164: Tier chunking strategy (opt-in tier-aware chunking)
	cmd.Flags().String("tier-strategy", "youngest-file", `Tier chunking strategy (requires --auto-tier):
  youngest-file: Conservative strategy - assigns tier based on youngest file per chunk (default)
  tier-aware:    Optimal cost - groups files by tier before chunking (30-60% savings)

⚠️  WARNING: tier-aware uses GLACIER/DEEP_ARCHIVE with cost implications:
  • GLACIER: 90-day minimum storage ($0.004/GB-month, $0.01/GB retrieval, 3-5hr access)
  • DEEP_ARCHIVE: 180-day minimum ($0.00099/GB-month, $0.02/GB retrieval, 12hr access)
  • Early deletion penalties apply if removed before minimum duration
  • Best for long-term archives accessed <1x per year

  See: https://github.com/scttfrdmn/cargoship/issues/168`)

	cmd.Flags().IntVar(&shardCount, "shard-count", 0, "Number of shards for parallel uploads (0=auto, 4-32=manual, default: 0)")
	cmd.Flags().StringVar(&shardStrategy, "shard-strategy", "hash", "Shard distribution strategy (hash, size, type, directory)")
	cmd.Flags().IntVar(&compressionLevel, "compression-level", 3, "Zstd compression level (1-22, recommended 1-19)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Disable progress display")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Enable interactive TUI mode with per-shard progress (Issue #112)")

	// v0.6.2: Advanced transporter flags
	cmd.Flags().StringVar(&transporterType, "transporter", "staging", "S3 transporter type: basic, staging, adaptive, optimized, none")
	cmd.Flags().BoolVar(&enableOptimization, "optimization", true, "Enable optimization features (BBR/CUBIC, adaptive staging, BDP)")
	cmd.Flags().StringVar(&congestionControl, "congestion-control", "auto", "Congestion control algorithm: bbr, cubic, auto")
	cmd.Flags().BoolVar(&disableStaging, "disable-staging", false, "Disable adaptive staging (reduces memory usage)")

	// Issue #155: Observability flags
	cmd.Flags().BoolVar(&enableTracing, "tracing", false, "Enable distributed tracing")
	cmd.Flags().StringVar(&tracingExporter, "tracing-exporter", "stdout", "Tracing exporter: stdout, jaeger, otlp, none")
	cmd.Flags().StringVar(&tracingEndpoint, "tracing-endpoint", "", "Tracing endpoint URL (required for jaeger/otlp exporters)")
	cmd.Flags().Float64Var(&tracingSampleRate, "tracing-sample-rate", 1.0, "Trace sampling rate (0.0-1.0, default: 1.0 = 100%)")
	cmd.Flags().StringVar(&prometheusAddr, "prometheus-addr", "", "Prometheus metrics HTTP address (e.g., :9090)")

	// Issue #163: Encryption flags
	cmd.Flags().StringVar(&kmsKeyID, "kms-key-id", "", "AWS KMS key ID or ARN for encryption (data chunks encrypted with SSE-KMS)")
	cmd.Flags().BoolVar(&encryptManifest, "encrypt-manifest", false, "Encrypt manifest with KMS envelope encryption (requires --kms-key-id)")

	// Issue #108: Deduplication flag
	cmd.Flags().Bool("enable-dedup", false, "Enable cross-shard file deduplication (10-30% space savings for redundant datasets)")

	// Issue #119: Resume configuration
	cmd.Flags().BoolVar(&forceRestart, "force-restart", false, "Ignore saved state and start fresh upload (bypasses resume detection)")

	// Issue #168: Skip confirmation prompts (for automation)
	cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompts (auto-accept warnings)")

	// Issue #166: Direct upload optimization (fast path for small files)
	cmd.Flags().Bool("direct-upload", false, "Enable direct upload mode (bypasses archiving/compression for small files)")
	cmd.Flags().Bool("force-direct-upload", false, "Force direct upload regardless of thresholds (for benchmarking)")
	cmd.Flags().Int("direct-upload-threshold-mb", 500, "Max total size in MB for auto direct upload (default: 500)")
	cmd.Flags().Int("direct-upload-workers", 256, "Worker count for direct upload (default: 256)")

	// Issue #179: Incremental sync flags
	cmd.Flags().BoolVar(&incrementalMode, "incremental", false, "Enable incremental sync: only upload new or changed files")
	cmd.Flags().StringVar(&prevManifest, "prev-manifest", "", "Path to previous manifest JSON (or .json.gz) for incremental sync")

	// Issue #180: DVC .dvc file generation
	cmd.Flags().BoolVar(&generateDVCFiles, "generate-dvc-files", false, "Generate DVC sidecar .dvc files after upload")
	cmd.Flags().StringVar(&dvcCacheDir, "dvc-cache-dir", ".dvc/cache", "Local DVC cache directory (recorded in manifest; default: .dvc/cache)")
	cmd.Flags().StringVar(&dvcOutputDir, "dvc-output-dir", "", "Directory to write .dvc files (default: source directory)")

	// Issue #183: DVC budget integration
	cmd.Flags().StringVar(&dvcProject, "project", "", "Project ID for cost tracking (e.g. 'dvc_cache' for DVC remotes)")
	cmd.Flags().StringArrayVar(&uploadTags, "tag", nil, "Custom tag in key=value format, repeatable (e.g. --tag dvc_cache=true --tag env=prod)")

	return cmd
}

// parseS3Destination parses s3://bucket/prefix format
func parseS3Destination(dest string) (bucket, prefix string, err error) {
	if len(dest) < 5 || dest[:5] != "s3://" {
		return "", "", fmt.Errorf("destination must start with s3://")
	}

	pathStr := dest[5:]
	slashIdx := -1
	for i, c := range pathStr {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		// Just bucket, no prefix
		return pathStr, "", nil
	}

	bucket = pathStr[:slashIdx]
	prefix = pathStr[slashIdx+1:]

	// Remove trailing slash from prefix
	if len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}

	return bucket, prefix, nil
}

// promptForResume displays upload information and prompts user to resume
// Returns true if user wants to resume, false to start fresh
func promptForResume(state *resume.UploadState) bool {
	// Display upload information
	fmt.Println("\n📦 Detected interrupted upload:")
	fmt.Printf("   Upload ID:       %s\n", state.UploadID)
	fmt.Printf("   Source:          %s\n", state.SourceDir)
	fmt.Printf("   Destination:     s3://%s/%s\n", state.Bucket, state.Prefix)
	fmt.Printf("   Progress:        %.1f%% complete\n", state.Progress())
	fmt.Printf("   Started:         %s ago\n", formatDuration(state.Age()))

	if state.TotalFiles > 0 {
		fmt.Printf("   Files:           %d / %d completed\n", state.CompletedFiles, state.TotalFiles)
	}

	// Check if terminal is interactive
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Non-interactive mode - don't resume by default
		fmt.Println("\n⚠️  Non-interactive terminal detected - skipping resume prompt")
		fmt.Println("   Use --force-restart to explicitly start fresh")
		return false
	}

	// Prompt user
	fmt.Print("\nResume this upload? [Y/n]: ")
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		// On input error, default to not resuming
		return false
	}

	// Empty or "y" means yes
	return response == "" || response == "y" || response == "Y"
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
	return fmt.Sprintf("%.1f days", d.Hours()/24)
}
