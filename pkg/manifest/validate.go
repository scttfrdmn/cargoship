package manifest

import (
	"fmt"
	"strings"
)

// ValidationError represents a manifest validation error
type ValidationError struct {
	Field    string // Field that failed validation
	Expected string // Expected value or condition
	Actual   string // Actual value found
	Severity string // "error" or "warning"
	Message  string // Human-readable error message
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Severity, e.Field, e.Message)
}

// ValidationResult contains the results of manifest validation
type ValidationResult struct {
	Valid    bool               // Overall validation status
	Errors   []*ValidationError // Critical errors
	Warnings []*ValidationError // Non-critical warnings
	Checks   map[string]bool    // Individual check results
}

// AddError adds a validation error
func (vr *ValidationResult) AddError(field, expected, actual, message string) {
	vr.Errors = append(vr.Errors, &ValidationError{
		Field:    field,
		Expected: expected,
		Actual:   actual,
		Severity: "error",
		Message:  message,
	})
	vr.Valid = false
}

// AddWarning adds a validation warning
func (vr *ValidationResult) AddWarning(field, expected, actual, message string) {
	vr.Warnings = append(vr.Warnings, &ValidationError{
		Field:    field,
		Expected: expected,
		Actual:   actual,
		Severity: "warning",
		Message:  message,
	})
}

// HasErrors returns true if there are any validation errors
func (vr *ValidationResult) HasErrors() bool {
	return len(vr.Errors) > 0
}

// HasWarnings returns true if there are any validation warnings
func (vr *ValidationResult) HasWarnings() bool {
	return len(vr.Warnings) > 0
}

// Summary returns a human-readable validation summary
func (vr *ValidationResult) Summary() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Validation: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[vr.Valid])
	fmt.Fprintf(&sb, "Errors: %d, Warnings: %d\n", len(vr.Errors), len(vr.Warnings))

	if len(vr.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range vr.Errors {
			fmt.Fprintf(&sb, "  • %s\n", err.Message)
		}
	}

	if len(vr.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warn := range vr.Warnings {
			fmt.Fprintf(&sb, "  • %s\n", warn.Message)
		}
	}

	return sb.String()
}

// Validator provides manifest validation functionality (Issue #91)
type Validator struct {
	manifest *Manifest
}

// NewValidator creates a new manifest validator
func NewValidator(m *Manifest) *Validator {
	return &Validator{manifest: m}
}

// Validate performs comprehensive manifest validation
func (v *Validator) Validate() *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: make(map[string]bool),
	}

	// Run all validation checks
	v.validateVersion(result)
	v.validateMetadata(result)
	v.validateShardConsistency(result)
	v.validateChunkConsistency(result)
	v.validateFileConsistency(result)
	v.validateChecksums(result)

	return result
}

// validateVersion verifies the manifest version (Issue #91 - criterion 1)
func (v *Validator) validateVersion(result *ValidationResult) {
	if v.manifest.Version == "" {
		result.AddError("version", ManifestVersion, "", "Manifest version is missing")
		result.Checks["version"] = false
		return
	}

	if v.manifest.Version != ManifestVersion {
		result.AddWarning("version", ManifestVersion, v.manifest.Version,
			fmt.Sprintf("Manifest version mismatch (expected %s, got %s)", ManifestVersion, v.manifest.Version))
		result.Checks["version"] = true // Warning, not error
		return
	}

	result.Checks["version"] = true
}

// validateMetadata validates essential manifest metadata (Issue #91)
func (v *Validator) validateMetadata(result *ValidationResult) {
	valid := true

	// Validate UploadID
	if v.manifest.UploadID == "" {
		result.AddError("upload_id", "non-empty", "", "Upload ID is missing")
		valid = false
	}

	// Validate timestamps
	if v.manifest.CreatedAt.IsZero() {
		result.AddError("created_at", "valid timestamp", "zero", "Created timestamp is missing")
		valid = false
	}

	// Validate S3 location
	if v.manifest.Bucket == "" {
		result.AddError("bucket", "non-empty", "", "S3 bucket is missing")
		valid = false
	}

	if v.manifest.Region == "" {
		result.AddError("region", "non-empty", "", "AWS region is missing")
		valid = false
	}

	result.Checks["metadata"] = valid
}

