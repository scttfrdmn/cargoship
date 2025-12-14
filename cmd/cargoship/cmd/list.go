package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewListCmd creates the 'list' command for querying uploaded files
func NewListCmd() *cobra.Command {
	var (
		bucket   string
		prefix   string
		uploadID string
		pattern  string
		verbose  bool
		region   string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List files from a CargoShip upload using the manifest",
		Long: `Query and display files from a CargoShip upload without downloading archives.

The list command downloads the lightweight manifest.json.gz file (~30KB) from S3
and displays all uploaded files with their locations and metadata.

Examples:
  # List all files from an upload
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # List files matching a pattern
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234 --pattern "*.log"

  # Verbose output with chunk and shard information
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234 --verbose
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

			// Download manifest from S3
			manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
			fmt.Printf("Downloading manifest: s3://%s/%s\n", bucket, manifestKey)

			getObjectInput := &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(manifestKey),
			}

			result, err := s3Client.GetObject(ctx, getObjectInput)
			if err != nil {
				return fmt.Errorf("failed to download manifest from S3: %w", err)
			}
			defer func() {
				if closeErr := result.Body.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close S3 response body: %v\n", closeErr)
				}
			}()

			// Read all bytes from S3 response
			manifestBytes, err := io.ReadAll(result.Body)
			if err != nil {
				return fmt.Errorf("failed to read manifest from S3: %w", err)
			}

			// Decompress and deserialize manifest
			m, err := manifest.FromJSONCompressed(manifestBytes)
			if err != nil {
				return fmt.Errorf("failed to deserialize manifest: %w", err)
			}

			// Create query interface
			query := manifest.NewManifestQuery(m)

			// Filter files by pattern using ManifestQuery API
			files := query.ListFiles(pattern)

			// Calculate total compressed size from shards
			var totalCompressedSize int64
			for _, shard := range m.Shards {
				totalCompressedSize += shard.CompressedSize
			}

			// Display results
			fmt.Printf("\n📦 Upload: %s\n", m.UploadID)
			fmt.Printf("📅 Created: %s\n", m.CreatedAt.Format(time.RFC3339))
			if !m.CompletedAt.IsZero() {
				fmt.Printf("✅ Completed: %s\n", m.CompletedAt.Format(time.RFC3339))
			}
			fmt.Printf("📊 Stats: %d files, %d chunks, %d shards\n",
				m.TotalFiles, m.TotalChunks, m.ShardCount)
			fmt.Printf("💾 Size: %s uncompressed, %s compressed (%.1f%% ratio)\n",
				humanize.Bytes(uint64(m.TotalBytes)),
				humanize.Bytes(uint64(totalCompressedSize)),
				m.CompressionRatio*100)

			if pattern != "" {
				fmt.Printf("\n🔍 Filtered to %d files matching '%s':\n\n", len(files), pattern)
			} else {
				fmt.Printf("\n📄 Files (%d total):\n\n", len(files))
			}

			// Display files
			for i, file := range files {
				if verbose {
					// Verbose output with full details
					fmt.Printf("[%d] %s\n", i+1, file.Path)
					fmt.Printf("    Size: %s\n", humanize.Bytes(uint64(file.Size)))
					fmt.Printf("    Modified: %s\n", file.ModTime.Format(time.RFC3339))
					fmt.Printf("    Chunk: %d, Shard: %d\n", file.ChunkID, file.ShardID)
					fmt.Printf("    S3 Key: %s\n", file.S3Key)
					fmt.Println()
				} else {
					// Compact output
					fmt.Printf("%-80s %10s  %s\n",
						truncate(file.Path, 80),
						humanize.Bytes(uint64(file.Size)),
						file.ModTime.Format("2006-01-02 15:04"),
					)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (required)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix for upload (default: empty)")
	cmd.Flags().StringVarP(&uploadID, "upload-id", "u", "", "Upload ID to query (required)")
	cmd.Flags().StringVar(&pattern, "pattern", "", "Filter files by glob pattern (e.g., '*.log')")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show verbose output with full file details")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")

	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("upload-id"); err != nil {
		panic(fmt.Sprintf("failed to mark upload-id flag as required: %v", err))
	}

	return cmd
}

// truncate truncates a string to maxLen characters, adding "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
