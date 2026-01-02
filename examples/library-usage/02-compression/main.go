// Example 2: Efficient File Lookup with ManifestQuery
//
// This example demonstrates how to use the ManifestQuery API for O(1) file lookups.
// This is critical for ObjectFS to provide fast POSIX operations on CargoShip archives.
//
// Usage:
//   go run main.go <manifest.json> <file-path-or-pattern>

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: go run main.go <manifest.json> <file-path-or-pattern>")
	}

	manifestPath := os.Args[1]
	searchTerm := os.Args[2]

	// Load manifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Failed to read manifest: %v", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		log.Fatalf("Failed to parse manifest: %v", err)
	}

	// Create ManifestQuery for O(1) lookups
	query := manifest.NewManifestQuery(&m)

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Searching in manifest: %s\n", m.UploadID)
	fmt.Printf("Total files: %d\n", m.TotalFiles)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Detect search type
	if strings.Contains(searchTerm, "*") || strings.Contains(searchTerm, "?") {
		// Pattern search (glob)
		findByPattern(query, &m, searchTerm)
	} else if strings.HasSuffix(searchTerm, "/") {
		// Directory listing
		listDirectory(query, &m, searchTerm)
	} else {
		// Exact file lookup
		findExactFile(query, &m, searchTerm)
	}
}

// findExactFile demonstrates O(1) exact file lookup
func findExactFile(query *manifest.ManifestQuery, m *manifest.Manifest, filePath string) {
	fmt.Printf("Searching for exact match: %s\n", filePath)
	fmt.Println()

	// O(1) hash map lookup
	fileEntry := query.FindFile(filePath)
	if fileEntry == nil {
		fmt.Printf("✗ File not found: %s\n", filePath)
		return
	}

	fmt.Println("✓ File found!")
	fmt.Println("─────────────────────────────────────────────────────────────")
	displayFileDetails(fileEntry, m)
}

// findByPattern demonstrates pattern matching
func findByPattern(query *manifest.ManifestQuery, m *manifest.Manifest, pattern string) {
	fmt.Printf("Searching for pattern: %s\n", pattern)
	fmt.Println()

	matches := query.ListFiles(pattern)

	if len(matches) == 0 {
		fmt.Printf("✗ No files match pattern: %s\n", pattern)
		return
	}

	fmt.Printf("✓ Found %d matching files:\n", len(matches))
	fmt.Println("─────────────────────────────────────────────────────────────")

	displayLimit := 20
	for i, file := range matches {
		if i >= displayLimit {
			fmt.Printf("... and %d more matches\n", len(matches)-displayLimit)
			break
		}
		fmt.Printf("%3d. %s (%d bytes, chunk:%d)\n",
			i+1, file.Path, file.Size, file.ChunkID)
	}
}

// listDirectory demonstrates directory listing
func listDirectory(query *manifest.ManifestQuery, m *manifest.Manifest, dirPath string) {
	fmt.Printf("Listing directory: %s\n", dirPath)
	fmt.Println()

	// Find all files in this directory (not recursive)
	var filesInDir []*manifest.FileEntry
	dirPrefix := strings.TrimSuffix(dirPath, "/") + "/"

	for i := range m.Files {
		file := &m.Files[i]
		if strings.HasPrefix(file.Path, dirPrefix) {
			// Check if it's a direct child (not in subdirectory)
			relPath := strings.TrimPrefix(file.Path, dirPrefix)
			if !strings.Contains(relPath, "/") {
				filesInDir = append(filesInDir, file)
			}
		}
	}

	if len(filesInDir) == 0 {
		fmt.Printf("✗ No files in directory: %s\n", dirPath)
		return
	}

	fmt.Printf("✓ Found %d files in directory:\n", len(filesInDir))
	fmt.Println("─────────────────────────────────────────────────────────────")

	for i, file := range filesInDir {
		basename := filepath.Base(file.Path)
		fmt.Printf("%3d. %-40s  %10d bytes\n", i+1, basename, file.Size)
	}
}

// displayFileDetails shows detailed information about a file
func displayFileDetails(file *manifest.FileEntry, m *manifest.Manifest) {
	fmt.Printf("Path:         %s\n", file.Path)
	fmt.Printf("Size:         %d bytes (%.2f KB)\n", file.Size, float64(file.Size)/1024)
	fmt.Printf("Modified:     %s\n", file.ModTime.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Find the chunk this file is in
	var chunkEntry *manifest.ChunkEntry
	for i := range m.Chunks {
		if m.Chunks[i].ID == file.ChunkID {
			chunkEntry = &m.Chunks[i]
			break
		}
	}

	if chunkEntry != nil {
		fmt.Println("Chunk Information:")
		fmt.Printf("  Chunk ID:          %d\n", chunkEntry.ID)
		fmt.Printf("  Shard ID:          %d\n", chunkEntry.ShardID)
		fmt.Printf("  S3 Key:            %s\n", chunkEntry.S3Key)
		fmt.Printf("  Files in chunk:    %d\n", chunkEntry.FileCount)
		fmt.Printf("  Compressed size:   %d bytes (%.2f MB)\n",
			chunkEntry.CompressedSize, float64(chunkEntry.CompressedSize)/(1024*1024))
		fmt.Printf("  Uncompressed size: %d bytes (%.2f MB)\n",
			chunkEntry.UncompressedSize, float64(chunkEntry.UncompressedSize)/(1024*1024))

		compressionRatio := float64(chunkEntry.CompressedSize) / float64(chunkEntry.UncompressedSize) * 100
		fmt.Printf("  Compression:       %.1f%%\n", compressionRatio)
		fmt.Println()

		fmt.Println("To extract this file:")
		fmt.Printf("  1. Download chunk: aws s3 cp s3://%s/%s ./\n", m.Bucket, chunkEntry.S3Key)
		fmt.Printf("  2. Decompress: zstd -d %s\n", filepath.Base(chunkEntry.S3Key))
		fmt.Printf("  3. Extract file: tar -xf <decompressed> %s\n", file.Path)
	}

	if file.Checksum != "" {
		fmt.Printf("\nChecksum (SHA256): %s\n", file.Checksum)
	}
}
