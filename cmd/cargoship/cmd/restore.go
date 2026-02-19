package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewRestoreCmd creates the 'restore' command for hash-based and DVC-aware
// selective file restoration (Issue #189).
func NewRestoreCmd() *cobra.Command {
	var (
		hash       string
		filePaths  []string
		gitCommit  string
		dvcStage   string
		region     string
		cacheGB    int64
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "restore S3_URL OUTPUT_DIR",
		Short: "Restore specific files from a CargoShip archive using hash, path, commit, or DVC stage",
		Long: `Restore targeted files from a CargoShip archive without downloading the whole dataset.

Restoration modes (pick one or combine --file with others):
  --hash        : Restore a single file by its MD5 content hash
  --file        : Restore one or more exact file paths
  --git-commit  : Restore all files from a specific git commit
  --dvc-stage   : Restore all files produced by a DVC pipeline stage

The SelectiveExtractor groups files by their containing S3 chunk, downloading
each distinct chunk at most once. An LRU cache (default 10 GB) avoids redundant
downloads across repeated restore calls on the same session.

Examples:
  # Restore a file by its MD5 hash
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --hash d8e8fca2dc0f896fd7cb4cb0031ba249

  # Restore specific files by path
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --file data/train.csv --file models/model.pkl

  # Restore all files from a DVC pipeline stage
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage preprocess

  # Restore all files from a git commit
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --git-commit abc1234567890

  # Increase cache for large chunk sets
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage train --cache-gb 20
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s3URL := args[0]
			outputDir := args[1]

			bucket, prefix, err := parseS3URL(s3URL)
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}

			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)
			kmsClient := kms.NewFromConfig(cfg)

			// Parse upload ID from prefix.
			var actualPrefix, uploadID string
			if idx := strings.Index(prefix, "/uploads/"); idx != -1 {
				actualPrefix = prefix[:idx]
				uploadID = prefix[idx+9:]
			} else {
				uploadID = prefix
			}

			fmt.Printf("📥 Loading manifest: s3://%s/%s\n", bucket, prefix)
			m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}
			fmt.Printf("✅ Manifest loaded: %d files, %d chunks\n\n", m.TotalFiles, m.TotalChunks)

			maxCacheBytes := cacheGB * 1024 * 1024 * 1024
			se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes)

			var stats *manifest.RestoreStats

			switch {
			case hash != "":
				fmt.Printf("🔍 Restoring file by hash: %s\n", hash)
				stats, err = se.ExtractFileByHash(ctx, hash, outputDir)

			case dvcStage != "":
				fmt.Printf("🔍 Restoring DVC stage: %s\n", dvcStage)
				stats, err = se.BatchRestoreByDVCStage(ctx, dvcStage, outputDir)

			case gitCommit != "":
				fmt.Printf("🔍 Restoring git commit: %s\n", gitCommit)
				stats, err = se.BatchRestoreByCommit(ctx, gitCommit, outputDir)

			case len(filePaths) > 0:
				fmt.Printf("🔍 Restoring %d file(s) by path\n", len(filePaths))
				stats, err = se.BatchRestore(ctx, filePaths, outputDir)

			default:
				return fmt.Errorf("specify at least one of --hash, --file, --git-commit, or --dvc-stage")
			}

			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			if jsonOutput {
				fmt.Printf(`{"restored":%d,"failed":%d,"bytes":%d,"chunks_downloaded":%d}`+"\n",
					stats.Restored, stats.Failed, stats.Bytes, stats.ChunksDownloaded)
				return nil
			}

			fmt.Printf("✅ Restore complete!\n")
			fmt.Printf("   Files restored:    %d\n", stats.Restored)
			if stats.Failed > 0 {
				fmt.Printf("   Files failed:      %d\n", stats.Failed)
			}
			fmt.Printf("   Data written:      %s\n", humanize.Bytes(uint64(stats.Bytes)))
			fmt.Printf("   Chunks downloaded: %d\n", stats.ChunksDownloaded)
			fmt.Printf("   Output directory:  %s\n", outputDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&hash, "hash", "", "MD5 content hash of the file to restore")
	cmd.Flags().StringArrayVar(&filePaths, "file", nil, "Exact file path(s) to restore (repeatable)")
	cmd.Flags().StringVar(&gitCommit, "git-commit", "", "Restore all files from this git commit SHA")
	cmd.Flags().StringVar(&dvcStage, "dvc-stage", "", "Restore all files produced by this DVC pipeline stage")
	cmd.Flags().StringVarP(&region, "region", "r", "us-east-1", "AWS region")
	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB (0 = default 10 GB)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output restore statistics as JSON")

	return cmd
}
