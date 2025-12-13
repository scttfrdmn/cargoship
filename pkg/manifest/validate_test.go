package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// createValidManifest creates a valid test manifest
func createValidManifest() *Manifest {
	return &Manifest{
		Version:     ManifestVersion,
		UploadID:    "test-upload-123",
		CreatedAt:   time.Now(),
		Bucket:      "test-bucket",
		Region:      "us-west-2",
		Prefix:      "cargoship",
		ShardCount:  2,
		TotalFiles:  3,
		TotalBytes:  3000,
		TotalChunks: 2,
		Files: []FileEntry{
			{Path: "file1.txt", Size: 1000, ChunkID: 0, ShardID: 0},
			{Path: "file2.txt", Size: 1000, ChunkID: 0, ShardID: 0},
			{Path: "file3.txt", Size: 1000, ChunkID: 1, ShardID: 1},
		},
		Chunks: []ChunkEntry{
			{ID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst", FileCount: 2, UncompressedSize: 2000, CompressedSize: 1000},
			{ID: 1, ShardID: 1, S3Key: "shard-1/chunk-1.tar.zst", FileCount: 1, UncompressedSize: 1000, CompressedSize: 500},
		},
		Shards: []ShardEntry{
			{ID: 0, ChunkCount: 1, FileCount: 2, UncompressedSize: 2000, CompressedSize: 1000},
			{ID: 1, ChunkCount: 1, FileCount: 1, UncompressedSize: 1000, CompressedSize: 500},
		},
	}
}

// TestValidator_ValidManifest tests validation of a valid manifest (Issue #91)
func TestValidator_ValidManifest(t *testing.T) {
	m := createValidManifest()
	validator := NewValidator(m)

	result := validator.Validate()

	assert.True(t, result.Valid, "Valid manifest should pass validation")
	assert.Empty(t, result.Errors, "Valid manifest should have no errors")
	assert.True(t, result.Checks["version"], "Version check should pass")
	assert.True(t, result.Checks["metadata"], "Metadata check should pass")
	assert.True(t, result.Checks["shard_consistency"], "Shard consistency check should pass")
	assert.True(t, result.Checks["chunk_consistency"], "Chunk consistency check should pass")
	assert.True(t, result.Checks["file_consistency"], "File consistency check should pass")
	assert.True(t, result.Checks["checksums"], "Checksum check should pass")
}

// TestValidator_InvalidVersion tests version validation (Issue #91 - criterion 1)
func TestValidator_InvalidVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectError bool
		expectWarn  bool
	}{
		{
			name:        "missing version",
			version:     "",
			expectError: true,
		},
		{
			name:        "wrong version",
			version:     "v0.5.0",
			expectWarn:  true,
			expectError: false,
		},
		{
			name:        "valid version",
			version:     ManifestVersion,
			expectError: false,
			expectWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createValidManifest()
			m.Version = tt.version
			validator := NewValidator(m)

			result := validator.Validate()

			if tt.expectError {
				assert.False(t, result.Valid, "Should fail validation")
				assert.NotEmpty(t, result.Errors, "Should have errors")
				assert.False(t, result.Checks["version"], "Version check should fail")
			} else if tt.expectWarn {
				assert.True(t, result.Valid, "Should pass validation with warning")
				assert.NotEmpty(t, result.Warnings, "Should have warnings")
				assert.True(t, result.Checks["version"], "Version check should pass with warning")
			} else {
				assert.True(t, result.Valid, "Should pass validation")
				assert.Empty(t, result.Errors, "Should have no errors")
				assert.True(t, result.Checks["version"], "Version check should pass")
			}
		})
	}
}

// TestValidator_MissingMetadata tests metadata validation (Issue #91)
func TestValidator_MissingMetadata(t *testing.T) {
	tests := []struct {
		name       string
		modifyFunc func(*Manifest)
		errorField string
	}{
		{
			name: "missing upload ID",
			modifyFunc: func(m *Manifest) {
				m.UploadID = ""
			},
			errorField: "upload_id",
		},
		{
			name: "missing created timestamp",
			modifyFunc: func(m *Manifest) {
				m.CreatedAt = time.Time{}
			},
			errorField: "created_at",
		},
		{
			name: "missing bucket",
			modifyFunc: func(m *Manifest) {
				m.Bucket = ""
			},
			errorField: "bucket",
		},
		{
			name: "missing region",
			modifyFunc: func(m *Manifest) {
				m.Region = ""
			},
			errorField: "region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createValidManifest()
			tt.modifyFunc(m)
			validator := NewValidator(m)

			result := validator.Validate()

			assert.False(t, result.Valid, "Should fail validation")
			assert.NotEmpty(t, result.Errors, "Should have errors")
			assert.False(t, result.Checks["metadata"], "Metadata check should fail")

			// Verify specific error field
			found := false
			for _, err := range result.Errors {
				if err.Field == tt.errorField {
					found = true
					break
				}
			}
			assert.True(t, found, "Should have error for field %s", tt.errorField)
		})
	}
}

