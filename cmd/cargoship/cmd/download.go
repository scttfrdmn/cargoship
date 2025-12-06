package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewDownloadCmd creates the 'download' command for selective extraction
func NewDownloadCmd() *cobra.Command {
	var (
		bucket     string
		prefix     string
		uploadID   string
		pattern    string
		files      []string
		shardIDs   []int
		outputDir  string
		region     string
		verbose    bool
		dryRun     bool
		workers    int
	)

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download and extract files from a CargoShip upload",
		Long: `Download and selectively extract files from a CargoShip upload using the manifest.

The download command provides efficient selective extraction by:
1. Downloading the lightweight manifest first (~30KB)
2. Identifying which chunks contain the requested files
3. Only downloading and extracting necessary chunks (10x faster than full download)

Selective extraction options:
  --pattern    : Glob pattern matching (e.g., "*.log", "data/*.csv")
  --files      : Comma-separated list of exact file paths
  --shard-ids  : Only download specific shard IDs (0-7 by default)

Examples:
  # Download all files from an upload
  cargoship download --bucket my-bucket --upload-id 20231208-123456-abcd1234 --output-dir ./restored

  # Download files matching a pattern
  cargoship download --bucket my-bucket --upload-id 20231208-123456-abcd1234 \
    --pattern "*.log" --output-dir ./logs

  # Download specific files
  cargoship download --bucket my-bucket --upload-id 20231208-123456-abcd1234 \
    --files "data/report.csv,data/summary.csv" --output-dir ./reports

  # Download specific shards only
  cargoship download --bucket my-bucket --upload-id 20231208-123456-abcd1234 \
    --shard-ids 0,2,4 --output-dir ./restored

  # Dry run to see what would be downloaded
  cargoship download --bucket my-bucket --upload-id 20231208-123456-abcd1234 \
    --pattern "*.csv" --dry-run
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
			if outputDir == "" && !dryRun {
				return fmt.Errorf("--output-dir is required (unless --dry-run is specified)")
			}

			// Load AWS config
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)

			// Step 1: Download manifest from S3
			manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
			fmt.Printf("📥 Downloading manifest: s3://%s/%s\n", bucket, manifestKey)

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

			fmt.Printf("✅ Manifest loaded: %d files, %d chunks, %d shards\n\n",
				m.TotalFiles, m.TotalChunks, m.ShardCount)

			// Step 2: Determine which files to download based on filters
			query := manifest.NewManifestQuery(m)
			var filesToDownload []manifest.FileEntry

			if len(files) > 0 {
				// Download specific files by exact path
				for _, filePath := range files {
					file := query.FindFile(filePath)
					if file != nil {
						filesToDownload = append(filesToDownload, *file)
					} else {
						fmt.Printf("⚠️  Warning: file not found in manifest: %s\n", filePath)
					}
				}
			} else if pattern != "" {
				// Download files matching pattern
				filesToDownload = query.ListFiles(pattern)
			} else if len(shardIDs) > 0 {
				// Download all files from specific shards
				for _, shardID := range shardIDs {
					shardFiles := query.FilesInShard(shardID)
					filesToDownload = append(filesToDownload, shardFiles...)
				}
			} else {
				// Download all files
				filesToDownload = query.ListFiles("")
			}

			if len(filesToDownload) == 0 {
				fmt.Println("⚠️  No files matched the selection criteria")
				return nil
			}

			// Calculate total size
			var totalSize int64
			for _, file := range filesToDownload {
				totalSize += file.Size
			}

			fmt.Printf("📦 Selected %d files (%s uncompressed)\n\n",
				len(filesToDownload), humanize.Bytes(uint64(totalSize)))

			// Step 3: Group files by chunk to minimize chunk downloads
			chunkFiles := make(map[int][]manifest.FileEntry)
			for _, file := range filesToDownload {
				chunkFiles[file.ChunkID] = append(chunkFiles[file.ChunkID], file)
			}

			fmt.Printf("🎯 Need to download %d chunks (out of %d total)\n\n", len(chunkFiles), m.TotalChunks)

			// Dry run - just show what would be downloaded
			if dryRun {
				fmt.Println("🔍 Dry run - would download:")
				for chunkID, files := range chunkFiles {
					chunk := findChunk(m, chunkID)
					if chunk != nil {
						fmt.Printf("\n  Chunk %d (s3://%s/%s, %s compressed):\n",
							chunkID, bucket, chunk.S3Key, humanize.Bytes(uint64(chunk.CompressedSize)))
						for _, file := range files {
							fmt.Printf("    - %s (%s)\n", file.Path, humanize.Bytes(uint64(file.Size)))
						}
					}
				}
				fmt.Printf("\nTotal: %d files, %s\n", len(filesToDownload), humanize.Bytes(uint64(totalSize)))
				return nil
			}

			// Step 4: Create output directory
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Step 5: Download and extract chunks
			startTime := time.Now()
			extractedCount := 0
			var extractedSize int64

			for chunkID, chunkFileList := range chunkFiles {
				chunk := findChunk(m, chunkID)
				if chunk == nil {
					fmt.Printf("⚠️  Warning: chunk %d not found in manifest\n", chunkID)
					continue
				}

				fmt.Printf("📥 Downloading chunk %d/%d: %s (%s)\n",
					chunkID+1, len(chunkFiles), chunk.S3Key, humanize.Bytes(uint64(chunk.CompressedSize)))

				// Download chunk from S3
				chunkResult, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(chunk.S3Key),
				})
				if err != nil {
					fmt.Printf("❌ Failed to download chunk %d: %v\n", chunkID, err)
					continue
				}

				// Extract files from chunk
				extracted, size, err := extractFilesFromChunk(chunkResult.Body, chunkFileList, outputDir, m.CompressionType, verbose)
				if err != nil {
					fmt.Printf("❌ Failed to extract chunk %d: %v\n", chunkID, err)
					_ = chunkResult.Body.Close()
					continue
				}

				_ = chunkResult.Body.Close()
				extractedCount += extracted
				extractedSize += size

				fmt.Printf("✅ Extracted %d files (%s) from chunk %d\n\n", extracted, humanize.Bytes(uint64(size)), chunkID)
			}

			duration := time.Since(startTime)
			throughput := float64(extractedSize) / duration.Seconds() / 1024 / 1024 // MB/s

			fmt.Printf("✅ Download complete!\n")
			fmt.Printf("   Files extracted: %d files\n", extractedCount)
			fmt.Printf("   Data size: %s\n", humanize.Bytes(uint64(extractedSize)))
			fmt.Printf("   Duration: %s\n", duration.Round(time.Millisecond))
			fmt.Printf("   Throughput: %.2f MB/s\n", throughput)
			fmt.Printf("   Output directory: %s\n", outputDir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (required)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix for upload (default: empty)")
	cmd.Flags().StringVarP(&uploadID, "upload-id", "u", "", "Upload ID to download (required)")
	cmd.Flags().StringVar(&pattern, "pattern", "", "Filter files by glob pattern (e.g., '*.log')")
	cmd.Flags().StringSliceVar(&files, "files", nil, "Comma-separated list of exact file paths to download")
	cmd.Flags().IntSliceVar(&shardIDs, "shard-ids", nil, "Comma-separated list of shard IDs to download (0-7)")
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "Output directory for extracted files (required unless --dry-run)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show verbose output (list each file as extracted)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be downloaded without actually downloading")
	cmd.Flags().IntVar(&workers, "workers", 4, "Number of parallel download workers (future use)")

	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("upload-id"); err != nil {
		panic(fmt.Sprintf("failed to mark upload-id flag as required: %v", err))
	}

	return cmd
}

// findChunk finds a chunk by ID in the manifest
func findChunk(m *manifest.Manifest, chunkID int) *manifest.ChunkEntry {
	for i := range m.Chunks {
		if m.Chunks[i].ID == chunkID {
			return &m.Chunks[i]
		}
	}
	return nil
}

// extractFilesFromChunk extracts selected files from a compressed tar archive
func extractFilesFromChunk(reader io.Reader, filesToExtract []manifest.FileEntry, outputDir, compressionType string, verbose bool) (int, int64, error) {
	// Build a map of files to extract for fast lookup
	fileMap := make(map[string]manifest.FileEntry)
	for _, file := range filesToExtract {
		fileMap[file.Path] = file
	}

	// Decompress based on compression type
	var decompressor io.Reader

	switch compressionType {
	case "zstd":
		decoder, err := zstd.NewReader(reader)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		defer decoder.Close()
		decompressor = decoder

	case "gzip":
		decoder, err := gzip.NewReader(reader)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create gzip decoder: %w", err)
		}
		defer func() { _ = decoder.Close() }()
		decompressor = decoder

	default:
		return 0, 0, fmt.Errorf("unsupported compression type: %s", compressionType)
	}

	// Extract files from tar archive
	tarReader := tar.NewReader(decompressor)
	extractedCount := 0
	var extractedSize int64

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return extractedCount, extractedSize, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Check if this file should be extracted
		fileEntry, shouldExtract := fileMap[header.Name]
		if !shouldExtract {
			continue
		}

		// Create output file path
		outputPath := filepath.Join(outputDir, header.Name)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return extractedCount, extractedSize, fmt.Errorf("failed to create directory for %s: %w", header.Name, err)
		}

		// Extract file
		switch header.Typeflag {
		case tar.TypeReg:
			// Regular file
			outFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return extractedCount, extractedSize, fmt.Errorf("failed to create file %s: %w", outputPath, err)
			}

			written, err := io.Copy(outFile, tarReader)
			_ = outFile.Close()
			if err != nil {
				return extractedCount, extractedSize, fmt.Errorf("failed to extract file %s: %w", header.Name, err)
			}

			// Restore modification time
			if err := os.Chtimes(outputPath, time.Now(), fileEntry.ModTime); err != nil {
				// Non-fatal, just warn
				if verbose {
					fmt.Printf("⚠️  Warning: failed to restore mod time for %s: %v\n", header.Name, err)
				}
			}

			extractedCount++
			extractedSize += written

			if verbose {
				fmt.Printf("  ✓ %s (%s)\n", header.Name, humanize.Bytes(uint64(written)))
			}

		case tar.TypeDir:
			// Directory
			if err := os.MkdirAll(outputPath, os.FileMode(header.Mode)); err != nil {
				return extractedCount, extractedSize, fmt.Errorf("failed to create directory %s: %w", outputPath, err)
			}
		}
	}

	return extractedCount, extractedSize, nil
}
