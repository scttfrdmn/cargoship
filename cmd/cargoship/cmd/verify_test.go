package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewVerifyCmd_Structure tests the verify command structure (Issue #99)
func TestNewVerifyCmd_Structure(t *testing.T) {
	cmd := NewVerifyCmd()

	// Verify command properties
	assert.Equal(t, "verify", cmd.Name())
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Verify command use includes S3 URL argument
	assert.Contains(t, cmd.Use, "[s3://bucket/prefix/uploads/upload-id]")

	// Verify flags exist
	flags := []string{"bucket", "prefix", "upload-id", "region", "quick", "verbose"}
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
	assert.Equal(t, "false", cmd.Flags().Lookup("quick").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("verbose").DefValue)
}

// TestVerifyCmd_FlagTypes tests that flags have correct types (Issue #99)
func TestVerifyCmd_FlagTypes(t *testing.T) {
	cmd := NewVerifyCmd()

	// String flags
	stringFlags := []string{"bucket", "prefix", "upload-id", "region"}
	for _, flag := range stringFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "string", f.Value.Type(), "Flag %s should be string type", flag)
	}

	// Bool flags
	boolFlags := []string{"quick", "verbose"}
	for _, flag := range boolFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "bool", f.Value.Type(), "Flag %s should be bool type", flag)
	}
}

// TestVerifyCmd_HelpText tests help text includes key information (Issue #99)
func TestVerifyCmd_HelpText(t *testing.T) {
	cmd := NewVerifyCmd()

	// Verify long description includes key features
	keyPhrases := []string{
		"integrity",
		"manifest",
		"checksums",
		"validation",
	}

	for _, phrase := range keyPhrases {
		assert.Contains(t, cmd.Long, phrase,
			"Help text should mention '%s'", phrase)
	}

	// Verify examples are provided
	assert.Contains(t, cmd.Long, "Examples:")
	assert.Contains(t, cmd.Long, "cargoship verify")

	// Verify exit codes are documented
	assert.Contains(t, cmd.Long, "Exit Codes:")
	assert.Contains(t, cmd.Long, "0 - All checks passed")
	assert.Contains(t, cmd.Long, "1 - Verification failed")
}

// TestVerifyCmd_RequiredParameters tests parameter validation (Issue #99)
func TestVerifyCmd_RequiredParameters(t *testing.T) {
	tests := []struct {
		name        string
		bucket      string
		uploadID    string
		expectError bool
	}{
		{
			name:        "both bucket and upload-id provided",
			bucket:      "test-bucket",
			uploadID:    "test-upload-123",
			expectError: false, // Would fail on S3 call, but params are valid
		},
		{
			name:        "missing bucket",
			bucket:      "",
			uploadID:    "test-upload-123",
			expectError: true,
		},
		{
			name:        "missing upload-id",
			bucket:      "test-bucket",
			uploadID:    "",
			expectError: true,
		},
		{
			name:        "both missing",
			bucket:      "",
			uploadID:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewVerifyCmd()

			// Set flags
			_ = cmd.Flags().Set("bucket", tt.bucket)
			_ = cmd.Flags().Set("upload-id", tt.uploadID)

			// Execute command (will fail on S3 call, but we're testing param validation)
			err := cmd.RunE(cmd, []string{})

			if tt.expectError {
				assert.Error(t, err, "Expected error for missing required parameters")
			} else {
				// We expect an AWS error, not a parameter validation error
				if err != nil {
					assert.NotContains(t, err.Error(), "is required",
						"Should not be parameter validation error")
				}
			}
		})
	}
}

// TestVerifyCmd_S3URLParsing tests S3 URL argument parsing (Issue #99)
func TestVerifyCmd_S3URLParsing(t *testing.T) {
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
			// Parse S3 URL
			bucket, path, err := parseS3URLForTest(tt.s3URL)
			if err != nil && !tt.expectError {
				t.Fatalf("Failed to parse S3 URL: %v", err)
			}

			if tt.expectError && err != nil {
				return // Expected to fail
			}

			// Parse upload path
			prefix, uploadID, err := parseUploadPath(path)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectBucket, bucket)
				assert.Equal(t, tt.expectPrefix, prefix)
				assert.Equal(t, tt.expectUploadID, uploadID)
			}
		})
	}
}

// TestVerifyCmd_QuickFlag tests quick validation flag (Issue #99)
func TestVerifyCmd_QuickFlag(t *testing.T) {
	cmd := NewVerifyCmd()

	// Verify quick flag exists and defaults to false
	quickFlag := cmd.Flags().Lookup("quick")
	assert.NotNil(t, quickFlag)
	assert.Equal(t, "false", quickFlag.DefValue)
	assert.Equal(t, "bool", quickFlag.Value.Type())

	// Verify usage message
	assert.Contains(t, quickFlag.Usage, "Quick validation")
	assert.Contains(t, quickFlag.Usage, "metadata only")
}

// TestVerifyCmd_VerboseFlag tests verbose output flag (Issue #99)
func TestVerifyCmd_VerboseFlag(t *testing.T) {
	cmd := NewVerifyCmd()

	// Verify verbose flag exists and defaults to false
	verboseFlag := cmd.Flags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)
	assert.Equal(t, "bool", verboseFlag.Value.Type())

	// Verify usage message
	assert.Contains(t, verboseFlag.Usage, "detailed")
}
