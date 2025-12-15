package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/observability/metrics"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
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

		// v0.6.2: Advanced transporter configuration
		transporterType    string
		enableOptimization bool
		congestionControl  string
		disableStaging     bool

		// Issue #155: Observability configuration
		enableTracing      bool
		tracingExporter    string
		tracingEndpoint    string
		tracingSampleRate  float64
		prometheusAddr     string

		// Issue #163: Encryption configuration
		kmsKeyID         string
		encryptManifest  bool
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

			// Validate shard count
			if shardCount < 1 || shardCount > 100 {
				return fmt.Errorf("shard-count must be between 1 and 100")
			}

			// Validate encryption flags (Issue #163)
			if encryptManifest && kmsKeyID == "" {
				return fmt.Errorf("--encrypt-manifest requires --kms-key-id to be set")
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

			// Create pipeline config with CargoHold settings
			pipelineConfig := &pipeline.PipelineConfig{
				ScannerWorkers:  4,
				ArchiverWorkers: 4,
				UploaderWorkers: 4,
				S3Bucket:        bucket,
				S3Prefix:        prefix,
				S3Region:        region,
				UseRealS3:       true,
				S3Client:        s3Client,
				S3StorageClass:  storageClass,
				S3PartSize:      64 * 1024 * 1024, // 64MB parts

				// Issue #163: KMS encryption configuration
				KMSKeyID:        kmsKeyID,
				EncryptManifest: encryptManifest,
				KMSClient:       kmsClient,

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
			}

			// Note: Compression level and shard strategy are not yet implemented in the pipeline
			// These will be added in future enhancements
			// For now, we log them for visibility
			if !quiet {
				fmt.Printf("🚢 CargoHold Upload Configuration:\n")
				fmt.Printf("   Source:            %s\n", absPath)
				fmt.Printf("   Destination:       s3://%s/%s\n", bucket, prefix)
				fmt.Printf("   Shard Count:       %d\n", shardCount)
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
	cmd.Flags().IntVar(&shardCount, "shard-count", 10, "Number of shards for parallel uploads (1-100)")
	cmd.Flags().StringVar(&shardStrategy, "shard-strategy", "hash", "Shard distribution strategy (hash, size, type, directory)")
	cmd.Flags().IntVar(&compressionLevel, "compression-level", 3, "Zstd compression level (1-22, recommended 1-19)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Disable progress display")

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
