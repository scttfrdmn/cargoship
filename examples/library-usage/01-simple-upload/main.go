// Example 1: Reading a CargoShip Manifest
//
// This example demonstrates how to load and parse a CargoShip manifest file.
// Manifests contain all metadata about an uploaded archive, including file
// locations, chunk information, and compression settings.
//
// Usage:
//   go run main.go /path/to/manifest.json

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <manifest.json>")
	}

	manifestPath := os.Args[1]

	// Read the manifest file from disk
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Failed to read manifest: %v", err)
	}

	// Parse the JSON into a Manifest struct
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		log.Fatalf("Failed to parse manifest: %v", err)
	}

	// Display manifest metadata
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("CargoShip Manifest")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Upload ID:       %s\n", m.UploadID)
	fmt.Printf("Created:         %s\n", m.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Completed:       %s\n", m.CompletedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Source Path:     %s\n", m.SourcePath)
	fmt.Printf("Hostname:        %s\n", m.Hostname)
	fmt.Println()

	// Display S3 location
	fmt.Println("S3 Location:")
	fmt.Printf("  Bucket:        %s\n", m.Bucket)
	fmt.Printf("  Prefix:        %s\n", m.Prefix)
	fmt.Printf("  Region:        %s\n", m.Region)
	fmt.Println()

	// Display statistics
	fmt.Println("Statistics:")
	fmt.Printf("  Total Files:   %d\n", m.TotalFiles)
	fmt.Printf("  Total Bytes:   %d (%.2f GB)\n", m.TotalBytes, float64(m.TotalBytes)/(1024*1024*1024))
	fmt.Printf("  Total Chunks:  %d\n", m.TotalChunks)
	fmt.Printf("  Shard Count:   %d\n", m.ShardCount)
	fmt.Println()

	// Display compression settings
	fmt.Println("Compression:")
	fmt.Printf("  Type:          %s\n", m.CompressionType)
	fmt.Printf("  Level:         %d\n", m.CompressionLevel)
	fmt.Printf("  Ratio:         %.2f%%\n", m.CompressionRatio*100)
	fmt.Println()

	// Display encryption info if present
	if m.Encryption != nil && m.Encryption.Enabled {
		fmt.Println("Encryption:")
		fmt.Printf("  Enabled:       %t\n", m.Encryption.Enabled)
		if m.Encryption.DataKMSKeyID != "" {
			fmt.Printf("  Data KMS Key:  %s\n", m.Encryption.DataKMSKeyID)
		}
		if m.Encryption.ManifestEncrypted {
			fmt.Printf("  Manifest KMS:  %s\n", m.Encryption.ManifestKMSKeyID)
		}
		fmt.Println()
	}

	// Display first 10 files as a sample
	fmt.Println("Files (showing first 10):")
	fmt.Println("─────────────────────────────────────────────────────────────")
	displayCount := 10
	if len(m.Files) < displayCount {
		displayCount = len(m.Files)
	}
	for i := 0; i < displayCount; i++ {
		f := m.Files[i]
		fmt.Printf("%3d. %-40s  %10d bytes  chunk:%d  shard:%d\n",
			i+1, truncate(f.Path, 40), f.Size, f.ChunkID, f.ShardID)
	}
	if len(m.Files) > displayCount {
		fmt.Printf("     ... and %d more files\n", len(m.Files)-displayCount)
	}
	fmt.Println()

	// Display chunk summary
	fmt.Println("Chunks:")
	fmt.Println("─────────────────────────────────────────────────────────────")
	for i, chunk := range m.Chunks {
		if i >= 5 { // Show first 5 chunks
			fmt.Printf("     ... and %d more chunks\n", len(m.Chunks)-5)
			break
		}
		compressionPct := float64(chunk.CompressedSize) / float64(chunk.UncompressedSize) * 100
		fmt.Printf("Chunk %3d: %d files, %d bytes → %d bytes (%.1f%% compressed)\n",
			chunk.ID, chunk.FileCount, chunk.UncompressedSize, chunk.CompressedSize, compressionPct)
		fmt.Printf("           S3 Key: %s\n", chunk.S3Key)
	}
	fmt.Println()

	fmt.Println("✓ Manifest parsed successfully")
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
