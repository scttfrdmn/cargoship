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

			// Build list of all S3 keys to delete: the upload's data objects
			// (chunk objects, or one object per file for a direct upload — #487),
			// then the manifest object(s).
			keysToDelete := uploadObjectKeys(m)

			// Add the manifest object(s): both the plaintext and encrypted names,
			// since the upload may have used --encrypt-manifest (#479). Deleting a
			// key that doesn't exist is a harmless no-op.
			keysToDelete = append(keysToDelete,
				fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID),
				fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json.gz", prefix, uploadID),
			)

			// #592: chain-aware guard. Incremental sync uploads only changed files
			// into a new uploads/<id>/ prefix; a later version's effective view still
			// references this upload's chunk objects by S3Key, and its chain
			// resolution walks this upload's manifest. Deleting an upload that a newer
			// version chains through would make that version unrestorable — so refuse
			// unless --force. Enumerate decryption-aware so encrypted-manifest chains
			// are covered too.
			allManifests, listErr := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if listErr != nil && !force {
				return fmt.Errorf("unable to verify no other upload depends on %s (re-run with --force to skip this check): %w", uploadID, listErr)
			}
			deps := manifest.DependentUploads(m.UploadID, allManifests)
			blocked := len(deps) > 0 && !force
			if len(deps) > 0 {
				fmt.Printf("\n⛔ %d newer upload(s) chain through %s and reference its objects:\n   %s\n",
					len(deps), m.UploadID, strings.Join(deps, ", "))
				fmt.Println("   Deleting it would make those uploads unrestorable.")
				if force {
					fmt.Println("   --force set: proceeding anyway — the dependent uploads above WILL be corrupted.")
				}
			}

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
				if blocked {
					fmt.Println("\n⛔ This delete would be REFUSED without --force (dependent uploads exist).")
				}
				return nil
			}

			// #592: refuse a chain-stranding delete before touching S3.
			if blocked {
				return fmt.Errorf("refusing to delete %s: %d dependent upload(s) (%s) would be stranded; re-run with --force to override",
					m.UploadID, len(deps), strings.Join(deps, ", "))
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

// uploadObjectKeys returns the S3 keys of the data objects an upload stored: the
// chunk objects for a chunked upload, or one object per file for a DIRECT upload
// (len(Chunks)==0), where there are no shards/chunks. It de-duplicates and drops
// empty keys (dedup duplicates reference the original's key). It does NOT include
// the manifest object(s). Enumerating direct-upload file keys is what keeps
// `delete` from orphaning a direct upload's objects (#487).
func uploadObjectKeys(m *manifest.Manifest) []string {
	seen := make(map[string]bool)
	var keys []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if len(m.Chunks) == 0 {
		for i := range m.Files {
			add(m.Files[i].S3Key)
		}
		return keys
	}
	for i := range m.Chunks {
		add(m.Chunks[i].S3Key)
	}
	return keys
}

// deleteS3Objects removes keys from bucket in batches of 1000 (the S3
// DeleteObjects limit), returning the count successfully deleted. Per-key S3
// errors are collected into the returned error but do not stop later batches.
func deleteS3Objects(ctx context.Context, s3Client *s3.Client, bucket string, keys []string) (int, error) {
	const batchSize = 1000
	deleted := 0
	var errs []string
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]
		objects := make([]types.ObjectIdentifier, 0, len(batch))
		for _, k := range batch {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(k)})
		}
		out, err := s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return deleted, fmt.Errorf("delete batch %d-%d: %w", i, end, err)
		}
		deleted += len(batch) - len(out.Errors)
		for _, e := range out.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
		}
	}
	if len(errs) > 0 {
		return deleted, fmt.Errorf("%d object(s) failed to delete: %s", len(errs), strings.Join(errs, "; "))
	}
	return deleted, nil
}

// listUploadManifests enumerates every upload under <prefix>/uploads/ and returns
// their parsed manifests, decryption-aware so encrypted-manifest uploads are
// included (ListAllManifests matches only the plaintext names). Uploads whose
// manifest can't be read are skipped. Used by the #592 chain-aware delete guard
// to walk the PreviousManifestID graph in memory.
func listUploadManifests(ctx context.Context, s3Client *s3.Client, kmsClient *kms.Client, bucket, prefix string) ([]*manifest.Manifest, error) {
	listPrefix := prefix
	if listPrefix != "" && !strings.HasSuffix(listPrefix, "/") {
		listPrefix += "/"
	}
	listPrefix += "uploads/"

	seen := make(map[string]bool)
	var ids []string
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(listPrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list uploads: %w", err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			// Only manifest objects tell us an upload's identity + chain. Match both
			// plaintext and encrypted names.
			base := key[strings.LastIndex(key, "/")+1:]
			if !strings.HasPrefix(base, "manifest.") ||
				(!strings.HasSuffix(base, ".json") && !strings.HasSuffix(base, ".json.gz")) {
				continue
			}
			// Extract the <id> segment after "uploads/".
			parts := strings.Split(key, "/")
			for i, p := range parts {
				if p == "uploads" && i+1 < len(parts) {
					if id := parts[i+1]; id != "" && !seen[id] {
						seen[id] = true
						ids = append(ids, id)
					}
					break
				}
			}
		}
	}

	var manifests []*manifest.Manifest
	for _, id := range ids {
		m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, prefix, id)
		if err != nil {
			continue // unreadable upload — skip; the guard errs toward the readable set
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
