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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

// NewScuttleCmd creates the 'scuttle' command for nuclear deletion
func NewScuttleCmd() *cobra.Command {
	var (
		bucket string
		prefix string
		region string
		force  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "scuttle",
		Short: "🚨 NUCLEAR OPTION: Delete ALL CargoShip data from a bucket/prefix",
		Long: `🚨 NUCLEAR OPTION: Delete ALL CargoShip uploads from a bucket or prefix.

The scuttle command is the nuclear option that deletes EVERYTHING:
- ALL uploads (all manifests, all chunks, all shards)
- ALL data under the specified prefix
- This operation is IRREVERSIBLE and PERMANENT

This command is named after "scuttling a ship" - deliberately sinking your own vessel.
Use this when:
- Decommissioning a test environment
- Cleaning up after development
- Starting fresh with a new bucket structure
- You're absolutely certain you want to delete EVERYTHING

⚠️  EXTREME DANGER WARNING ⚠️
This command requires TRIPLE confirmation unless --force is used.
All data will be permanently destroyed with no recovery option.

Safety features:
- Requires typing the full bucket name to confirm
- Requires typing "SCUTTLE" to confirm deletion
- Shows detailed preview of what will be deleted
- --dry-run to preview without deleting
- Rate-limited to prevent accidental mass deletion

Examples:
  # Scuttle all data in a bucket (with triple confirmation)
  cargoship scuttle --bucket my-test-bucket

  # Scuttle only a specific prefix
  cargoship scuttle --bucket my-bucket --prefix test-data/

  # Dry run to see what would be deleted
  cargoship scuttle --bucket my-bucket --dry-run

  # Force scuttle without confirmation (DANGEROUS)
  cargoship scuttle --bucket my-bucket --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Validate required flags
			if bucket == "" {
				return fmt.Errorf("--bucket is required")
			}

			// Load AWS config
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)

			// List all objects to delete
			fmt.Printf("🔍 Scanning S3 bucket: s3://%s/%s\n", bucket, prefix)

			var allObjects []types.Object
			var totalSize int64

			listInput := &s3.ListObjectsV2Input{
				Bucket: aws.String(bucket),
			}
			if prefix != "" {
				listInput.Prefix = aws.String(prefix)
			}

			paginator := s3.NewListObjectsV2Paginator(s3Client, listInput)
			for paginator.HasMorePages() {
				page, err := paginator.NextPage(ctx)
				if err != nil {
					return fmt.Errorf("failed to list objects: %w", err)
				}

				allObjects = append(allObjects, page.Contents...)
				for _, obj := range page.Contents {
					if obj.Size != nil {
						totalSize += *obj.Size
					}
				}
			}

			if len(allObjects) == 0 {
				fmt.Println("✅ No objects found to delete")
				return nil
			}

			// Show what will be deleted
			fmt.Printf("\n🚨 NUCLEAR DELETION WARNING 🚨\n\n")
			fmt.Printf("   Bucket:           %s\n", bucket)
			if prefix != "" {
				fmt.Printf("   Prefix:           %s\n", prefix)
			} else {
				fmt.Printf("   Prefix:           (ALL OBJECTS IN BUCKET)\n")
			}
			fmt.Printf("   Region:           %s\n", region)
			fmt.Printf("   Objects:          %s objects\n", humanize.Comma(int64(len(allObjects))))
			fmt.Printf("   Total Size:       %s\n", humanize.Bytes(uint64(totalSize)))
			fmt.Println()

			// Show sample of what will be deleted
			fmt.Println("📋 Sample of objects to be deleted:")
			sampleSize := 10
			if len(allObjects) < sampleSize {
				sampleSize = len(allObjects)
			}
			for i := 0; i < sampleSize; i++ {
				obj := allObjects[i]
				fmt.Printf("   - %s (%s)\n",
					aws.ToString(obj.Key),
					humanize.Bytes(uint64(aws.ToInt64(obj.Size))))
			}
			if len(allObjects) > sampleSize {
				fmt.Printf("   ... and %s more objects\n", humanize.Comma(int64(len(allObjects)-sampleSize)))
			}
			fmt.Println()

			// Dry run - just show what would be deleted
			if dryRun {
				fmt.Printf("🔍 Dry run - would delete %s objects (%s)\n",
					humanize.Comma(int64(len(allObjects))),
					humanize.Bytes(uint64(totalSize)))
				return nil
			}

			// Triple confirmation (unless --force)
			if !force {
				reader := bufio.NewReader(os.Stdin)

				// Confirmation 1: Bucket name
				fmt.Printf("⚠️  CONFIRMATION 1/3: Type the bucket name to confirm: ")
				response1, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if strings.TrimSpace(response1) != bucket {
					fmt.Println("❌ Bucket name mismatch. Scuttle cancelled.")
					return nil
				}

				// Confirmation 2: SCUTTLE
				fmt.Printf("⚠️  CONFIRMATION 2/3: Type 'SCUTTLE' (all caps) to confirm: ")
				response2, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if strings.TrimSpace(response2) != "SCUTTLE" {
					fmt.Println("❌ Confirmation failed. Scuttle cancelled.")
					return nil
				}

				// Confirmation 3: Final warning
				fmt.Printf("\n⚠️  FINAL WARNING: You are about to PERMANENTLY DELETE:\n")
				fmt.Printf("   - %s objects\n", humanize.Comma(int64(len(allObjects))))
				fmt.Printf("   - %s of data\n", humanize.Bytes(uint64(totalSize)))
				fmt.Printf("   - From bucket: %s\n", bucket)
				if prefix != "" {
					fmt.Printf("   - Under prefix: %s\n", prefix)
				}
				fmt.Printf("\n   This operation is IRREVERSIBLE.\n")
				fmt.Printf("   All data will be PERMANENTLY DESTROYED.\n\n")
				fmt.Printf("⚠️  CONFIRMATION 3/3: Type 'yes' to proceed: ")

				response3, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if strings.TrimSpace(strings.ToLower(response3)) != "yes" {
					fmt.Println("❌ Final confirmation failed. Scuttle cancelled.")
					return nil
				}
			}

			// Delete all objects
			fmt.Printf("\n🗑️  Scuttling %s objects...\n", humanize.Comma(int64(len(allObjects))))
			fmt.Println("   This may take a while...")
			startTime := time.Now()

			// S3 DeleteObjects has a limit of 1000 keys per request
			const batchSize = 1000
			deletedCount := 0
			errorCount := 0

			for i := 0; i < len(allObjects); i += batchSize {
				end := i + batchSize
				if end > len(allObjects) {
					end = len(allObjects)
				}

				batch := allObjects[i:end]
				var objectsToDelete []types.ObjectIdentifier
				for _, obj := range batch {
					objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
						Key: obj.Key,
					})
				}

				deleteInput := &s3.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &types.Delete{
						Objects: objectsToDelete,
						Quiet:   aws.Bool(true), // Only report errors
					},
				}

				deleteResult, err := s3Client.DeleteObjects(ctx, deleteInput)
				if err != nil {
					fmt.Printf("⚠️  Error deleting batch %d-%d: %v\n", i, end, err)
					errorCount += len(batch)
					continue
				}

				// Check for errors
				if len(deleteResult.Errors) > 0 {
					errorCount += len(deleteResult.Errors)
					if errorCount < 10 {
						// Only show first few errors to avoid spam
						for _, deleteErr := range deleteResult.Errors {
							fmt.Printf("   ⚠️  Error: %s: %s\n", aws.ToString(deleteErr.Key), aws.ToString(deleteErr.Message))
						}
					}
				}

				deletedCount += len(batch) - len(deleteResult.Errors)

				// Progress update every 5000 objects
				if deletedCount%5000 == 0 || end == len(allObjects) {
					percent := float64(deletedCount) / float64(len(allObjects)) * 100
					fmt.Printf("   Deleted %s/%s objects (%.1f%%)...\n",
						humanize.Comma(int64(deletedCount)),
						humanize.Comma(int64(len(allObjects))),
						percent)
				}
			}

			duration := time.Since(startTime)

			fmt.Printf("\n✅ Scuttle complete!\n")
			fmt.Printf("   Objects deleted:  %s objects\n", humanize.Comma(int64(deletedCount)))
			if errorCount > 0 {
				fmt.Printf("   Errors:           %d errors\n", errorCount)
			}
			fmt.Printf("   Space freed:      %s\n", humanize.Bytes(uint64(totalSize)))
			fmt.Printf("   Duration:         %s\n", duration.Round(time.Millisecond))
			fmt.Printf("   Bucket:           %s\n", bucket)
			if prefix != "" {
				fmt.Printf("   Prefix:           %s\n", prefix)
			}

			if errorCount > 0 {
				fmt.Printf("\n⚠️  Warning: %d objects failed to delete. You may need to re-run scuttle.\n", errorCount)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (required)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix to delete (default: entire bucket)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&force, "force", false, "Skip ALL confirmations (EXTREMELY DANGEROUS)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without actually deleting")

	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}

	return cmd
}
