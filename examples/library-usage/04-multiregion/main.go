// Example 4: Downloading Chunks from S3
//
// This example demonstrates how to download CargoShip manifests and chunks from S3.
// This is essential for ObjectFS to access CargoShip archives stored in S3.
//
// Usage:
//   go run main.go <s3-manifest-url> <chunk-filename> <cache-dir>
//   Example: go run main.go s3://mybucket/uploads/manifest.json chunk-001.tar.zst ./cache/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("Usage: go run main.go <s3-manifest-url> <chunk-filename> <cache-dir>")
	}

	manifestURL := os.Args[1]
	chunkFilename := os.Args[2]
	cacheDir := os.Args[3]

	if !strings.HasPrefix(manifestURL, "s3://") {
		log.Fatal("Manifest URL must start with s3://")
	}

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("CargoShip S3 Chunk Downloader")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Parse S3 URL
	bucket, key := parseS3URL(manifestURL)
	fmt.Printf("Bucket:       %s\n", bucket)
	fmt.Printf("Manifest key: %s\n", key)
	fmt.Printf("Cache dir:    %s\n", cacheDir)
	fmt.Println()

	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	}

	// Initialize AWS SDK
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	// Download manifest
	fmt.Println("Downloading manifest...")
	manifestPath := filepath.Join(cacheDir, "manifest.json")
	if err := downloadS3File(ctx, s3Client, bucket, key, manifestPath); err != nil {
		log.Fatalf("Failed to download manifest: %v", err)
	}
	fmt.Printf("✓ Manifest downloaded: %s\n", manifestPath)
	fmt.Println()

	// Parse manifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Failed to read manifest: %v", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		log.Fatalf("Failed to parse manifest: %v", err)
	}

	fmt.Printf("Manifest info:\n")
	fmt.Printf("  Upload ID:    %s\n", m.UploadID)
	fmt.Printf("  Total files:  %d\n", m.TotalFiles)
	fmt.Printf("  Total chunks: %d\n", m.TotalChunks)
	fmt.Println()

	// Find the chunk in the manifest
	var chunkEntry *manifest.ChunkEntry
	for i := range m.Chunks {
		if filepath.Base(m.Chunks[i].S3Key) == chunkFilename {
			chunkEntry = &m.Chunks[i]
			break
		}
	}

	if chunkEntry == nil {
		log.Fatalf("Chunk not found in manifest: %s", chunkFilename)
	}

	// Download chunk
	fmt.Printf("Downloading chunk: %s\n", chunkFilename)
	fmt.Printf("  Chunk ID:     %d\n", chunkEntry.ID)
	fmt.Printf("  Files:        %d\n", chunkEntry.FileCount)
	fmt.Printf("  Compressed:   %d bytes (%.2f MB)\n",
		chunkEntry.CompressedSize, float64(chunkEntry.CompressedSize)/(1024*1024))
	fmt.Printf("  S3 Key:       %s\n", chunkEntry.S3Key)
	fmt.Println()

	chunkPath := filepath.Join(cacheDir, chunkFilename)
	if err := downloadS3File(ctx, s3Client, bucket, chunkEntry.S3Key, chunkPath); err != nil {
		log.Fatalf("Failed to download chunk: %v", err)
	}

	fmt.Printf("✓ Chunk downloaded: %s\n", chunkPath)
	fmt.Println()

	// Verify download
	info, err := os.Stat(chunkPath)
	if err != nil {
		log.Fatalf("Failed to stat chunk: %v", err)
	}

	fmt.Println("Download complete:")
	fmt.Printf("  Local path:   %s\n", chunkPath)
	fmt.Printf("  Size:         %d bytes\n", info.Size())
	fmt.Printf("  Files in chunk: %d\n", chunkEntry.FileCount)
	fmt.Println()

	fmt.Println("Next steps:")
	fmt.Println("  1. Decompress: zstd -d " + chunkPath)
	fmt.Println("  2. List files: tar -tf " + strings.TrimSuffix(chunkPath, ".zst"))
	fmt.Println("  3. Extract: tar -xf " + strings.TrimSuffix(chunkPath, ".zst") + " <file-path>")
}

// parseS3URL parses s3://bucket/key into bucket and key
func parseS3URL(s3URL string) (bucket, key string) {
	s3URL = strings.TrimPrefix(s3URL, "s3://")
	parts := strings.SplitN(s3URL, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("Invalid S3 URL format: %s", s3URL)
	}
	return parts[0], parts[1]
}

// downloadS3File downloads a file from S3 to local disk
func downloadS3File(ctx context.Context, client *s3.Client, bucket, key, localPath string) error {
	// Get object from S3
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("S3 GetObject failed: %w", err)
	}
	defer result.Body.Close()

	// Create local file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy S3 object to file
	written, err := file.ReadFrom(result.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("  Downloaded %d bytes\n", written)
	return nil
}