// TestValidator_ShardCountMismatch tests shard validation (Issue #91 - criterion 2)
func TestValidator_ShardCountMismatch(t *testing.T) {
	m := createValidManifest()
	m.ShardCount = 3 // Declared 3, but only have 2 shards
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["shard_consistency"], "Shard consistency check should fail")

	// Verify error message mentions shard count mismatch
	found := false
	for _, err := range result.Errors {
		if err.Field == "shard_count" {
			found = true
			assert.Contains(t, err.Message, "mismatch")
			break
		}
	}
	assert.True(t, found, "Should have shard count mismatch error")
}

// TestValidator_InvalidShardID tests shard ID validation (Issue #91)
func TestValidator_InvalidShardID(t *testing.T) {
	m := createValidManifest()
	m.Shards[0].ID = 10 // Out of range (should be 0-1)
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["shard_consistency"], "Shard consistency check should fail")
}

// TestValidator_ChunkCountMismatch tests chunk validation (Issue #91 - criterion 4)
func TestValidator_ChunkCountMismatch(t *testing.T) {
	m := createValidManifest()
	m.TotalChunks = 5 // Declared 5, but only have 2 chunks
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["chunk_consistency"], "Chunk consistency check should fail")

	// Verify error message mentions chunk count mismatch
	found := false
	for _, err := range result.Errors {
		if err.Field == "total_chunks" {
			found = true
			assert.Contains(t, err.Message, "mismatch")
			break
		}
	}
	assert.True(t, found, "Should have chunk count mismatch error")
}

// TestValidator_DuplicateChunkID tests duplicate chunk detection (Issue #91)
func TestValidator_DuplicateChunkID(t *testing.T) {
	m := createValidManifest()
	m.Chunks[1].ID = 0 // Duplicate ID
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["chunk_consistency"], "Chunk consistency check should fail")

	// Verify error message mentions duplicate
	found := false
	for _, err := range result.Errors {
		if contains(err.Message, "duplicate") || contains(err.Message, "Duplicate") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have duplicate chunk ID error")
}

// TestValidator_MissingChunkS3Key tests missing S3 key detection (Issue #91)
func TestValidator_MissingChunkS3Key(t *testing.T) {
	m := createValidManifest()
	m.Chunks[0].S3Key = "" // Missing S3 key
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["chunk_consistency"], "Chunk consistency check should fail")
}

// TestValidator_FileCountMismatch tests file validation (Issue #91)
func TestValidator_FileCountMismatch(t *testing.T) {
	m := createValidManifest()
	m.TotalFiles = 10 // Declared 10, but only have 3 files
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["file_consistency"], "File consistency check should fail")

	// Verify error message mentions file count mismatch
	found := false
	for _, err := range result.Errors {
		if err.Field == "total_files" {
			found = true
			assert.Contains(t, err.Message, "mismatch")
			break
		}
	}
	assert.True(t, found, "Should have file count mismatch error")
}

// TestValidator_DuplicateFilePath tests duplicate file detection (Issue #91)
func TestValidator_DuplicateFilePath(t *testing.T) {
	m := createValidManifest()
	m.Files[1].Path = "file1.txt" // Duplicate path
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["file_consistency"], "File consistency check should fail")

	// Verify error message mentions duplicate
	found := false
	for _, err := range result.Errors {
		if contains(err.Message, "Duplicate file path") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have duplicate file path error")
}

// TestValidator_InvalidFileChunkID tests file chunk ID validation (Issue #91)
func TestValidator_InvalidFileChunkID(t *testing.T) {
	m := createValidManifest()
	m.Files[0].ChunkID = 99 // Out of range
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["file_consistency"], "File consistency check should fail")
}

