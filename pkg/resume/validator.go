package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ChangeDetectionResult contains the results of file change detection
type ChangeDetectionResult struct {
	ModifiedFiles  []string // Files that have changed since upload started
	DeletedFiles   []string // Files that were in upload but are now missing
	NewFiles       []string // Files that exist now but weren't in original upload
	UnchangedFiles []string // Files that haven't changed
}

// HasChanges returns true if any files have been modified, deleted, or added
func (r *ChangeDetectionResult) HasChanges() bool {
	return len(r.ModifiedFiles) > 0 || len(r.DeletedFiles) > 0 || len(r.NewFiles) > 0
}

// TotalChanges returns the total number of changes detected
func (r *ChangeDetectionResult) TotalChanges() int {
	return len(r.ModifiedFiles) + len(r.DeletedFiles) + len(r.NewFiles)
}

// ValidateSourceFiles checks if source files have changed since the upload started
// Returns a ChangeDetectionResult with lists of modified/deleted/new files
func ValidateSourceFiles(state *UploadState) (*ChangeDetectionResult, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}
	if state.SourceDir == "" {
		return nil, fmt.Errorf("source directory cannot be empty")
	}

	result := &ChangeDetectionResult{
		ModifiedFiles:  []string{},
		DeletedFiles:   []string{},
		NewFiles:       []string{},
		UnchangedFiles: []string{},
	}

	// Track which files we've seen in the current scan
	seenFiles := make(map[string]bool)

	// If no file hashes were stored, we can't detect changes
	if len(state.FileHashes) == 0 {
		// Return empty result - no baseline to compare against
		return result, nil
	}

	// Scan source directory and check each file
	err := filepath.Walk(state.SourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files that can't be accessed
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(state.SourceDir, path)
		if err != nil {
			return err
		}

		// Mark as seen
		seenFiles[relPath] = true

		// Check if file was in original upload
		expectedHash, exists := state.FileHashes[relPath]
		if !exists {
			// New file - wasn't in original upload
			result.NewFiles = append(result.NewFiles, relPath)
			return nil
		}

		// Compute current hash
		actualHash, err := ComputeFileHash(path)
		if err != nil {
			// Skip files that can't be hashed (permissions, etc.)
			return nil
		}

		// Compare hashes
		if actualHash != expectedHash {
			result.ModifiedFiles = append(result.ModifiedFiles, relPath)
		} else {
			result.UnchangedFiles = append(result.UnchangedFiles, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan source directory: %w", err)
	}

	// Find deleted files (in original upload but not in current scan)
	for relPath := range state.FileHashes {
		if !seenFiles[relPath] {
			result.DeletedFiles = append(result.DeletedFiles, relPath)
		}
	}

	return result, nil
}

// ComputeFileHash computes the SHA256 hash of a file
// Returns hex-encoded hash string with "sha256:" prefix
func ComputeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	hashBytes := hasher.Sum(nil)
	return "sha256:" + hex.EncodeToString(hashBytes), nil
}

// ComputeDirectoryHashes computes hashes for all files in a directory
// Returns a map of relative path -> hash string
// Useful for building FileHashes during initial upload
func ComputeDirectoryHashes(rootDir string) (map[string]string, error) {
	hashes := make(map[string]string)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files that can't be accessed
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		// Compute hash
		hash, err := ComputeFileHash(path)
		if err != nil {
			// Skip files that can't be hashed
			return nil
		}

		hashes[relPath] = hash
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to compute directory hashes: %w", err)
	}

	return hashes, nil
}

// TODO: FastValidateSourceFiles - implement fast check using mtime+size
// Much faster than full validation but less reliable
// Would require storing mtime+size in UploadState alongside hashes
