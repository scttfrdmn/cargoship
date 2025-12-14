package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/scttfrdmn/cargoship/pkg/migration"
)

// NewMigrateCmd creates the migrate command
func NewMigrateCmd() *cobra.Command {
	var (
		region           string
		tempDir          string
		keepTemp         bool
		deleteOriginal   bool
		shardCount       int
		compressionLevel int
		storageClass     string
		dryRun           bool
		quiet            bool
		skipValidation   bool
	)

	cmd := &cobra.Command{
		Use:   "migrate SOURCE_ARCHIVE DESTINATION",
		Short: "Convert traditional archives to CargoHold sharded format",
		Long: `Download a traditional tar.zst archive from S3 and re-upload
using CargoHold's intelligent sharding system.

The migrate command:
1. Downloads the traditional archive from S3
2. Extracts files to a temporary location
3. Re-uploads using CargoHold sharding with compression
4. Generates a manifest for selective extraction
5. Optionally deletes the original archive

Examples:
  # Migrate archive with default settings
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded

  # Migrate and delete original
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --delete-original

  # Dry run to estimate migration
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --dry-run

  # Custom temp directory and shard count
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded \
    --temp-dir /mnt/fast-ssd --shard-count 16

  # Keep temp files for debugging
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --keep-temp
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceArchive := args[0]
			destination := args[1]

			// Load AWS configuration
			ctx := context.Background()
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			// Create S3 client
			s3Client := s3.NewFromConfig(cfg)

			// Create migration request
			req := &migration.MigrateRequest{
				SourceArchive:    sourceArchive,
				Destination:      destination,
				Region:           region,
				TempDir:          tempDir,
				KeepTemp:         keepTemp,
				DeleteOriginal:   deleteOriginal,
				ShardCount:       shardCount,
				CompressionLevel: compressionLevel,
				StorageClass:     storageClass,
				DryRun:           dryRun,
				Quiet:            quiet,
				SkipValidation:   skipValidation,
			}

			// Handle dry run mode
			if dryRun {
				return handleDryRun(ctx, s3Client, req)
			}

			// Create migrator with progress callback
			migratorConfig := &migration.MigratorConfig{
				S3Client: s3Client,
			}

			if !quiet {
				migratorConfig.ProgressCallback = displayProgress
			}

			migrator := migration.NewMigrator(migratorConfig)

			// Display migration start
			if !quiet {
				fmt.Println("🚢 CargoShip Migration")
				fmt.Println("====================")
				fmt.Printf("Source:      %s\n", sourceArchive)
				fmt.Printf("Destination: %s\n", destination)
				fmt.Printf("Shards:      %d\n", shardCount)
				fmt.Printf("Compression: zstd level %d\n", compressionLevel)
				fmt.Println()
			}

			// Perform migration
			result, err := migrator.Migrate(ctx, req)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			// Display result
			if !quiet {
				displayResult(result, sourceArchive, destination)
			}

			// Display warnings/errors
			if len(result.Errors) > 0 {
				fmt.Println("\n⚠️  Warnings:")
				for _, err := range result.Errors {
					fmt.Printf("  • %v\n", err)
				}
			}

			if !result.Success {
				return fmt.Errorf("migration completed with errors")
			}

			return nil
		},
	}

	// Flags
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&tempDir, "temp-dir", "", "Temporary directory for extraction (default: OS temp)")
	cmd.Flags().BoolVar(&keepTemp, "keep-temp", false, "Keep temporary files after migration")
	cmd.Flags().BoolVar(&deleteOriginal, "delete-original", false, "Delete original archive after successful migration")
	cmd.Flags().IntVar(&shardCount, "shard-count", 8, "Number of shards for CargoHold (1-100)")
	cmd.Flags().IntVar(&compressionLevel, "compression-level", 3, "Zstd compression level (1-22)")
	cmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER_IR, DEEP_ARCHIVE)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Estimate migration without performing it")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Disable progress display")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip pre-flight validation checks")

	return cmd
}

// handleDryRun handles dry-run mode
func handleDryRun(ctx context.Context, s3Client *s3.Client, req *migration.MigrateRequest) error {
	fmt.Println("🔍 Dry Run Mode - Estimating Migration")
	fmt.Println("======================================")
	fmt.Println()

	// Parse source to get location
	source, err := migration.ParseS3URL(req.SourceArchive)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}

	// Create migrator for estimation
	migrator := migration.NewMigrator(&migration.MigratorConfig{
		S3Client: s3Client,
	})

	// Get estimate
	estimate, err := migrator.EstimateMigration(ctx, req, source)
	if err != nil {
		return fmt.Errorf("estimation failed: %w", err)
	}

	// Display estimate
	fmt.Printf("📦 Source Archive\n")
	fmt.Printf("  Size: %s (compressed)\n", humanize.Bytes(uint64(estimate.SourceArchiveSize)))
	fmt.Printf("  Estimated Uncompressed: %s\n", humanize.Bytes(uint64(estimate.EstimatedUncompressedSize)))
	fmt.Println()

	fmt.Printf("💾 Disk Space\n")
	fmt.Printf("  Required: %s\n", humanize.Bytes(uint64(estimate.TempDiskSpaceRequired)))
	fmt.Printf("  Available: %s\n", humanize.Bytes(uint64(estimate.AvailableDiskSpace)))
	if estimate.AvailableDiskSpace >= estimate.TempDiskSpaceRequired {
		fmt.Printf("  Status: ✅ Sufficient\n")
	} else {
		fmt.Printf("  Status: ❌ Insufficient\n")
	}
	fmt.Println()

	fmt.Printf("🔧 CargoHold Configuration\n")
	fmt.Printf("  Shards: %d\n", estimate.EstimatedShards)
	fmt.Printf("  Estimated Chunks: ~%d\n", estimate.EstimatedChunks)
	fmt.Println()

	fmt.Printf("💰 Estimated Costs\n")
	fmt.Printf("  Download: $%.4f\n", estimate.EstimatedDownloadCost)
	fmt.Printf("  Upload: $%.4f\n", estimate.EstimatedUploadCost)
	fmt.Printf("  Storage (monthly): $%.4f\n", estimate.EstimatedStorageCost)
	fmt.Printf("  Total (one-time): $%.4f\n", estimate.EstimatedDownloadCost+estimate.EstimatedUploadCost)
	fmt.Println()

	fmt.Printf("⏱️  Estimated Duration\n")
	fmt.Printf("  Time: %v\n", estimate.EstimatedDuration.Round(time.Second))
	fmt.Println()

	// Display warnings
	if len(estimate.Warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, warning := range estimate.Warnings {
			fmt.Printf("  • %s\n", warning)
		}
		fmt.Println()
	}

	// Display conclusion
	if estimate.CanProceed {
		fmt.Println("✅ Migration can proceed")
		fmt.Println("\nRun without --dry-run to perform the migration")
	} else {
		fmt.Println("❌ Migration cannot proceed")
		fmt.Println("\nResolve the issues above before attempting migration")
		return fmt.Errorf("migration cannot proceed")
	}

	return nil
}

// displayProgress displays migration progress
func displayProgress(update migration.ProgressUpdate) {
	switch update.Phase {
	case migration.PhaseValidation:
		fmt.Printf("\r🔍 %s...                           ", update.Message)

	case migration.PhaseExtraction:
		if update.BytesProcessed > 0 {
			throughput := update.ThroughputBytesPerSec / (1024 * 1024) // MB/s
			fmt.Printf("\r📥 Extracting: %s | %s | %.1f MB/s     ",
				humanize.Comma(update.FilesProcessed),
				humanize.Bytes(uint64(update.BytesProcessed)),
				throughput)
		} else {
			fmt.Printf("\r📥 %s...                           ", update.Message)
		}

	case migration.PhaseUpload:
		if update.TotalBytes > 0 {
			percent := float64(update.BytesProcessed) / float64(update.TotalBytes) * 100
			throughput := update.ThroughputBytesPerSec / (1024 * 1024) // MB/s
			fmt.Printf("\r📤 Uploading: %.1f%% | %s / %s | %.1f MB/s     ",
				percent,
				humanize.Bytes(uint64(update.BytesProcessed)),
				humanize.Bytes(uint64(update.TotalBytes)),
				throughput)
		} else {
			fmt.Printf("\r📤 %s...                           ", update.Message)
		}

	case migration.PhaseCleanup:
		fmt.Printf("\r🧹 %s...                           ", update.Message)

	case migration.PhaseComplete:
		fmt.Printf("\r✅ %s                              \n", update.Message)
	}
}

// displayResult displays the migration result
func displayResult(result *migration.MigrateResult, sourceArchive, destination string) {
	fmt.Println()
	fmt.Println("✅ Migration Complete!")
	fmt.Println("=====================")
	fmt.Println()

	fmt.Printf("📊 Summary\n")
	fmt.Printf("  Files Migrated:    %s\n", humanize.Comma(result.FilesUploaded))
	fmt.Printf("  Data Size:         %s (uncompressed)\n", humanize.Bytes(uint64(result.BytesUploaded)))
	fmt.Printf("  Shards Created:    %d\n", result.ShardsCreated)
	fmt.Printf("  Chunks Created:    %s\n", humanize.Comma(int64(result.ChunksCreated)))
	if result.CompressionRatio > 0 {
		fmt.Printf("  Compression Ratio: %.2fx\n", result.CompressionRatio)
	}
	fmt.Printf("  Duration:          %v\n", result.Duration.Round(time.Second))
	fmt.Printf("    • Extraction:    %v\n", result.ExtractionTime.Round(time.Second))
	fmt.Printf("    • Upload:        %v\n", result.UploadTime.Round(time.Second))
	fmt.Println()

	fmt.Printf("📍 Original Archive\n")
	fmt.Printf("  Location: %s\n", sourceArchive)
	if result.OriginalArchiveDeleted {
		fmt.Printf("  Status:   🗑️  Deleted\n")
	} else {
		fmt.Printf("  Status:   ⚠️  Preserved (use --delete-original to remove)\n")
	}
	fmt.Println()

	fmt.Printf("📍 New CargoHold Format\n")
	parsedDest, _ := migration.ParseS3URL(destination)
	fmt.Printf("  Location: s3://%s/%s/uploads/%s\n", parsedDest.Bucket, parsedDest.Key, result.UploadID)
	fmt.Printf("  Manifest: %s\n", result.ManifestLocation)
	fmt.Printf("  Shards:   shard-0 through shard-%d\n", result.ShardsCreated-1)
	fmt.Println()

	// Note: Temp directory cleanup is handled by the migrator based on --keep-temp flag

	fmt.Printf("💡 Next Steps\n")
	fmt.Printf("  • Verify migration: cargoship verify %s\n", result.ManifestLocation)
	fmt.Printf("  • List files:       cargoship list %s\n", result.ManifestLocation)
	fmt.Printf("  • Download files:   cargoship download %s ./restored\n", result.ManifestLocation)
	fmt.Println()
}