// validateShardConsistency validates shard data consistency (Issue #91 - criterion 2)
func (v *Validator) validateShardConsistency(result *ValidationResult) {
	valid := true

	// Check if shard count matches declared count
	if len(v.manifest.Shards) != v.manifest.ShardCount {
		result.AddError("shard_count",
			fmt.Sprintf("%d shards", v.manifest.ShardCount),
			fmt.Sprintf("%d shards", len(v.manifest.Shards)),
			fmt.Sprintf("Shard count mismatch: declared %d, found %d", v.manifest.ShardCount, len(v.manifest.Shards)))
		valid = false
	}

	// Validate each shard
	for i, shard := range v.manifest.Shards {
		// Check shard ID is within range
		if shard.ID < 0 || shard.ID >= v.manifest.ShardCount {
			result.AddError(fmt.Sprintf("shard[%d].id", i),
				fmt.Sprintf("0-%d", v.manifest.ShardCount-1),
				fmt.Sprintf("%d", shard.ID),
				fmt.Sprintf("Shard ID %d is out of range (0-%d)", shard.ID, v.manifest.ShardCount-1))
			valid = false
		}

		// Check shard has chunks
		if shard.ChunkCount <= 0 {
			result.AddWarning(fmt.Sprintf("shard[%d].chunk_count", i),
				">0",
				fmt.Sprintf("%d", shard.ChunkCount),
				fmt.Sprintf("Shard %d has no chunks", shard.ID))
		}

		// Check shard sizes are consistent
		if shard.CompressedSize > shard.UncompressedSize {
			result.AddWarning(fmt.Sprintf("shard[%d].size", i),
				"compressed <= uncompressed",
				fmt.Sprintf("compressed=%d > uncompressed=%d", shard.CompressedSize, shard.UncompressedSize),
				fmt.Sprintf("Shard %d: compressed size exceeds uncompressed size (unusual)", shard.ID))
		}
	}

	result.Checks["shard_consistency"] = valid
}

// validateChunkConsistency validates chunk data consistency (Issue #91 - criterion 4)
func (v *Validator) validateChunkConsistency(result *ValidationResult) {
	valid := true

	// Check if total chunks matches declared count
	if len(v.manifest.Chunks) != v.manifest.TotalChunks {
		result.AddError("total_chunks",
			fmt.Sprintf("%d chunks", v.manifest.TotalChunks),
			fmt.Sprintf("%d chunks", len(v.manifest.Chunks)),
			fmt.Sprintf("Chunk count mismatch: declared %d, found %d", v.manifest.TotalChunks, len(v.manifest.Chunks)))
		valid = false
	}

	// Validate each chunk
	chunkIDsSeen := make(map[int]bool)
	for i, chunk := range v.manifest.Chunks {
		// Check for duplicate chunk IDs
		if chunkIDsSeen[chunk.ID] {
			result.AddError(fmt.Sprintf("chunk[%d].id", i),
				"unique",
				fmt.Sprintf("%d (duplicate)", chunk.ID),
				fmt.Sprintf("Duplicate chunk ID %d", chunk.ID))
			valid = false
		}
		chunkIDsSeen[chunk.ID] = true

		// Check shard ID is valid
		if chunk.ShardID < 0 || chunk.ShardID >= v.manifest.ShardCount {
			result.AddError(fmt.Sprintf("chunk[%d].shard_id", i),
				fmt.Sprintf("0-%d", v.manifest.ShardCount-1),
				fmt.Sprintf("%d", chunk.ShardID),
				fmt.Sprintf("Chunk %d has invalid shard ID %d", chunk.ID, chunk.ShardID))
			valid = false
		}

		// Check chunk has S3 key
		if chunk.S3Key == "" {
			result.AddError(fmt.Sprintf("chunk[%d].s3_key", i),
				"non-empty",
				"",
				fmt.Sprintf("Chunk %d is missing S3 key", chunk.ID))
			valid = false
		}

		// Check chunk has files
		if chunk.FileCount <= 0 {
			result.AddWarning(fmt.Sprintf("chunk[%d].file_count", i),
				">0",
				fmt.Sprintf("%d", chunk.FileCount),
				fmt.Sprintf("Chunk %d has no files", chunk.ID))
		}

		// Check chunk sizes are consistent
		if chunk.CompressedSize > chunk.UncompressedSize {
			result.AddWarning(fmt.Sprintf("chunk[%d].size", i),
				"compressed <= uncompressed",
				fmt.Sprintf("compressed=%d > uncompressed=%d", chunk.CompressedSize, chunk.UncompressedSize),
				fmt.Sprintf("Chunk %d: compressed size exceeds uncompressed size (unusual)", chunk.ID))
		}
	}

	result.Checks["chunk_consistency"] = valid
}

