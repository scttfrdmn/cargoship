// Example 3: Extracting Files from Chunks
//
// This example demonstrates how to extract individual files from CargoShip chunks.
// This is the core operation ObjectFS needs to provide file read access.
//
// Usage:
//   go run main.go <chunk-file.tar.zst> <file-path> <output-dir>

package main

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("Usage: go run main.go <chunk-file.tar.zst> <file-path> <output-dir>")
	}

	chunkPath := os.Args[1]
	filePath := os.Args[2]
	outputDir := os.Args[3]

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("CargoShip File Extraction")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Chunk:          %s\n", filepath.Base(chunkPath))
	fmt.Printf("File to extract: %s\n", filePath)
	fmt.Printf("Output dir:     %s\n", outputDir)
	fmt.Println()

	// Verify chunk file exists
	if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
		log.Fatalf("Chunk file not found: %s", chunkPath)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Extract the file
	fmt.Println("Opening chunk...")
	extractedPath, err := extractFileFromChunk(chunkPath, filePath, outputDir)
	if err != nil {
		log.Fatalf("Failed to extract file: %v", err)
	}

	fmt.Println()
	fmt.Println("✓ File extracted successfully!")
	fmt.Printf("  Output path: %s\n", extractedPath)

	// Verify extracted file
	info, err := os.Stat(extractedPath)
	if err != nil {
		log.Fatalf("Failed to stat extracted file: %v", err)
	}

	fmt.Printf("  Size:        %d bytes (%.2f KB)\n", info.Size(), float64(info.Size())/1024)
	fmt.Printf("  Modified:    %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Display first few bytes if it's a text file
	if info.Size() > 0 && info.Size() < 1024*1024 { // < 1MB
		content, err := os.ReadFile(extractedPath)
		if err == nil && isPrintable(content[:min(100, len(content))]) {
			fmt.Println("File preview:")
			fmt.Println("─────────────────────────────────────────────────────────────")
			preview := string(content)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fmt.Println(preview)
		}
	}
}

// extractFileFromChunk extracts a specific file from a compressed tar chunk
func extractFileFromChunk(chunkPath, targetFile, outputDir string) (string, error) {
	// Open chunk file
	file, err := os.Open(chunkPath)
	if err != nil {
		return "", fmt.Errorf("failed to open chunk: %w", err)
	}
	defer file.Close()

	// Create zstd decoder
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()

	// Create tar reader
	tarReader := tar.NewReader(decoder)

	// Search for target file in tar archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("file not found in chunk: %s", targetFile)
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar header: %w", err)
		}

		// Check if this is the file we're looking for
		if header.Name != targetFile {
			continue
		}

		// Found the file! Extract it
		fmt.Printf("✓ Found file in chunk (size: %d bytes)\n", header.Size)
		fmt.Println("Extracting...")

		// Construct output path
		outputPath := filepath.Join(outputDir, filepath.Base(targetFile))

		// Create output file
		outFile, err := os.Create(outputPath)
		if err != nil {
			return "", fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()

		// Copy file content from tar to output file
		bytesWritten, err := io.Copy(outFile, tarReader)
		if err != nil {
			return "", fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Printf("✓ Wrote %d bytes\n", bytesWritten)

		// Set file permissions from tar header
		if err := outFile.Chmod(header.FileInfo().Mode()); err != nil {
			// Non-fatal: log warning but continue
			fmt.Printf("Warning: failed to set permissions: %v\n", err)
		}

		return outputPath, nil
	}
}

// isPrintable checks if bytes are printable ASCII/UTF-8
func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			return false
		}
	}
	return true
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