// TestValidator_TotalSizeMismatch tests total size validation (Issue #91)
func TestValidator_TotalSizeMismatch(t *testing.T) {
	m := createValidManifest()
	m.TotalBytes = 9999 // Doesn't match sum of file sizes (3000)
	validator := NewValidator(m)

	result := validator.Validate()

	assert.False(t, result.Valid, "Should fail validation")
	assert.NotEmpty(t, result.Errors, "Should have errors")
	assert.False(t, result.Checks["file_consistency"], "File consistency check should fail")

	// Verify error message mentions size mismatch
	found := false
	for _, err := range result.Errors {
		if err.Field == "total_bytes" {
			found = true
			assert.Contains(t, err.Message, "mismatch")
			break
		}
	}
	assert.True(t, found, "Should have total size mismatch error")
}

// TestValidator_ChecksumCoverage tests checksum validation (Issue #91 - criterion 3)
func TestValidator_ChecksumCoverage(t *testing.T) {
	t.Run("no checksums - warning only", func(t *testing.T) {
		m := createValidManifest()
		validator := NewValidator(m)

		result := validator.Validate()

		// Should pass validation but have warnings
		assert.True(t, result.Valid, "Should pass validation")
		assert.Empty(t, result.Errors, "Should have no errors")
		assert.NotEmpty(t, result.Warnings, "Should have warnings about missing checksums")
		assert.True(t, result.Checks["checksums"], "Checksum check should pass (optional)")
	})

	t.Run("with checksums - no warnings", func(t *testing.T) {
		m := createValidManifest()
		// Add checksums
		for i := range m.Files {
			m.Files[i].Checksum = "abc123"
		}
		for i := range m.Chunks {
			m.Chunks[i].Checksum = "def456"
		}

		validator := NewValidator(m)
		result := validator.Validate()

		assert.True(t, result.Valid, "Should pass validation")
		assert.Empty(t, result.Errors, "Should have no errors")
		// Should not have checksum warnings when checksums are present
		checksumWarnings := 0
		for _, warn := range result.Warnings {
			if contains(warn.Field, "checksum") {
				checksumWarnings++
			}
		}
		assert.Equal(t, 0, checksumWarnings, "Should have no checksum warnings when checksums present")
	})
}

// TestValidator_ValidateQuick tests quick validation (Issue #91)
func TestValidator_ValidateQuick(t *testing.T) {
	m := createValidManifest()
	// Add an error that would be caught by full validation but not quick validation
	m.Files[0].ChunkID = 99 // Invalid, but not checked in quick validation

	validator := NewValidator(m)
	result := validator.ValidateQuick()

	// Quick validation should pass (only checks version, metadata, shard consistency)
	assert.True(t, result.Valid, "Quick validation should pass")
	assert.Empty(t, result.Errors, "Quick validation should have no errors")

	// Full validation should fail
	fullResult := validator.Validate()
	assert.False(t, fullResult.Valid, "Full validation should fail")
	assert.NotEmpty(t, fullResult.Errors, "Full validation should have errors")
}

// TestValidationResult_Summary tests validation result summary (Issue #91 - criterion 5)
func TestValidationResult_Summary(t *testing.T) {
	result := &ValidationResult{
		Valid:  false,
		Checks: make(map[string]bool),
	}

	result.AddError("field1", "expected1", "actual1", "Error message 1")
	result.AddWarning("field2", "expected2", "actual2", "Warning message 1")

	summary := result.Summary()

	assert.Contains(t, summary, "Validation: FAIL")
	assert.Contains(t, summary, "Errors: 1")
	assert.Contains(t, summary, "Warnings: 1")
	assert.Contains(t, summary, "Error message 1")
	assert.Contains(t, summary, "Warning message 1")
}

// TestValidationError_Error tests ValidationError error interface (Issue #91)
func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:    "test_field",
		Expected: "expected_value",
		Actual:   "actual_value",
		Severity: "error",
		Message:  "Test error message",
	}

	errStr := err.Error()

	assert.Contains(t, errStr, "error")
	assert.Contains(t, errStr, "test_field")
	assert.Contains(t, errStr, "Test error message")
}

// TestValidationResult_Methods tests ValidationResult helper methods (Issue #91)
func TestValidationResult_Methods(t *testing.T) {
	result := &ValidationResult{
		Valid:  true,
		Checks: make(map[string]bool),
	}

	assert.False(t, result.HasErrors(), "Should have no errors initially")
	assert.False(t, result.HasWarnings(), "Should have no warnings initially")

	result.AddError("field1", "exp", "act", "Error")
	assert.True(t, result.HasErrors(), "Should have errors after adding error")
	assert.False(t, result.Valid, "Should be invalid after adding error")

	result.AddWarning("field2", "exp", "act", "Warning")
	assert.True(t, result.HasWarnings(), "Should have warnings after adding warning")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
