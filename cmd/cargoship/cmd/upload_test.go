package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewUploadCmd_Structure tests the upload command structure (Issue #95)
func TestNewUploadCmd_Structure(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify command properties
	assert.Equal(t, "upload", cmd.Name())
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Verify command usage
	assert.Contains(t, cmd.Use, "SOURCE_DIR")
	assert.Contains(t, cmd.Use, "DESTINATION")

	// Verify flags exist
	flags := []string{"bucket", "region", "storage-class", "shard-count", "shard-strategy", "compression-level", "quiet"}
	for _, flag := range flags {
		f := cmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "Flag %s should exist", flag)
	}

	// Verify flag shortcuts
	assert.Equal(t, "b", cmd.Flags().Lookup("bucket").Shorthand)
	assert.Equal(t, "r", cmd.Flags().Lookup("region").Shorthand)

	// Verify default values
	assert.Equal(t, "us-west-2", cmd.Flags().Lookup("region").DefValue)
	assert.Equal(t, "STANDARD", cmd.Flags().Lookup("storage-class").DefValue)
	assert.Equal(t, "0", cmd.Flags().Lookup("shard-count").DefValue) // Issue #106: Auto-select by default
	assert.Equal(t, "hash", cmd.Flags().Lookup("shard-strategy").DefValue)
	assert.Equal(t, "3", cmd.Flags().Lookup("compression-level").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("quiet").DefValue)
}

// TestUploadCmd_FlagTypes tests that flags have correct types (Issue #95)
func TestUploadCmd_FlagTypes(t *testing.T) {
	cmd := NewUploadCmd()

	// String flags
	stringFlags := []string{"bucket", "region", "storage-class", "shard-strategy"}
	for _, flag := range stringFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "string", f.Value.Type(), "Flag %s should be string type", flag)
	}

	// Int flags
	intFlags := []string{"shard-count", "compression-level"}
	for _, flag := range intFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "int", f.Value.Type(), "Flag %s should be int type", flag)
	}

	// Bool flags
	boolFlags := []string{"quiet"}
	for _, flag := range boolFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "bool", f.Value.Type(), "Flag %s should be bool type", flag)
	}
}

// TestUploadCmd_HelpText tests help text includes key information (Issue #95)
func TestUploadCmd_HelpText(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify long description includes key features
	keyPhrases := []string{
		"CargoHold",
		"shard",
		"compression",
		"parallel",
		"manifest",
	}

	for _, phrase := range keyPhrases {
		assert.Contains(t, cmd.Long, phrase,
			"Help text should mention '%s'", phrase)
	}

	// Verify shard strategies are documented
	strategies := []string{"hash", "size", "type", "directory"}
	for _, strategy := range strategies {
		assert.Contains(t, cmd.Long, strategy,
			"Help text should document shard strategy '%s'", strategy)
	}

	// Verify examples are provided
	assert.Contains(t, cmd.Long, "Examples:")
	assert.Contains(t, cmd.Long, "cargoship upload")
}

// TestParseS3Destination tests S3 destination parsing (Issue #95)
func TestParseS3Destination(t *testing.T) {
	tests := []struct {
		name         string
		destination  string
		expectBucket string
		expectPrefix string
		expectError  bool
	}{
		{
			name:         "bucket only",
			destination:  "s3://my-bucket",
			expectBucket: "my-bucket",
			expectPrefix: "",
			expectError:  false,
		},
		{
			name:         "bucket with prefix",
			destination:  "s3://my-bucket/dataset",
			expectBucket: "my-bucket",
			expectPrefix: "dataset",
			expectError:  false,
		},
		{
			name:         "bucket with nested prefix",
			destination:  "s3://my-bucket/prod/cargoship/dataset",
			expectBucket: "my-bucket",
			expectPrefix: "prod/cargoship/dataset",
			expectError:  false,
		},
		{
			name:         "bucket with trailing slash",
			destination:  "s3://my-bucket/dataset/",
			expectBucket: "my-bucket",
			expectPrefix: "dataset",
			expectError:  false,
		},
		{
			name:        "invalid - no s3:// prefix",
			destination: "my-bucket/dataset",
			expectError: true,
		},
		{
			name:        "invalid - http URL",
			destination: "http://my-bucket/dataset",
			expectError: true,
		},
		{
			name:        "invalid - empty",
			destination: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix, err := parseS3Destination(tt.destination)

			if tt.expectError {
				assert.Error(t, err, "Expected error for destination: %s", tt.destination)
			} else {
				assert.NoError(t, err, "Unexpected error for destination: %s", tt.destination)
				assert.Equal(t, tt.expectBucket, bucket, "Bucket mismatch")
				assert.Equal(t, tt.expectPrefix, prefix, "Prefix mismatch")
			}
		})
	}
}

// TestUploadCmd_ShardStrategyValidation tests shard strategy validation (Issue #95)
func TestUploadCmd_ShardStrategyValidation(t *testing.T) {
	validStrategies := []string{"hash", "size", "type", "directory"}
	invalidStrategies := []string{"invalid", "random", "", "foo"}

	// Test that valid strategies are documented in help text
	cmd := NewUploadCmd()
	for _, strategy := range validStrategies {
		assert.Contains(t, cmd.Long, strategy,
			"Valid strategy '%s' should be documented", strategy)
	}

	// Test that invalid strategies would be rejected (validation happens in RunE)
	// We can't easily test RunE without S3, but we can verify the flag accepts string values
	for _, strategy := range invalidStrategies {
		cmd := NewUploadCmd()
		err := cmd.Flags().Set("shard-strategy", strategy)
		assert.NoError(t, err, "Flag should accept any string value (validation in RunE)")
	}
}

// TestUploadCmd_CompressionLevelRange tests compression level range (Issue #95)
func TestUploadCmd_CompressionLevelRange(t *testing.T) {
	// Verify default is within recommended range
	cmd := NewUploadCmd()
	defaultLevel := cmd.Flags().Lookup("compression-level").DefValue
	assert.Equal(t, "3", defaultLevel, "Default compression level should be 3")

	// Verify help text mentions zstd range
	assert.Contains(t, cmd.Flags().Lookup("compression-level").Usage, "1-22",
		"Flag usage should document zstd range")
	usage := cmd.Flags().Lookup("compression-level").Usage
	assert.True(t, strings.Contains(usage, "zstd") || strings.Contains(usage, "Zstd"),
		"Flag usage should mention zstd or Zstd")
}

// TestUploadCmd_ShardCountRange tests shard count range (Issue #95, updated for Issue #106)
func TestUploadCmd_ShardCountRange(t *testing.T) {
	// Verify default is 0 (auto-select)
	cmd := NewUploadCmd()
	defaultCount := cmd.Flags().Lookup("shard-count").DefValue
	assert.Equal(t, "0", defaultCount, "Default shard count should be 0 (auto)")

	// Verify help text mentions valid range and auto mode
	usage := cmd.Flags().Lookup("shard-count").Usage
	assert.Contains(t, usage, "0=auto",
		"Flag usage should document auto mode")
	assert.Contains(t, usage, "4-32",
		"Flag usage should document valid range")
}

// TestUploadCmd_RequiredArguments tests that command requires 2 arguments (Issue #95)
func TestUploadCmd_RequiredArguments(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify Args is set to ExactArgs(2)
	// We can't directly test cobra.ExactArgs, but we can verify the command has Args set
	assert.NotNil(t, cmd.Args, "Args validator should be set")
}
