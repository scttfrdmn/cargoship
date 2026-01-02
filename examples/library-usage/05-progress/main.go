// Example 5: Manifest Validation
//
// This example demonstrates how to validate CargoShip manifests for integrity.
// ObjectFS should validate manifests before mounting to ensure data consistency.
//
// Usage:
//   go run main.go <manifest.json>

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

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("CargoShip Manifest Validation")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Load manifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Failed to read manifest: %v", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		log.Fatalf("Failed to parse manifest JSON: %v", err)
	}

	fmt.Printf("Manifest: %s\n", m.UploadID)
	fmt.Println()

	// Run validation
	fmt.Println("Running validation checks...")
	fmt.Println()

	errors := validateManifest(&m)

	if len(errors) == 0 {
		fmt.Println("✓ All validation checks passed!")
		fmt.Println()
		displayManifestSummary(&m)
	} else {
		fmt.Printf("✗ Validation failed with %d errors:\n", len(errors))
		fmt.Println("─────────────────────────────────────────────────────────────")
		for i, err := range errors {
			fmt.Printf("%d. %s\n", i+1, err)
		}
		fmt.Println()
		os.Exit(1)
	}
}

// validateManifest performs comprehensive validation checks
func validateManifest(m *manifest.Manifest) []string {
	var errors []string

	// 1. Check version
	if m.Version == "" {
		errors = append(errors, "Missing manifest version")
	}

	// 2. Check required fields
	if m.UploadID == "" {
		errors = append(errors, "Missing upload ID")
	}
	if m.Bucket == "" {
		errors = append(errors, "Missing S3 bucket")
	}
	if m.Region == "" {
		errors = append(errors, "Missing AWS region")
	}

	// 3. Check statistics consistency
	if m.TotalFiles != int64(len(m.Files)) {
		errors = append(errors, fmt.Sprintf("TotalFiles mismatch: declared=%d, actual=%d",
			m.TotalFiles, len(m.Files)))
	}
	if m.TotalChunks != len(m.Chunks) {
		errors = append(errors, fmt.Sprintf("TotalChunks mismatch: declared=%d, actual=%d",
			m.TotalChunks, len(m.Chunks)))
	}

	// 4. Check chunk references
	chunkIDs := make(map[int]bool)
	for _, chunk := range m.Chunks {
		chunkIDs[chunk.ID] = true
	}

	for i, file := range m.Files {
		if !chunkIDs[file.ChunkID] {
			errors = append(errors, fmt.Sprintf("File %d references non-existent chunk %d: %s",
				i, file.ChunkID, file.Path))
		}
	}

	// 5. Check shard references
	if m.ShardCount != len(m.Shards) {
		errors = append(errors, fmt.Sprintf("ShardCount mismatch: declared=%d, actual=%d",
			m.ShardCount, len(m.Shards)))
	}

	shardIDs := make(map[int]bool)
	for _, shard := range m.Shards {
		shardIDs[shard.ID] = true
	}

	for i, chunk := range m.Chunks {
		if !shardIDs[chunk.ShardID] {
			errors = append(errors, fmt.Sprintf("Chunk %d references non-existent shard %d",
				i, chunk.ShardID))
		}
	}

	// 6. Check total bytes calculation
	var calculatedBytes int64
	for _, file := range m.Files {
		calculatedBytes += file.Size
	}
	if m.TotalBytes != calculatedBytes {
		errors = append(errors, fmt.Sprintf("TotalBytes mismatch: declared=%d, calculated=%d",
			m.TotalBytes, calculatedBytes))
	}

	// 7. Check compression ratio
	if m.CompressionRatio < 0 || m.CompressionRatio > 1 {
		errors = append(errors, fmt.Sprintf("Invalid compression ratio: %.2f (must be 0-1)",
			m.CompressionRatio))
	}

	// 8. Check for duplicate file paths
	seen := make(map[string]bool)
	for i, file := range m.Files {
		if seen[file.Path] {
			errors = append(errors, fmt.Sprintf("Duplicate file path at index %d: %s",
				i, file.Path))
		}
		seen[file.Path] = true
	}

	return errors
}

// displayManifestSummary shows a summary of the validated manifest
func displayManifestSummary(m *manifest.Manifest) {
	fmt.Println("Manifest Summary:")
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Printf("Version:         %s\n", m.Version)
	fmt.Printf("Upload ID:       %s\n", m.UploadID)
	fmt.Printf("Created:         %s\n", m.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Source:          %s @ %s\n", m.SourcePath, m.Hostname)
	fmt.Println()
	fmt.Printf("S3 Location:     s3://%s/%s\n", m.Bucket, m.Prefix)
	fmt.Printf("Region:          %s\n", m.Region)
	fmt.Println()
	fmt.Printf("Files:           %d\n", m.TotalFiles)
	fmt.Printf("Total size:      %d bytes (%.2f GB)\n",
		m.TotalBytes, float64(m.TotalBytes)/(1024*1024*1024))
	fmt.Printf("Chunks:          %d\n", m.TotalChunks)
	fmt.Printf("Shards:          %d\n", m.ShardCount)
	fmt.Println()
	fmt.Printf("Compression:     %s (level %d, %.1f%% ratio)\n",
		m.CompressionType, m.CompressionLevel, m.CompressionRatio*100)

	if m.Encryption != nil && m.Encryption.Enabled {
		fmt.Println()
		fmt.Println("Encryption:      Enabled")
		if m.Encryption.DataKMSKeyID != "" {
			fmt.Printf("  Data KMS Key:  %s\n", m.Encryption.DataKMSKeyID)
		}
		if m.Encryption.ManifestEncrypted {
			fmt.Printf("  Manifest KMS:  %s\n", m.Encryption.ManifestKMSKeyID)
		}
	}
	fmt.Println()
	fmt.Println("✓ Manifest is valid and ready for use")
}