// validateFileConsistency validates file data consistency (Issue #91)
func (v *Validator) validateFileConsistency(result *ValidationResult) {
	valid := true

	// Check if total files matches declared count
	if int64(len(v.manifest.Files)) != v.manifest.TotalFiles {
		result.AddError("total_files",
			fmt.Sprintf("%d files", v.manifest.TotalFiles),
			fmt.Sprintf("%d files", len(v.manifest.Files)),
			fmt.Sprintf("File count mismatch: declared %d, found %d", v.manifest.TotalFiles, len(v.manifest.Files)))
		valid = false
	}

	// Validate total size
	var calculatedSize int64
	filePathsSeen := make(map[string]bool)

	for i, file := range v.manifest.Files {
		// Check for duplicate file paths
		if filePathsSeen[file.Path] {
			result.AddError(fmt.Sprintf("file[%d].path", i),
				"unique",
				fmt.Sprintf("%s (duplicate)", file.Path),
				fmt.Sprintf("Duplicate file path: %s", file.Path))
			valid = false
		}
		filePathsSeen[file.Path] = true

		// Check file has path
		if file.Path == "" {
			result.AddError(fmt.Sprintf("file[%d].path", i),
				"non-empty",
				"",
				fmt.Sprintf("File at index %d is missing path", i))
			valid = false
		}

		// Check chunk ID is valid. Direct-upload manifests have no chunks
		// (TotalChunks == 0); each file is its own S3 object, so validate that it
		// carries an S3 key instead of a chunk reference. (Issue #228)
		if v.manifest.TotalChunks == 0 {
			if file.S3Key == "" {
				result.AddError(fmt.Sprintf("file[%d].s3_key", i),
					"non-empty",
					"",
					fmt.Sprintf("Direct-upload file %s has no S3 key", file.Path))
				valid = false
			}
		} else if file.ChunkID < 0 || file.ChunkID >= v.manifest.TotalChunks {
			result.AddError(fmt.Sprintf("file[%d].chunk_id", i),
				fmt.Sprintf("0-%d", v.manifest.TotalChunks-1),
				fmt.Sprintf("%d", file.ChunkID),
				fmt.Sprintf("File %s has invalid chunk ID %d", file.Path, file.ChunkID))
			valid = false
		}

		// Check shard ID is valid
		if file.ShardID < 0 || file.ShardID >= v.manifest.ShardCount {
			result.AddError(fmt.Sprintf("file[%d].shard_id", i),
				fmt.Sprintf("0-%d", v.manifest.ShardCount-1),
				fmt.Sprintf("%d", file.ShardID),
				fmt.Sprintf("File %s has invalid shard ID %d", file.Path, file.ShardID))
			valid = false
		}

		// Accumulate total size
		calculatedSize += file.Size
	}

	// Verify total size matches
	if calculatedSize != v.manifest.TotalBytes {
		result.AddError("total_bytes",
			fmt.Sprintf("%d bytes", v.manifest.TotalBytes),
			fmt.Sprintf("%d bytes", calculatedSize),
			fmt.Sprintf("Total size mismatch: declared %d, calculated %d", v.manifest.TotalBytes, calculatedSize))
		valid = false
	}

	result.Checks["file_consistency"] = valid
}

// validateChecksums validates file checksums if present (Issue #91 - criterion 3)
func (v *Validator) validateChecksums(result *ValidationResult) {
	checksumCount := 0
	chunkChecksumCount := 0

	// Count files with checksums
	for _, file := range v.manifest.Files {
		if file.Checksum != "" {
			checksumCount++
		}
	}

	// Count chunks with checksums
	for _, chunk := range v.manifest.Chunks {
		if chunk.Checksum != "" {
			chunkChecksumCount++
		}
	}

	// Report checksum coverage
	if checksumCount == 0 {
		result.AddWarning("file_checksums",
			fmt.Sprintf("%d checksums", len(v.manifest.Files)),
			"0 checksums",
			"No file checksums present (integrity verification unavailable)")
	}

	if chunkChecksumCount == 0 {
		result.AddWarning("chunk_checksums",
			fmt.Sprintf("%d checksums", len(v.manifest.Chunks)),
			"0 checksums",
			"No chunk checksums present (integrity verification unavailable)")
	}

	// Always pass - checksums are optional
	result.Checks["checksums"] = true
}

// ValidateQuick performs a quick validation (metadata and counts only)
func (v *Validator) ValidateQuick() *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: make(map[string]bool),
	}

	v.validateVersion(result)
	v.validateMetadata(result)
	v.validateShardConsistency(result)

	return result
}
