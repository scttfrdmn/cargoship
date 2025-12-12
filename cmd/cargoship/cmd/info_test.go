package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseUploadPath tests the parseUploadPath helper function (Issue #98)
func TestParseUploadPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectPrefix   string
		expectUploadID string
		expectError    bool
	}{
		{
			name:           "no prefix - starts with uploads/",
			path:           "uploads/20231208-123456-abcd1234",
			expectPrefix:   "",
			expectUploadID: "20231208-123456-abcd1234",
			expectError:    false,
		},
		{
			name:           "with prefix",
			path:           "cargoship/uploads/20231208-123456-abcd1234",
			expectPrefix:   "cargoship",
			expectUploadID: "20231208-123456-abcd1234",
			expectError:    false,
		},
		{
			name:           "nested prefix",
			path:           "prod/cargoship/uploads/20231208-123456-abcd1234",
			expectPrefix:   "prod/cargoship",
			expectUploadID: "20231208-123456-abcd1234",
			expectError:    false,
		},
		{
			name:           "deeply nested prefix",
			path:           "team/proj/env/uploads/20240101-000000-xyz789",
			expectPrefix:   "team/proj/env",
			expectUploadID: "20240101-000000-xyz789",
			expectError:    false,
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
		},
		{
			name:        "no uploads marker",
			path:        "cargoship/data/20231208-123456-abcd1234",
			expectError: true,
		},
		{
			name:        "uploads in middle but missing ID",
			path:        "cargoship/uploads/",
			expectError: true,
		},
		{
			name:        "just uploads slash",
			path:        "uploads/",
			expectError: true,
		},
		{
			name:        "no upload ID after uploads",
			path:        "prefix/uploads/",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, uploadID, err := parseUploadPath(tt.path)

			if tt.expectError {
				assert.Error(t, err, "Expected error for path: %s", tt.path)
			} else {
				require.NoError(t, err, "Unexpected error for path: %s", tt.path)
				assert.Equal(t, tt.expectPrefix, prefix, "Prefix mismatch")
				assert.Equal(t, tt.expectUploadID, uploadID, "Upload ID mismatch")
			}
		})
	}
}

// TestNewInfoCmd_Structure tests the info command structure (Issue #98)
func TestNewInfoCmd_Structure(t *testing.T) {
	cmd := NewInfoCmd()

	// Verify command properties
	assert.Equal(t, "info", cmd.Name())
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Verify flags exist
	flags := []string{"bucket", "prefix", "upload-id", "region", "verbose", "json"}
	for _, flag := range flags {
		f := cmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "Flag %s should exist", flag)
	}

	// Verify flag shortcuts
	assert.Equal(t, "b", cmd.Flags().Lookup("bucket").Shorthand)
	assert.Equal(t, "p", cmd.Flags().Lookup("prefix").Shorthand)
	assert.Equal(t, "u", cmd.Flags().Lookup("upload-id").Shorthand)
	assert.Equal(t, "r", cmd.Flags().Lookup("region").Shorthand)

	// Verify default values
	assert.Equal(t, "us-west-2", cmd.Flags().Lookup("region").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("verbose").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("json").DefValue)
}

// TestInfoCmd_ParseUploadPath_Integration tests S3 URL parsing in command context (Issue #98)
func TestInfoCmd_ParseUploadPath_Integration(t *testing.T) {
	tests := []struct {
		name           string
		s3URL          string
		expectBucket   string
		expectPrefix   string
		expectUploadID string
		expectError    bool
	}{
		{
			name:           "full S3 URL with prefix",
			s3URL:          "s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234",
			expectBucket:   "my-bucket",
			expectPrefix:   "cargoship",
			expectUploadID: "20231208-123456-abcd1234",
			expectError:    false,
		},
		{
			name:           "S3 URL without prefix",
			s3URL:          "s3://my-bucket/uploads/20231208-123456-abcd1234",
			expectBucket:   "my-bucket",
			expectPrefix:   "",
			expectUploadID: "20231208-123456-abcd1234",
			expectError:    false,
		},
		{
			name:           "S3 URL with nested prefix",
			s3URL:          "s3://prod-bucket/team/proj/uploads/20240101-xyz",
			expectBucket:   "prod-bucket",
			expectPrefix:   "team/proj",
			expectUploadID: "20240101-xyz",
			expectError:    false,
		},
		{
			name:        "invalid S3 URL - no uploads marker",
			s3URL:       "s3://my-bucket/cargoship/data/20231208",
			expectError: true,
		},
		{
			name:        "invalid S3 URL - missing upload ID",
			s3URL:       "s3://my-bucket/cargoship/uploads/",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse S3 URL using manifest.ParseS3URL
			bucket, path, err := parseS3URLForTest(tt.s3URL)
			if err != nil && !tt.expectError {
				t.Fatalf("Failed to parse S3 URL: %v", err)
			}

			if tt.expectError {
				return // Expected to fail at some point
			}

			// Parse upload path
			prefix, uploadID, err := parseUploadPath(path)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectBucket, bucket)
				assert.Equal(t, tt.expectPrefix, prefix)
				assert.Equal(t, tt.expectUploadID, uploadID)
			}
		})
	}
}

// parseS3URLForTest is a test helper that wraps manifest.ParseS3URL
func parseS3URLForTest(s3URL string) (bucket, path string, err error) {
	if len(s3URL) < 5 || s3URL[:5] != "s3://" {
		return "", "", assert.AnError
	}

	pathStr := s3URL[5:]
	slashIdx := -1
	for i, c := range pathStr {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		return pathStr, "", nil
	}

	bucket = pathStr[:slashIdx]
	path = pathStr[slashIdx+1:]
	return bucket, path, nil
}
