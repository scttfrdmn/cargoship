package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewDeleteCmd creates the 'delete' command for removing uploads
func NewDeleteCmd() *cobra.Command {
	var (
		bucket   string
		prefix   string
		uploadID string
		region   string
		force    bool
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a CargoShip upload from S3",
		Long: `Delete a complete CargoShip upload including all chunks, shards, and manifest.

The delete command removes an entire upload from S3:
1. Downloads manifest to identify all S3 objects
2. Deletes all chunk archives across all shards
3. Deletes the manifest file
4. Shows summary of deleted objects and saved costs

WARNING: This operation is IRREVERSIBLE. All data will be permanently deleted.

Use --dry-run to preview what would be deleted without actually deleting.
Use --force to skip confirmation prompt (dangerous for automation).

Examples:
  # Delete an upload (with confirmation)
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # Dry run to see what would be deleted
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234 --dry-run

  # Force delete without confirmation (automation)
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234 --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Validate required flags
			if bucket == "" {
				return fmt.Errorf("--bucket is required")
			}
			if uploadID == "" {
				return fmt.Errorf("--upload-id is required")
			}

			// Load AWS config
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)
			kmsClient := kms.NewFromConfig(cfg)

			// Download manifest (decryption-aware: handles --encrypt-manifest uploads,
			// #479 — a delete that can't read the manifest would orphan objects).
			fmt.Printf("📥 Loading manifest: s3://%s/%s/uploads/%s/\n", bucket, prefix, uploadID)
			m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, prefix, uploadID)
			if err != nil {
				return fmt.Errorf("failed to download manifest from S3: %w", err)
			}

			// Build list of all S3 keys to delete
			var keysToDelete []string

			// Add all chunk keys from all shards
			for _, shard := range m.Shards {
				keysToDelete = append(keysToDelete, shard.ChunkKeys...)
			}

			// Add the manifest object(s): both the plaintext and encrypted names,
			// since the upload may have used --encrypt-manifest (#479). Deleting a
			// key that doesn't exist is a harmless no-op.
			keysToDelete = append(keysToDelete,
				fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID),
				fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json.gz", prefix, uploadID),
			)

			// Calculate total size
			var totalCompressedSize int64
			for _, shard := range m.Shards {
				totalCompressedSize += shard.CompressedSize
			}

			// Show what will be deleted
			fmt.Printf("\n⚠️  Will delete upload: %s\n", m.UploadID)
			fmt.Printf("   Files:            %s files\n", humanize.Comma(m.TotalFiles))
			fmt.Printf("   Size:             %s (uncompressed)\n", humanize.Bytes(uint64(m.TotalBytes)))
			fmt.Printf("   Compressed:       %s\n", humanize.Bytes(uint64(totalCompressedSize)))
			fmt.Printf("   Shards:           %d shards\n", m.ShardCount)
			fmt.Printf("   Chunks:           %d chunks\n", m.TotalChunks)
			fmt.Printf("   S3 Objects:       %d objects\n", len(keysToDelete))
			fmt.Printf("   Created:          %s (%s ago)\n",
				m.CreatedAt.Format(time.RFC3339),
				humanize.Time(m.CreatedAt))
			fmt.Println()

			// Dry run - just show what would be deleted
			if dryRun {
				fmt.Println("🔍 Dry run - would delete:")
				for i, key := range keysToDelete {
					if i < 10 {
						fmt.Printf("   - s3://%s/%s\n", bucket, key)
					} else if i == 10 {
						fmt.Printf("   ... and %d more objects\n", len(keysToDelete)-10)
						break
					}
				}
				fmt.Printf("\nTotal: %d S3 objects\n", len(keysToDelete))
				return nil
			}

			// Confirmation prompt (unless --force)
			if !force {
				fmt.Printf("⚠️  WARNING: This will permanently delete %d S3 objects (%s)\n",
					len(keysToDelete),
					humanize.Bytes(uint64(totalCompressedSize)))
				fmt.Printf("   Type 'yes' to confirm deletion: ")

				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}

				response = strings.TrimSpace(strings.ToLower(response))
				if response != "yes" {
					fmt.Println("❌ Deletion cancelled")
					return nil
				}
			}

			// Delete all objects
			fmt.Printf("\n🗑️  Deleting %d objects...\n", len(keysToDelete))
			startTime := time.Now()

			// S3 DeleteObjects has a limit of 1000 keys per request
			const batchSize = 1000
			deletedCount := 0

			for i := 0; i < len(keysToDelete); i += batchSize {
				end := i + batchSize
				if end > len(keysToDelete) {
					end = len(keysToDelete)
				}

				batch := keysToDelete[i:end]
				var objects []types.ObjectIdentifier
				for _, key := range batch {
					objects = append(objects, types.ObjectIdentifier{
						Key: aws.String(key),
					})
				}

				deleteInput := &s3.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &types.Delete{
						Objects: objects,
						Quiet:   aws.Bool(true), // Only report errors
					},
				}

				deleteResult, err := s3Client.DeleteObjects(ctx, deleteInput)
				if err != nil {
					return fmt.Errorf("failed to delete batch %d-%d: %w", i, end, err)
				}

				// Check for errors
				if len(deleteResult.Errors) > 0 {
					fmt.Printf("⚠️  Encountered %d errors:\n", len(deleteResult.Errors))
					for _, deleteErr := range deleteResult.Errors {
						fmt.Printf("   - %s: %s\n", aws.ToString(deleteErr.Key), aws.ToString(deleteErr.Message))
					}
				}

				deletedCount += len(batch) - len(deleteResult.Errors)
				fmt.Printf("   Deleted %d/%d objects...\n", deletedCount, len(keysToDelete))
			}

			duration := time.Since(startTime)

			fmt.Printf("\n✅ Deletion complete!\n")
			fmt.Printf("   Objects deleted:  %d objects\n", deletedCount)
			fmt.Printf("   Space freed:      %s\n", humanize.Bytes(uint64(totalCompressedSize)))
			fmt.Printf("   Duration:         %s\n", duration.Round(time.Millisecond))
			fmt.Printf("   Upload ID:        %s\n", uploadID)

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (required)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix for upload (default: empty)")
	cmd.Flags().StringVarP(&uploadID, "upload-id", "u", "", "Upload ID to delete (required)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt (dangerous)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without actually deleting")

	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("upload-id"); err != nil {
		panic(fmt.Sprintf("failed to mark upload-id flag as required: %v", err))
	}

	return cmd
}
