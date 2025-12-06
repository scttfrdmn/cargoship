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

// NewInfoCmd creates the 'info' command for displaying upload metadata
func NewInfoCmd() *cobra.Command {
	var (
		bucket   string
		prefix   string
		uploadID string
		region   string
		verbose  bool
		json     bool
	)

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Display metadata and statistics for a CargoShip upload",
		Long: `Display comprehensive metadata for a CargoShip upload without downloading data.

The info command downloads only the lightweight manifest (~30KB) and displays:
- Upload identification (ID, timestamp, source)
- File statistics (count, size, compression ratio)
- Shard distribution (per-shard statistics)
- Compression settings (algorithm, level, ratio achieved)
- Storage location (bucket, prefix, region)

This is useful for:
- Inspecting upload metadata before downloading
- Verifying upload completed successfully
- Planning selective extractions
- Auditing archived datasets

Examples:
  # Display basic upload information
  cargoship info --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # Show detailed per-shard statistics
  cargoship info --bucket my-bucket --upload-id 20231208-123456-abcd1234 --verbose

  # Output as JSON for scripting
  cargoship info --bucket my-bucket --upload-id 20231208-123456-abcd1234 --json
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
			if !json {
				fmt.Printf("📥 Loading manifest: s3://%s/%s\n\n", bucket, manifestKey)
			}

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

			// Read manifest bytes
			manifestBytes, err := io.ReadAll(result.Body)
			if err != nil {
				return fmt.Errorf("failed to read manifest from S3: %w", err)
			}

			// Decompress and deserialize manifest
			m, err := manifest.FromJSONCompressed(manifestBytes)
			if err != nil {
				return fmt.Errorf("failed to deserialize manifest: %w", err)
			}

			// JSON output for scripting
			if json {
				jsonData, err := m.ToJSON()
				if err != nil {
					return fmt.Errorf("failed to serialize manifest as JSON: %w", err)
				}
				fmt.Println(string(jsonData))
				return nil
			}

			// Display upload metadata
			fmt.Printf("📦 %s\n", makeHeader("Upload Information"))
			fmt.Printf("   Upload ID:        %s\n", m.UploadID)
			fmt.Printf("   Created:          %s (%s ago)\n",
				m.CreatedAt.Format(time.RFC3339),
				humanize.Time(m.CreatedAt))

			if !m.CompletedAt.IsZero() {
				duration := m.CompletedAt.Sub(m.CreatedAt)
				fmt.Printf("   Completed:        %s (took %s)\n",
					m.CompletedAt.Format(time.RFC3339),
					duration.Round(time.Second))
			}

			fmt.Printf("   Source:           %s\n", m.SourcePath)
			fmt.Printf("   Hostname:         %s\n", m.Hostname)
			fmt.Printf("   Manifest Version: %s\n", m.Version)
			fmt.Println()

			// Storage location
			fmt.Printf("📍 %s\n", makeHeader("Storage Location"))
			fmt.Printf("   Bucket:           %s\n", m.Bucket)
			fmt.Printf("   Prefix:           %s\n", m.Prefix)
			fmt.Printf("   Region:           %s\n", m.Region)
			fmt.Printf("   S3 URL:           s3://%s/%s\n", m.Bucket, m.Prefix)
			fmt.Println()

			// File statistics
			fmt.Printf("📊 %s\n", makeHeader("Dataset Statistics"))
			fmt.Printf("   Total Files:      %s files\n", humanize.Comma(m.TotalFiles))
			fmt.Printf("   Total Size:       %s (uncompressed)\n", humanize.Bytes(uint64(m.TotalBytes)))

			// Calculate total compressed size
			var totalCompressedSize int64
			for _, shard := range m.Shards {
				totalCompressedSize += shard.CompressedSize
			}
			fmt.Printf("   Compressed Size:  %s (%.1f%% of original)\n",
				humanize.Bytes(uint64(totalCompressedSize)),
				m.CompressionRatio*100)

			// Space savings
			savedBytes := m.TotalBytes - totalCompressedSize
			if savedBytes > 0 {
				savedPercent := (float64(savedBytes) / float64(m.TotalBytes)) * 100
				fmt.Printf("   Space Saved:      %s (%.1f%% reduction)\n",
					humanize.Bytes(uint64(savedBytes)),
					savedPercent)
			}
			fmt.Println()

			// Compression settings
			fmt.Printf("🗜️  %s\n", makeHeader("Compression"))
			fmt.Printf("   Algorithm:        %s\n", m.CompressionType)
			fmt.Printf("   Level:            %d\n", m.CompressionLevel)
			fmt.Printf("   Ratio:            %.2f:1 (%.1f%% of original)\n",
				1.0/m.CompressionRatio,
				m.CompressionRatio*100)
			fmt.Println()

			// Shard distribution
			fmt.Printf("🎯 %s\n", makeHeader("Shard Distribution"))
			fmt.Printf("   Total Shards:     %d\n", m.ShardCount)
			fmt.Printf("   Total Chunks:     %d (%.1f chunks/shard avg)\n",
				m.TotalChunks,
				float64(m.TotalChunks)/float64(m.ShardCount))
			fmt.Println()

			// Verbose: Per-shard statistics
			if verbose {
				fmt.Printf("📈 %s\n", makeHeader("Per-Shard Statistics"))
				fmt.Println()

				for _, shard := range m.Shards {
					fmt.Printf("   Shard %d:\n", shard.ID)
					fmt.Printf("      Prefix:           %s\n", shard.Prefix)
					fmt.Printf("      Chunks:           %d chunks\n", shard.ChunkCount)
					fmt.Printf("      Files:            %s files\n", humanize.Comma(shard.FileCount))
					fmt.Printf("      Uncompressed:     %s\n", humanize.Bytes(uint64(shard.UncompressedSize)))
					fmt.Printf("      Compressed:       %s\n", humanize.Bytes(uint64(shard.CompressedSize)))

					if shard.UncompressedSize > 0 {
						ratio := float64(shard.CompressedSize) / float64(shard.UncompressedSize)
						fmt.Printf("      Compression:      %.1f%% of original\n", ratio*100)
					}
					fmt.Println()
				}
			} else {
				fmt.Printf("   💡 Use --verbose to see per-shard statistics\n\n")
			}

			// Storage efficiency metrics
			if m.TotalFiles > 0 {
				avgFileSize := m.TotalBytes / m.TotalFiles
				fmt.Printf("💾 %s\n", makeHeader("Storage Efficiency"))
				fmt.Printf("   Avg File Size:    %s\n", humanize.Bytes(uint64(avgFileSize)))

				if m.TotalChunks > 0 {
					avgChunkSize := totalCompressedSize / int64(m.TotalChunks)
					filesPerChunk := m.TotalFiles / int64(m.TotalChunks)
					fmt.Printf("   Avg Chunk Size:   %s (compressed)\n", humanize.Bytes(uint64(avgChunkSize)))
					fmt.Printf("   Files per Chunk:  %d files\n", filesPerChunk)
				}
				fmt.Println()
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (required)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix for upload (default: empty)")
	cmd.Flags().StringVarP(&uploadID, "upload-id", "u", "", "Upload ID to inspect (required)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed per-shard statistics")
	cmd.Flags().BoolVar(&json, "json", false, "Output manifest as JSON for scripting")

	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("upload-id"); err != nil {
		panic(fmt.Sprintf("failed to mark upload-id flag as required: %v", err))
	}

	return cmd
}

// makeHeader creates a formatted section header
func makeHeader(title string) string {
	return fmt.Sprintf("\033[1m%s\033[0m", title) // Bold text
}
