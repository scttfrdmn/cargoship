package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// NewSyncCmd creates the 'sync' command for incremental uploads (Issue #148)
func NewSyncCmd() *cobra.Command {
	var (
		storageClass     string
		shardCount       int
		shardStrategy    string
		compressionLevel int
		region           string
		useChecksum      bool
		trackDeletes     bool
		dryRun           bool
		force            bool
		quiet            bool
	)

	cmd := &cobra.Command{
		Use:   "sync SOURCE_DIR S3_URL",
		Short: "Incrementally sync directory to S3 (only upload new/changed files)",
		Long: `Incrementally sync a local directory to S3 by uploading only new or modified files.

The sync command provides efficient incremental backups by:
1. Downloading the latest manifest for the source path (if exists)
2. Comparing local filesystem state against the manifest
3. Uploading only files that are new or have changed
4. Creating a new manifest that references the previous one

First sync uploads everything (like 'upload' command).
Subsequent syncs only upload changed files, saving time and bandwidth.

Change detection (default: fast mode):
  - Size change: File size differs from manifest
  - Time change: Modification time is newer than manifest

Use --checksum for guaranteed accuracy (slower, computes SHA256).

Examples:
  # First sync: uploads all files
  cargoship sync /home/photos s3://my-bucket/backups

  # Second sync: only uploads new/changed photos
  cargoship sync /home/photos s3://my-bucket/backups

  # Dry run to see what would be synced
  cargoship sync /home/photos s3://my-bucket/backups --dry-run

  # Use checksum comparison (slower but accurate)
  cargoship sync /data s3://my-bucket/backups --checksum

  # Force full sync (ignore previous manifest)
  cargoship sync /data s3://my-bucket/backups --force
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Parse arguments
			sourcePath := args[0]
			s3URL := args[1]

			// Validate source directory exists
			absPath, err := filepath.Abs(sourcePath)
			if err != nil {
				return fmt.Errorf("invalid source path: %w", err)
			}

			stat, err := os.Stat(absPath)
			if err != nil {
				return fmt.Errorf("source path error: %w", err)
			}
			if !stat.IsDir() {
				return fmt.Errorf("source path must be a directory: %s", absPath)
			}

			// Parse S3 URL
			bucket, prefix, err := parseS3URL(s3URL)
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}

			// #316: now that these flags do something, validate them the way
			// upload does — an unknown strategy should be a usage error, not a
			// silent fall-through to the default.
			if err := pipeline.ValidateShardStrategy(shardStrategy); err != nil {
				return err
			}
			if compressionLevel < 1 || compressionLevel > 22 {
				return fmt.Errorf("compression-level must be between 1 and 22 (zstd range)")
			}

			if !quiet {
				fmt.Printf("🔄 CargoShip Sync: %s → s3://%s/%s\n\n", absPath, bucket, prefix)
			}

			// Load AWS config
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)

			// Step 1: Try to download previous manifest (if exists and not forced)
			var previousManifest *manifest.Manifest
			var syncType string

			if !force {
				previousManifest, err = downloadLatestManifest(ctx, s3Client, bucket, prefix, absPath)
				if err != nil {
					// No previous manifest found - this is the first sync
					if !quiet {
						fmt.Println("📝 No previous manifest found - performing full sync")
					}
					syncType = manifest.SyncTypeFull
				} else {
					if !quiet {
						fmt.Printf("✅ Found previous manifest: %s (%d files, %s)\n",
							previousManifest.UploadID,
							previousManifest.TotalFiles,
							humanize.Bytes(uint64(previousManifest.TotalBytes)))
					}
					syncType = manifest.SyncTypeIncremental
				}
			} else {
				if !quiet {
					fmt.Println("⚠️  Force mode: ignoring previous manifest")
				}
				syncType = manifest.SyncTypeFull
			}

			// Step 2: Scan local filesystem
			if !quiet {
				fmt.Println("\n🔍 Scanning local filesystem...")
			}

			localFiles, err := manifest.ScanLocalFiles(absPath)
			if err != nil {
				return fmt.Errorf("failed to scan local files: %w", err)
			}

			if !quiet {
				fmt.Printf("✅ Scanned %d files\n", len(localFiles))
			}

			// Step 3: Compute delta
			if !quiet {
				fmt.Println("\n📊 Computing delta...")
			}

			opts := &manifest.SyncOptions{
				UseChecksum:  useChecksum,
				TrackDeletes: trackDeletes,
			}

			delta, err := manifest.ComputeDelta(localFiles, previousManifest, opts)
			if err != nil {
				return fmt.Errorf("failed to compute delta: %w", err)
			}

			if !quiet {
				fmt.Printf("✅ Delta: %s\n", delta.SummaryString())
			}

			// Check if there are any changes
			if !delta.HasChanges() {
				if !quiet {
					fmt.Println("\n✨ No changes detected - everything is up to date!")
				}
				return nil
			}

			// Calculate total size to upload
			var totalSize int64
			changedFiles := delta.GetChangedFiles()
			for _, file := range changedFiles {
				totalSize += file.Size
			}

			if !quiet {
				fmt.Printf("\n📦 Ready to sync %d files (%s)\n",
					len(changedFiles), humanize.Bytes(uint64(totalSize)))

				if len(delta.New) > 0 {
					fmt.Printf("   ├─ New: %d files\n", len(delta.New))
				}
				if len(delta.Modified) > 0 {
					fmt.Printf("   ├─ Modified: %d files\n", len(delta.Modified))
				}
				if len(delta.Deleted) > 0 {
					fmt.Printf("   └─ Deleted: %d files (tracked only)\n", len(delta.Deleted))
				}
				fmt.Println()
			}

			// Dry run - stop here
			if dryRun {
				fmt.Println("🔍 Dry run - no files uploaded")
				return nil
			}

			// Step 4: Upload changed files using existing pipeline with file filtering
			// Convert FileInfo to string paths for pipeline
			var includeFiles []string
			for _, file := range changedFiles {
				includeFiles = append(includeFiles, file.Path)
			}

			pipelineConfig := &pipeline.PipelineConfig{
				S3Bucket:          bucket,
				S3Prefix:          prefix,
				S3Region:          region,
				S3StorageClass:    storageClass,
				EnableMultiPrefix: true,
				ShardCount:        shardCount,
				WorkersPerPrefix:  2,
				EnableManifest:    true,
				SourcePath:        absPath,

				// #316: sync advertised --shard-strategy and --compression-level
				// and dropped both, without even upload's printout to hint that
				// they were inert. --compression-level is an override, so only
				// an explicitly passed value is forwarded; 0 keeps content-aware
				// per-chunk selection.
				ShardStrategy: shardStrategy,
				CompressionLevel: func() int {
					if cmd.Flags().Changed("compression-level") {
						return compressionLevel
					}
					return 0
				}(),

				// Issue #148: Incremental sync configuration
				IncludeOnlyFiles: includeFiles,
				SyncType:         syncType,
				PreviousUploadID: func() string {
					if previousManifest != nil {
						return previousManifest.UploadID
					}
					return ""
				}(),
			}

			pipe, err := pipeline.NewPipeline(pipelineConfig)
			if err != nil {
				return fmt.Errorf("failed to create pipeline: %w", err)
			}

			// Inform user about filtered upload
			if !quiet {
				if len(includeFiles) > 0 {
					fmt.Printf("📤 Uploading %d changed files (skipping %d unchanged)\n\n",
						len(includeFiles), len(delta.Same))
				} else {
					fmt.Println("📤 Performing full upload (first sync)")
				}
			}

			result, err := pipe.Run(ctx, absPath)
			if err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}

			// Success summary
			if !quiet {
				fmt.Printf("\n✅ Sync complete!\n")
				fmt.Printf("   Upload ID: %s\n", result.UploadID)
				fmt.Printf("   Sync type: %s (detected)\n", syncType)
				if previousManifest != nil {
					fmt.Printf("   Previous: %s (detected)\n", previousManifest.UploadID)
				}
				fmt.Printf("   Delta: %s\n", delta.SummaryString())
				fmt.Printf("   Location: s3://%s/%s\n", bucket, prefix)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "S3 storage class (STANDARD, GLACIER_IR, DEEP_ARCHIVE)")
	cmd.Flags().IntVar(&shardCount, "shard-count", 10, "Number of shards for parallel uploads (1-100)")
	// #316: same wording as upload's flags, which these now share behavior with.
	cmd.Flags().StringVar(&shardStrategy, "shard-strategy", pipeline.ShardStrategyRoundRobin,
		"Shard distribution strategy (round-robin, hash, size, type, directory)")
	cmd.Flags().IntVar(&compressionLevel, "compression-level", 3,
		"Fixed zstd compression level (1-22), overriding per-chunk content-aware selection. Unset = content-aware")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&useChecksum, "checksum", false, "Use SHA256 checksum comparison (slower but accurate)")
	cmd.Flags().BoolVar(&trackDeletes, "track-deletes", false, "Track deleted files in manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be synced without uploading")
	cmd.Flags().BoolVar(&force, "force", false, "Force full sync (ignore previous manifest)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet mode (minimal output)")

	return cmd
}

// downloadLatestManifest attempts to download the latest manifest for a source path (Issue #148)
// Uses the new FindLatestManifestForSource function to search all manifests
func downloadLatestManifest(ctx context.Context, s3Client *s3.Client, bucket, prefix, sourcePath string) (*manifest.Manifest, error) {
	// Use FindLatestManifestForSource to search through all manifests
	m, err := manifest.FindLatestManifestForSource(ctx, s3Client, bucket, prefix, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest manifest: %w", err)
	}

	if m == nil {
		return nil, fmt.Errorf("no manifest found for source path: %s", sourcePath)
	}

	return m, nil
}
